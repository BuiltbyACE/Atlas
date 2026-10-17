package integrations

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// PGStore is the Postgres-backed Store.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// ClaimDelivery records a delivery and reports whether it is new. A unique
// conflict on delivery_id is a redelivery of the same delivery and a no-op.
func (s *PGStore) ClaimDelivery(ctx context.Context, deliveryID, eventType string) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		INSERT INTO integrations.github_deliveries (delivery_id, event_type)
		VALUES ($1, $2)
		ON CONFLICT (delivery_id) DO NOTHING`, deliveryID, eventType)
	if err != nil {
		return false, fmt.Errorf("integrations: claim delivery: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ReleaseDelivery forgets a delivery so a processing failure can be retried.
func (s *PGStore) ReleaseDelivery(ctx context.Context, deliveryID string) error {
	if _, err := s.db.Exec(ctx, `DELETE FROM integrations.github_deliveries WHERE delivery_id = $1`, deliveryID); err != nil {
		return fmt.Errorf("integrations: release delivery: %w", err)
	}
	return nil
}

// WriteEvent appends the integrations fact to the outbox.
func (s *PGStore) WriteEvent(ctx context.Context, ev outbox.Event) error {
	if _, err := outbox.Write(ctx, s.db, ev); err != nil {
		return fmt.Errorf("integrations: %w", err)
	}
	return nil
}
