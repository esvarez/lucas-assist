// Command local runs the same router/handlers as cmd/api behind a plain
// http.Server on :8080 — no SAM local emulation. Only the lambda.Start
// wrapper differs between cmd/api and this (architecture.md §11).
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/esvarez/lucas-assist/internal/api"
	"github.com/esvarez/lucas-assist/internal/store"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx := context.Background()

	endpoint := getenv("DYNAMODB_ENDPOINT", "http://localhost:8000")
	client, err := store.NewDynamoDBClient(ctx, endpoint)
	if err != nil {
		log.Fatalf("new dynamodb client: %v", err)
	}

	table := getenv("DYNAMODB_TABLE", "nudge-local")
	if err := store.EnsureTable(ctx, client, table); err != nil {
		log.Fatalf("ensure table %q: %v", table, err)
	}

	repo := store.NewDynamoRepository(client, table)
	router := api.NewRouter(repo)

	addr := ":8080"
	log.Printf("listening on %s (dynamodb endpoint %s, table %s)", addr, endpoint, table)
	log.Fatal(http.ListenAndServe(addr, router))
}
