package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/infrai-examples/game-invoice-mailer/internal/invoice"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := invoice.NewClient(key, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /invoices", func(w http.ResponseWriter, r *http.Request) {
		var request invoice.Invoice
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
			http.Error(w, "invalid invoice JSON", http.StatusBadRequest)
			return
		}
		result, err := client.Send(r.Context(), request)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, invoice.ErrAssetsPending) {
				status = http.StatusConflict
			}
			var apiErr *invoice.APIError
			if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
				status = apiErr.HTTPStatus
			}
			http.Error(w, err.Error(), status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})
	log.Println("invoice service listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
