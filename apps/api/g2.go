package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/safaristack/atlas/apps/api/internal/platform/database"
)

func main() {
	ctx := context.Background()
	pool, _ := database.Connect(ctx, "postgres://atlas:atlas@localhost:55433/atlas?sslmode=disable", slog.Default())
	defer pool.Close()
	var pid uuid.UUID
	_ = pool.QueryRow(ctx, "SELECT id FROM people WHERE email=", "warsamehusseinosman@gmail.com").Scan(&pid)
	var rid uuid.UUID
	_ = pool.QueryRow(ctx, "SELECT id FROM core.roles WHERE key='ceo'").Scan(&rid)
	_, _ = pool.Exec(ctx, "INSERT INTO core.person_roles (person_id, role_id) VALUES (,) ON CONFLICT DO NOTHING", pid, rid)
	fmt.Println("ok")
}
