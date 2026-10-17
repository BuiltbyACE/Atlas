// Package settings owns Atlas's typed configuration store (docs/ATLAS.md §6.7).
//
// A setting is a well-known key with a JSON value, a description and a version.
// The keys and their types are declared in code, so a value the system does not
// understand cannot be written: the API validates against the definition before
// it touches the database.
//
// Every change is versioned and audited (§6.6, §6.7). The current value lives
// in core.settings.version; each write appends to core.setting_changes with the
// value it replaced, so a setting's history is readable long after the current
// value has moved on.
package settings

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	// ErrNotFound marks an unknown setting key.
	ErrNotFound = errors.New("settings: not found")
	// ErrInvalid marks a value that does not match its definition.
	ErrInvalid = errors.New("settings: invalid")
	// ErrConflict marks a stale write: the value changed (or appeared) since
	// the caller last read it.
	ErrConflict = errors.New("settings: conflict")
)

// Kind is the JSON shape a setting's value must take.
type Kind string

// The value kinds the store understands. Keep this list small and explicit:
// each kind is a validation rule the API applies before a write.
const (
	KindString     Kind = "string"
	KindBool       Kind = "bool"
	KindInt        Kind = "int"
	KindEnum       Kind = "enum"
	KindStringList Kind = "string_list"
)

// Definition declares one setting: its key, its type, and the default served
// when no row exists yet.
type Definition struct {
	Key         string
	Kind        Kind
	Description string
	// Enum lists the accepted values for KindEnum.
	Enum []string
	// MaxLength bounds KindString values (0 = unbounded).
	MaxLength int
	// Min and Max bound KindInt values (nil = unbounded).
	Min     *float64
	Max     *float64
	Default any
}

// Validate reports whether value conforms to the definition. Values arrive
// from JSON, so numbers are float64 and lists are []any.
func (d Definition) Validate(value any) error {
	switch d.Kind {
	case KindBool:
		if _, ok := value.(bool); !ok {
			return d.invalid("a boolean")
		}
	case KindInt:
		var n float64
		switch v := value.(type) {
		case float64:
			if v != float64(int64(v)) {
				return d.invalid("an integer")
			}
			n = v
		case int:
			n = float64(v)
		case int32:
			n = float64(v)
		case int64:
			n = float64(v)
		default:
			return d.invalid("an integer")
		}
		if d.Min != nil && n < *d.Min {
			return d.invalid(fmt.Sprintf("an integer of at least %g", *d.Min))
		}
		if d.Max != nil && n > *d.Max {
			return d.invalid(fmt.Sprintf("an integer of at most %g", *d.Max))
		}
	case KindEnum:
		s, ok := value.(string)
		if !ok || !contains(d.Enum, s) {
			return d.invalid("one of " + strings.Join(d.Enum, ", "))
		}
	case KindStringList:
		arr, ok := value.([]any)
		if !ok {
			return d.invalid("a list of strings")
		}
		for _, item := range arr {
			if s, ok := item.(string); !ok || strings.TrimSpace(s) == "" {
				return d.invalid("a list of non-empty strings")
			}
		}
	case KindString:
		fallthrough
	default:
		s, ok := value.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return d.invalid("a non-empty string")
		}
		if d.MaxLength > 0 && len(s) > d.MaxLength {
			return d.invalid(fmt.Sprintf("a string of at most %d characters", d.MaxLength))
		}
	}
	return nil
}

func (d Definition) invalid(want string) error {
	return fmt.Errorf("%w: %s must be %s", ErrInvalid, d.Key, want)
}

func contains(xs []string, x string) bool {
	for _, candidate := range xs {
		if candidate == x {
			return true
		}
	}
	return false
}

