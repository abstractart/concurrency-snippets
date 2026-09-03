package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/concurrency-examples/golang/internal/transfer_balance/pg_optimistic"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is not set")
		fmt.Fprintln(os.Stderr, "Example: DATABASE_URL=postgres://user:pass@localhost/db go run ./cmd/transfer_balance/pg_optimistic")
		os.Exit(1)
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect:", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := pg_optimistic.Run(ctx, db); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
