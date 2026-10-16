package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// ErrInvalid marks a read request the caller got wrong (for example a history
// query without an entity).
var ErrInvalid = errors.New("audit: invalid")

// Service reads the audit trail (§6.3). The write side is the Writer.
type Service struct {
	store  Store
	logger *slog.Logger
}

// NewService builds the reader.
func NewService(store Store, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, logger: logger}
}

// List returns a page of the audit trail, newest first.
func (s *Service) List(ctx context.Context, q Query, limit int, before *Cursor) ([]Entry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.List(ctx, q, limit, before)
}

// History returns a page of one record's history.
func (s *Service) History(ctx context.Context, entityType string, entityID uuid.UUID, limit int, before *Cursor) ([]Entry, error) {
	if entityType == "" {
		return nil, fmt.Errorf("%w: entity_type is required", ErrInvalid)
	}
	return s.List(ctx, Query{EntityType: entityType, EntityID: &entityID}, limit, before)
}
