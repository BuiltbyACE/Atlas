package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/auth"
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

	// ensure CEO role exists; add all permissions to CEO by re-running seed already done? also ensure the person has CEO role
	// find person
	var personID uuid.UUID
	err = pool.QueryRow(ctx, SELECT id FROM people WHERE email=, "warsamehusseinosman@gmail.com").Scan(&personID)
	if err != nil {
		panic(err)
	}
	var roleID uuid.UUID
	err = pool.QueryRow(ctx, SELECT id FROM core.roles WHERE key='ceo').Scan(&roleID)
	if err != nil {
		panic(err)
	}
	// grant ceo role
	_, err = pool.Exec(ctx, INSERT INTO core.person_roles (person_id, role_id) VALUES (,) ON CONFLICT DO NOTHING, personID, roleID)
	if err != nil {
		panic(err)
	}
	fmt.Println("ok", personID.String())
}
