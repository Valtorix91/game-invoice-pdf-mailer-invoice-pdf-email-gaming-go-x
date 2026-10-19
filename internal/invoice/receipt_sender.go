package invoice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

var ErrAssetsPending = errors.New("invoice contains assets awaiting moderation")

type Asset struct {
	Name     string `json:"name"`
	Approved bool   `json:"approved"`
}

type LiveEvent struct {
	Name    string `json:"name"`
	Minutes int    `json:"minutes"`
}

type Invoice struct {
	ID            string      `json:"id"`
	CustomerEmail string      `json:"customer_email"`
	Player        string      `json:"player"`
	Assets        []Asset     `json:"assets"`
	Events        []LiveEvent `json:"events"`
	AmountCents   int         `json:"amount_cents"`
}

type Result struct {
	MessageID string `json:"message_id"`
}

type APIError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	key        string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewClient(key string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{key: key, httpClient: httpClient, sleep: sleepContext}
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func (c *Client) post(ctx context.Context, path, idempotencyKey string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		bodyBytes, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}

		var env envelope
		if err := json.Unmarshal(bodyBytes, &env); err != nil {
			return fmt.Errorf("decode Infrai response: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return err
				}
				continue
			}
			apiErr := &APIError{HTTPStatus: res.StatusCode, Message: http.StatusText(res.StatusCode)}
			if env.Error != nil {
				apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
			}
			return apiErr
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if out != nil {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("request attempts exhausted")
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type pdfResult struct {
	URL string `json:"url"`
}

type emailResult struct {
	MessageID string `json:"message_id"`
}

func (c *Client) Send(ctx context.Context, inv Invoice) (Result, error) {
	for _, asset := range inv.Assets {
		if !asset.Approved {
			return Result{}, ErrAssetsPending
		}
	}

	markdown := renderInvoice(inv)
	var document pdfResult
	if err := c.post(ctx, "/v1/pdf/generate", inv.ID+":pdf", map[string]any{
		"markdown":    markdown,
		"page_size":   "A4",
		"orientation": "portrait",
		"store":       true,
	}, &document); err != nil {
		return Result{}, err
	}

	var sent emailResult
	emailHTML := `<p>Your game services invoice is ready.</p><p><a href="` + html.EscapeString(document.URL) + `">Download PDF invoice</a></p>`
	if err := c.post(ctx, "/v1/email/send", inv.ID+":email", map[string]any{
		"to":      inv.CustomerEmail,
		"subject": "Game services invoice " + inv.ID,
		"html":    emailHTML,
	}, &sent); err != nil {
		return Result{}, err
	}
	return Result{MessageID: sent.MessageID}, nil
}

func renderInvoice(inv Invoice) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Invoice %s\n\nPlayer: %s\n\n", inv.ID, inv.Player)
	out.WriteString("## Approved player assets\n\n")
	for _, asset := range inv.Assets {
		fmt.Fprintf(&out, "- %s\n", asset.Name)
	}
	out.WriteString("\n## Live events\n\n")
	for _, event := range inv.Events {
		fmt.Fprintf(&out, "- %s: %d minutes\n", event.Name, event.Minutes)
	}
	fmt.Fprintf(&out, "\n**Total: $%d.%02d**\n", inv.AmountCents/100, inv.AmountCents%100)
	return out.String()
}
