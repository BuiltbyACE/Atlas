package main

import (
	"context"
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
	pool, _ := database.Connect(ctx, "postgres://atlas:atlas@localhost:55433/atlas?sslmode=disable", slog.Default())
	defer pool.Close()
	seed.Run(ctx, pool)
	var pid uuid.UUID
	pool.QueryRow(ctx, "SELECT id FROM people WHERE email=", "warsamehusseinosman@gmail.com").Scan(&pid)
	var rid uuid.UUID
	pool.QueryRow(ctx, "SELECT id FROM core.roles WHERE key='ceo'").Scan(&rid)
	pool.Exec(ctx, "INSERT INTO core.person_roles (person_id, role_id) VALUES (,) ON CONFLICT DO NOTHING", pid, rid)
}
