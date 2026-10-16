package settings

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

// Service reads and writes typed settings.
type Service struct {
	store  Store
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the settings service. aw may be nil (tests, or a
// deployment without an audit writer); when it is, changes are still versioned
// but not written to the global audit log.
func NewService(store Store, aw *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:  store,
		audit:  aw,
		now:    func() time.Time { return time.Now().UTC() },
		logger: logger,
	}
}

// List returns every known setting, in key order, with the stored value where
// one exists and the definition default otherwise.
func (s *Service) List(ctx context.Context) ([]Setting, error) {
	stored, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]Setting, len(stored))
	for _, st := range stored {
		byKey[st.Key] = st
	}

	defs := Definitions()
	out := make([]Setting, 0, len(defs))
	for _, d := range defs {
		if st, ok := byKey[d.Key]; ok {
			st.Kind = d.Kind
			if st.Description == "" {
				st.Description = d.Description
			}
			out = append(out, st)
			continue
		}
		out = append(out, Setting{
			Key: d.Key, Value: d.Default, Description: d.Description, Kind: d.Kind,
		})
	}
	return out, nil
}

// Get returns one known setting, or ErrNotFound for an unknown key.
func (s *Service) Get(ctx context.Context, key string) (Setting, error) {
	d, ok := DefinitionFor(key)
	if !ok {
		return Setting{}, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	st, err := s.store.Get(ctx, key)
	if err != nil {
		return Setting{}, err
	}
	if st == nil {
		return Setting{Key: key, Value: d.Default, Description: d.Description, Kind: d.Kind}, nil
	}
	st.Kind = d.Kind
	if st.Description == "" {
		st.Description = d.Description
	}
	return *st, nil
}

// Set validates a value against its definition, writes it, and audits the
// change. A stale ExpectedVersion is an ErrConflict.
func (s *Service) Set(ctx context.Context, in SetInput) (Setting, error) {
	d, ok := DefinitionFor(in.Key)
	if !ok {
		return Setting{}, fmt.Errorf("%w: %s", ErrNotFound, in.Key)
	}
	if err := d.Validate(in.Value); err != nil {
		return Setting{}, err
	}
	in.Description = d.Description

	st, chg, err := s.store.Set(ctx, in)
	if err != nil {
		return Setting{}, err
	}

	if err := s.writeAudit(ctx, chg); err != nil {
		// The value is committed; a failed audit write must not report the
		// change as failed (§6.3). It is logged loudly instead.
		s.logger.ErrorContext(ctx, "settings: audit write failed",
			slog.String("key", in.Key), slog.String("error", err.Error()))
	}
	st.Kind = d.Kind
	return *st, nil
}

// History returns a page of a setting's changes, newest first.
func (s *Service) History(ctx context.Context, key string, limit int, before *Cursor) ([]Change, error) {
	if _, ok := DefinitionFor(key); !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	if limit <= 0 {
		limit = 50
	}
	return s.store.History(ctx, key, limit, before)
}

func (s *Service) writeAudit(ctx context.Context, chg *Change) error {
	if s.audit == nil || chg == nil {
		return nil
	}
	actor := "system"
	actorType := audit.ActorSystem
	if chg.ChangedBy != nil {
		actor = chg.ChangedBy.String()
		actorType = audit.ActorUser
	}
	return s.audit.Write(ctx, nil, audit.Event{
		ActorType:  actorType,
		ActorID:    actor,
		Action:     "core.setting.updated",
		EntityType: "setting",
		Before:     map[string]any{"version": chg.Version - 1, "value": chg.OldValue},
		After:      map[string]any{"key": chg.Key, "version": chg.Version, "value": chg.NewValue},
		Reason:     chg.Reason,
	})
}
