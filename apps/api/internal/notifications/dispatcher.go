package notifications

import (
	"context"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// DispatcherConfig tunes the delivery engine. Every value is configuration,
// not code (§2.10).
type DispatcherConfig struct {
	// BatchSize is how many due notifications one poll claims.
	BatchSize int
	// PerProvider is the send concurrency allowed per channel — the per-provider
	// rate limit (§8.10). One slot is one in-flight send.
	PerProvider int
	// PollInterval is how often the dispatcher scans for due work. The §11 SLO
	// for reminders is 60 s, so a sub-second poll leaves ample headroom.
	PollInterval time.Duration
	// SendTimeout bounds a single provider call.
	SendTimeout time.Duration
	// Lease is how long a claim is held before another worker may retry it.
	Lease time.Duration
	// BaseBackoff and MaxBackoff bound the exponential retry delay.
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	// BreakerThreshold consecutive failures open a provider's circuit;
	// BreakerCooldown is how long it stays open before a probe is allowed.
	BreakerThreshold int
	BreakerCooldown  time.Duration
}

// DefaultDispatcherConfig returns production-sane defaults.
func DefaultDispatcherConfig() DispatcherConfig {
	return DispatcherConfig{
		BatchSize:        100,
		PerProvider:      8,
		PollInterval:     time.Second,
		SendTimeout:      10 * time.Second,
		Lease:            2 * time.Minute,
		BaseBackoff:      time.Second,
		MaxBackoff:       5 * time.Minute,
		BreakerThreshold: 5,
		BreakerCooldown:  30 * time.Second,
	}
}

// Dispatcher delivers due notifications through channel adapters. It is the
// only place a provider is called, and it runs in atlas-worker, never on a
// request path.
type Dispatcher struct {
	store   Store
	senders map[Channel]Sender
	cfg     DispatcherConfig
	logger  *slog.Logger
	now     func() time.Time
	worker  string

	slots   map[Channel]chan struct{}
	breaker map[Channel]*breaker
}

// NewDispatcher builds a dispatcher for the given senders.
func NewDispatcher(store Store, senders []Sender, cfg DispatcherConfig, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.BatchSize <= 0 {
		cfg = DefaultDispatcherConfig()
	}
	d := &Dispatcher{
		store:   store,
		senders: make(map[Channel]Sender, len(senders)),
		cfg:     cfg,
		logger:  logger,
		now:     func() time.Time { return time.Now().UTC() },
		worker:  workerID(),
		slots:   make(map[Channel]chan struct{}, len(senders)),
		breaker: make(map[Channel]*breaker, len(senders)),
	}
	for _, s := range senders {
		ch := s.Channel()
		d.senders[ch] = s
		d.slots[ch] = make(chan struct{}, max(1, cfg.PerProvider))
		d.breaker[ch] = newBreaker(cfg.BreakerThreshold, cfg.BreakerCooldown)
	}
	return d
}

// Run drives the dispatcher until ctx is cancelled.
func (d *Dispatcher) Run(ctx context.Context) error {
	d.logger.Info("notification dispatcher ready",
		slog.String("worker", d.worker),
		slog.Duration("poll", d.cfg.PollInterval),
		slog.Int("batch", d.cfg.BatchSize),
		slog.Int("per_provider", d.cfg.PerProvider),
		slog.Any("channels", d.channels()))

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if err := d.drain(ctx); err != nil {
			d.logger.Warn("notification drain", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			d.logger.Info("notification dispatcher stopping")
			return nil
		case <-ticker.C:
		}
	}
}

// drain claims one batch and delivers it concurrently, bounded per channel.
func (d *Dispatcher) drain(ctx context.Context) error {
	ns, err := d.store.ClaimDue(ctx, d.worker, d.cfg.Lease, d.cfg.BatchSize)
	if err != nil {
		return err
	}
	if len(ns) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	for i := range ns {
		n := ns[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.deliver(ctx, n)
		}()
	}
	wg.Wait()
	return nil
}

func (d *Dispatcher) deliver(ctx context.Context, n Notification) {
	slot, ok := d.slots[n.Channel]
	if !ok {
		// No adapter ships for this channel yet (whatsapp/sms in Phase 1). Fail
		// it rather than let it spin forever.
		d.complete(ctx, n, Outcome{Err: "no adapter for channel " + string(n.Channel)}, nil)
		return
	}

	slot <- struct{}{}
	defer func() { <-slot }()

	now := d.now()
	br := d.breaker[n.Channel]
	if br != nil && br.isOpen(now) {
		// Circuit open: push it past the cooldown without spending an attempt.
		_ = d.store.Defer(ctx, n.ID, br.openUntil(now))
		return
	}

	sender := d.senders[n.Channel]
	sendCtx, cancel := context.WithTimeout(ctx, d.cfg.SendTimeout)
	defer cancel()

	ref, err := sender.Send(sendCtx, Message{
		To:             n.RecipientEmail,
		Subject:        n.Subject,
		Body:           n.Body,
		IdempotencyKey: idempotencyKey(n),
	})
	if err == nil {
		if br != nil {
			br.recordSuccess()
		}
		ev, _ := outbox.NewNotificationSent(nil, nil, n.RecipientPersonID, n.ID, sender.Provider())
		d.complete(ctx, n, Outcome{Success: true, Provider: sender.Provider(), ProviderRef: ref}, &ev)
		return
	}

	if br != nil {
		br.recordFailure(now)
	}
	attempt := n.Attempts + 1
	if attempt >= n.MaxAttempts {
		ev, _ := outbox.NewNotificationFailed(nil, nil, n.RecipientPersonID, n.ID, err.Error())
		d.complete(ctx, n, Outcome{Provider: sender.Provider(), Err: err.Error()}, &ev)
		return
	}

	next := now.Add(backoff(d.cfg, attempt))
	d.complete(ctx, n, Outcome{Provider: sender.Provider(), Err: err.Error(), NextAttempt: &next}, nil)
}

// complete persists the outcome using a context that survives the parent's
// cancellation, so a shutdown never loses the record of an attempt already made.
func (d *Dispatcher) complete(ctx context.Context, n Notification, out Outcome, ev *outbox.Event) {
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := d.store.Complete(cctx, n, out, ev); err != nil {
		d.logger.Error("notification completion failed",
			slog.String("error", err.Error()),
			slog.String("notification_id", n.ID.String()))
	}
}

func (d *Dispatcher) channels() []string {
	out := make([]string, 0, len(d.senders))
	for ch := range d.senders {
		out = append(out, string(ch))
	}
	sort.Strings(out)
	return out
}

// backoff returns the retry delay for an attempt: exponential, capped, with
// jitter so a provider recovering after an outage is not stampeded.
func backoff(cfg DispatcherConfig, attempt int) time.Duration {
	base := cfg.BaseBackoff
	if base <= 0 {
		base = time.Second
	}
	maxDelay := cfg.MaxBackoff
	if maxDelay <= 0 {
		maxDelay = 5 * time.Minute
	}
	delay := float64(base) * math.Pow(2, float64(attempt-1))
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}
	jitter := rand.Float64() * delay * 0.2
	return time.Duration(delay + jitter)
}

func idempotencyKey(n Notification) string {
	if n.IdempotencyKey != "" {
		return n.IdempotencyKey + ":" + string(n.Channel)
	}
	return n.ID.String()
}

func workerID() string {
	id, err := os.Hostname()
	if err != nil || id == "" {
		id = "atlas-worker"
	}
	return id
}

// breaker is a per-provider circuit breaker (§8.10).
type breaker struct {
	mu        sync.Mutex
	failures  int
	openedAt  time.Time
	threshold int
	cooldown  time.Duration
}

func newBreaker(threshold int, cooldown time.Duration) *breaker {
	if threshold < 1 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &breaker{threshold: threshold, cooldown: cooldown}
}

func (b *breaker) isOpen(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openedAt.IsZero() {
		return false
	}
	if now.Sub(b.openedAt) >= b.cooldown {
		b.openedAt = time.Time{}
		b.failures = 0
		return false
	}
	return true
}

func (b *breaker) openUntil(now time.Time) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.openedAt.Add(b.cooldown)
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openedAt = time.Time{}
}

func (b *breaker) recordFailure(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.threshold && b.openedAt.IsZero() {
		b.openedAt = now
	}
}
