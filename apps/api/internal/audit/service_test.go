package audit

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestServiceHistoryRequiresEntityType(t *testing.T) {
	svc := NewService(&fakeStore{}, nil)
	if _, err := svc.History(context.Background(), "", uuid.New(), 10, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestServiceListAppliesDefaultLimit(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, nil)
	if _, err := svc.List(context.Background(), Query{}, 0, nil); err != nil {
		t.Fatalf("List: %v", err)
	}
	if store.lastLimit != 50 {
		t.Fatalf("store limit = %d, want default 50", store.lastLimit)
	}
}

func TestServiceHistoryNarrowsToEntity(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, nil)
	id := uuid.New()
	if _, err := svc.History(context.Background(), "person", id, 10, nil); err != nil {
		t.Fatalf("History: %v", err)
	}
	if store.lastQuery.EntityType != "person" || store.lastQuery.EntityID == nil || *store.lastQuery.EntityID != id {
		t.Fatalf("query = %+v, want the entity filter", store.lastQuery)
	}
}