// definitions is the closed catalogue of settings. It is code — reviewable and
// version-controlled — so adding a setting is a deliberate act, not a side
// effect of a client writing an arbitrary key.
var definitions = []Definition{
	{
		Key:         "company.name",
		Kind:        KindString,
		Description: "The legal name shown across Atlas and on documents.",
		MaxLength:   120,
		Default:     "Atlas",
	},
	{
		Key:         "company.currency",
		Kind:        KindEnum,
		Enum:        []string{"KES", "USD"},
		Description: "The reporting currency for money values stored in minor units.",
		Default:     "KES",
	},
	{
		Key:         "company.timezone",
		Kind:        KindString,
		Description: "The display timezone; values are stored in UTC and shown in this zone.",
		MaxLength:   64,
		Default:     "Africa/Nairobi",
	},
	{
		Key:         "notifications.email.enabled",
		Kind:        KindBool,
		Description: "Whether Atlas may deliver notifications by email.",
		Default:     true,
	},
	{
		Key:         "notifications.digest.enabled",
		Kind:        KindBool,
		Description: "Whether a daily summary is sent to each person.",
		Default:     true,
	},
	{
		Key:         "approvals.reminder_hours",
		Kind:        KindInt,
		Description: "How long an approval may sit unresolved before a reminder is sent.",
		Default:     24.0,
	},
	// Decision D4 (docs/adr/ADR-016): the approval thresholds. These record the
	// decided numbers; the approvals engine only applies them once the policy
	// change workflow (§6.7) can express the approver rules of §3.3.
	{
		Key:         "approvals.t1_minor",
		Kind:        KindInt,
		Description: "D4 threshold T1: below this amount (minor units) an expense is approved by the project lead / line manager.",
		Min:         floatPtr(0),
		Default:     1000000.0, // KES 10,000
	},
	{
		Key:         "approvals.t2_minor",
		Kind:        KindInt,
		Description: "D4 threshold T2: above this amount (minor units) expenses and purchases need both CEOs.",
		Min:         floatPtr(0),
		Default:     10000000.0, // KES 100,000
	},
	{
		Key:         "approvals.discount_max_pct",
		Kind:        KindInt,
		Description: "D4 threshold D1: maximum proposal discount (%) the Commercial CEO may approve alone.",
		Min:         floatPtr(0),
		Max:         floatPtr(100),
		Default:     10.0,
	},
	{
		Key:         "approvals.contract_ceo_min_minor",
		Kind:        KindInt,
		Description: "D4 threshold C2: contract value (minor units) above which both CEOs must sign.",
		Min:         floatPtr(0),
		Default:     50000000.0, // KES 500,000
	},
	// Decision D4 (docs/adr/ADR-016): the escalation ladder's timing windows,
	// read by the overdue-task sweep on every pass (§5.8).
	{
		Key:         "escalation.e1_hours",
		Kind:        KindInt,
		Description: "E1: hours a task may be overdue before the sweep raises L1 to the project owner.",
		Min:         floatPtr(0),
		Max:         floatPtr(8760),
		Default:     24.0,
	},
	{
		Key:         "escalation.e2_hours",
		Kind:        KindInt,
		Description: "E2: hours a task may be overdue before the sweep raises L2 to the domain CEO (escalation.l2_role).",
		Min:         floatPtr(0),
		Max:         floatPtr(8760),
		Default:     48.0,
	},
	{
		Key:         "escalation.e3_hours",
		Kind:        KindInt,
		Description: "E3: hours an L2 may sit unresolved before the sweep raises L3 to both CEOs' shared queue.",
		Min:         floatPtr(0),
		Max:         floatPtr(8760),
		Default:     48.0,
	},
	{
		Key:         "escalation.l2_role",
		Kind:        KindString,
		Description: "Role key escalated to at L2 — the \"domain CEO\" of §3.4; L3 is the shared both-CEO queue.",
		MaxLength:   64,
		Default:     "ceo_internal",
	},
}

// floatPtr is a tiny helper so the bounded definitions above stay readable.
func floatPtr(v float64) *float64 { return &v }

// byKey indexes the catalogue for lookup.
var byKey = func() map[string]Definition {
	m := make(map[string]Definition, len(definitions))
	for _, d := range definitions {
		if _, dup := m[d.Key]; dup {
			panic("settings: duplicate definition " + d.Key)
		}
		m[d.Key] = d
	}
	return m
}()

// DefinitionFor returns the definition for a key, if there is one.
func DefinitionFor(key string) (Definition, bool) {
	d, ok := byKey[key]
	return d, ok
}

// Definitions returns the catalogue, ordered by key.
func Definitions() []Definition {
	out := make([]Definition, len(definitions))
	copy(out, definitions)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Setting is a key with its current value.
type Setting struct {
	Key         string
	Value       any
	Description string
	Kind        Kind
	// Version is 0 when the value is still the definition default and no row
	// has been written yet.
	Version   int64
	UpdatedBy *uuid.UUID
	UpdatedAt *time.Time
}

// Change is one entry in a setting's history.
type Change struct {
	ID        uuid.UUID
	Key       string
	Version   int64
	OldValue  any
	NewValue  any
	ChangedBy *uuid.UUID
	ChangedAt time.Time
	Reason    string
}

// SetInput is one write. ExpectedVersion is the version the caller read
// (If-Match); 0 means "must not exist yet".
type SetInput struct {
	Key             string
	Value           any
	Description     string
	ExpectedVersion int64
	ActorID         uuid.UUID
	Reason          string
}
