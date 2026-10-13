package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/safaristack/atlas/apps/api/internal/platform/database"
	"github.com/safaristack/atlas/apps/api/internal/seed"
)

func main() {
	ctx := context.Background()
	os.Setenv("ATLAS_ADMIN_EMAIL", "warsamehusseinosman@gmail.com")
	os.Setenv("ATLAS_ADMIN_PASSWORD", "07077357337.w")
	pool, err := database.Connect(ctx, "postgres://atlas:atlas@localhost:55433/atlas?sslmode=disable", slog.Default())
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	if _, err := seed.Run(ctx, pool); err != nil {
		panic(err)
	}

	var personID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM people WHERE email=", "warsamehusseinosman@gmail.com").Scan(&personID); err != nil {
		panic(err)
	}
	var roleID uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT id FROM core.roles WHERE key='ceo'").Scan(&roleID); err != nil {
		panic(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO core.person_roles (person_id, role_id) VALUES (, ) ON CONFLICT DO NOTHING", personID, roleID); err != nil {
		panic(err)
	}
	fmt.Println("ok")
}

