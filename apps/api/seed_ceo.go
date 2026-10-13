package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/safaristack/atlas/apps/api/internal/platform/database"
	"github.com/safaristack/atlas/apps/api/internal/seed"
)

func main() {
	ctx := context.Background()
	pool, err := database.Connect(ctx, "postgres://atlas:atlas@localhost:55433/atlas?sslmode=disable", slog.Default())
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	if _, err := seed.Run(ctx, pool); err != nil {
		panic(err)
	}
	fmt.Println("seed done")
}
