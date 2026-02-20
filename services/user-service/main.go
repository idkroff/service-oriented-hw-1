package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "user-service",
		})
	})

	log.Println("user-service listening on :8000")
	log.Fatal(http.ListenAndServe(":8000", nil))
}
