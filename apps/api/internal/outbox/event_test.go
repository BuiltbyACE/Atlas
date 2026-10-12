package outbox

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestNewHello(t *testing.T) {
	ev, err := NewHello(nil, nil, "hello world")
	if err != nil {
		t.Fatalf("NewHello() error = %v", err)
	}

	if ev.Type != TypeSystemHelloV1 {
		t.Errorf("type = %q, want %q", ev.Type, TypeSystemHelloV1)
	}
	if ev.AggregateType != AggregateSystem {
		t.Errorf("aggregate_type = %q, want %q", ev.AggregateType, AggregateSystem)
	}
	if ev.ID == uuid.Nil {
		t.Error("event id must be set")
	}
	if v := ev.ID.Version(); v != 7 {
		t.Errorf("event id version = %d, want 7 (UUIDv7 gives time-ordered ids)", v)
	}
	if ev.OccurredAt.IsZero() {
		t.Error("occurred_at must be set")
	}
	if len(ev.Payload) == 0 {
		t.Fatal("payload must be set")
	}

	var p HelloPayload
	if err := ev.DecodePayload(&p); err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	if p.Message != "hello world" {
		t.Errorf("payload message = %q, want %q", p.Message, "hello world")
	}
	if p.GreetedAt.IsZero() {
		t.Error("payload greeted_at must be set")
	}
}

func TestWriteRejectsIncompleteEvent(t *testing.T) {
	// Write validates before touching the database, so a nil Execer is safe here.
	if _, err := Write(context.Background(), nil, Event{}); err == nil {
		t.Fatal("Write() should reject an event with no id/type/payload")
	}
}
