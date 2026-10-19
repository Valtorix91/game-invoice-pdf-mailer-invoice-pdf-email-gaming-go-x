package invoice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestSendModerationDecision(t *testing.T) {
	tests := []struct {
		name      string
		approved  bool
		wantCalls int
		wantErr   error
	}{
		{name: "approved asset generates and emails invoice", approved: true, wantCalls: 2},
		{name: "queued asset blocks billing", approved: false, wantCalls: 0, wantErr: ErrAssetsPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodPost || req.Header.Get("Authorization") != "Bearer test-key" || req.Header.Get("Idempotency-Key") == "" {
					t.Fatalf("unexpected request boundary: %s %#v", req.Method, req.Header)
				}
				var data any
				switch req.URL.Path {
				case "/v1/pdf/generate":
					data = map[string]any{
						"pdf_id":         "pdf_12345678901234567890",
						"url":            "https://documents.example/invoice.pdf",
						"size_bytes":     1024,
						"page_count":     1,
						"sha256":         strings.Repeat("a", 64),
						"created_at":     "2026-09-26T00:00:00Z",
						"retention_days": 7,
						"source":         "markdown",
					}
				case "/v1/email/send":
					var payload map[string]any
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(payload["html"].(string), "https://documents.example/invoice.pdf") {
						t.Fatal("email did not receive generated document reference")
					}
					data = map[string]string{"message_id": "msg_123"}
				default:
					t.Fatalf("unexpected path %s", req.URL.Path)
				}
				encoded, _ := json.Marshal(map[string]any{"ok": true, "data": data, "metadata": map[string]any{}})
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(encoded))), Header: make(http.Header)}, nil
			})
			client := NewClient("test-key", &http.Client{Transport: transport})
			result, err := client.Send(context.Background(), Invoice{
				ID: "inv_2048", CustomerEmail: "player@example.com", Player: "Ada",
				Assets: []Asset{{Name: "Guild crest", Approved: tt.approved}},
				Events: []LiveEvent{{Name: "Season final", Minutes: 90}}, AmountCents: 4200,
			})
			if !errorsIs(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", calls, tt.wantCalls)
			}
			if tt.wantErr == nil && result.MessageID != "msg_123" {
				t.Fatalf("message id = %q", result.MessageID)
			}
		})
	}
}

func errorsIs(got, want error) bool {
	if want == nil {
		return got == nil
	}
	return got == want
}
