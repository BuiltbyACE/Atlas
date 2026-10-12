package notifications

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBackoffGrowsAndCaps(t *testing.T) {
	cfg := DispatcherConfig{BaseBackoff: 2 * time.Second, MaxBackoff: 30 * time.Second}

	first := backoff(cfg, 1)
	if first < 2*time.Second || first > 2400*time.Millisecond {
		t.Fatalf("first backoff = %s, want within [2s, 2.4s]", first)
	}

	second := backoff(cfg, 2)
	if second <= first {
		t.Fatalf("backoff did not grow: %s then %s", first, second)
	}

	capped := backoff(cfg, 20)
	if capped > 36*time.Second {
		t.Fatalf("backoff = %s, exceeds the cap plus jitter", capped)
	}
}

func TestBreakerOpensAndRecovers(t *testing.T) {
	now := time.Now()
	b := newBreaker(2, time.Minute)

	if b.isOpen(now) {
		t.Fatal("a fresh breaker must be closed")
	}
	b.recordFailure(now)
	if b.isOpen(now) {
		t.Fatal("one failure must not open the circuit at threshold 2")
	}
	b.recordFailure(now)
	if !b.isOpen(now) {
		t.Fatal("the circuit must open at the threshold")
	}

	// After the cooldown it closes again.
	if b.isOpen(now.Add(time.Minute + time.Second)) {
		t.Fatal("the circuit must close after its cooldown")
	}

	// A success resets the count.
	b.recordFailure(now)
	b.recordSuccess()
	b.recordFailure(now)
	if b.isOpen(now) {
		t.Fatal("a success must reset the failure count")
	}
}

func TestIdempotencyKeyIsStable(t *testing.T) {
	id := uuid.Must(uuid.NewV7())

	withKey := Notification{ID: id, Channel: ChannelEmail, IdempotencyKey: "task-42"}
	if got := idempotencyKey(withKey); got != "task-42:email" {
		t.Fatalf("idempotencyKey = %q, want %q", got, "task-42:email")
	}

	withoutKey := Notification{ID: id, Channel: ChannelEmail}
	if got := idempotencyKey(withoutKey); got != id.String() {
		t.Fatalf("idempotencyKey = %q, want the notification id %q", got, id)
	}
}
