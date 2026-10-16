package settings

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

// fakeStore satisfies Store in memory so the service's own logic (defaults,
// validation, audit) can be tested without a database.
type fakeStore struct {
	list      []Setting
	get       *Setting
	setResult *Setting
	setChange *Change
	setErr    error
	setCalls  []SetInput
}

func (f *fakeStore) List(context.Context) ([]Setting, error) { return f.list, nil }

func (f *fakeStore) Get(context.Context, string) (*Setting, error) { return f.get, nil }

func (f *fakeStore) Set(_ context.Context, in SetInput) (*Setting, *Change, error) {
	f.setCalls = append(f.setCalls, in)
	if f.setErr != nil {
		return nil, nil, f.setErr
	}
	return f.setResult, f.setChange, nil
}

func (f *fakeStore) History(context.Context, string, int, *Cursor) ([]Change, error) { return nil, nil }

// fakeExecer captures the audit writer's INSERT when it runs without a tx.
type fakeExecer struct {
	execs int
}

func (f *fakeExecer) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	f.execs++
	return pgconn.CommandTag{}, nil
}

func TestDefinitionValidate(t *testing.T) {
	tests := []struct {
		name  string
		def   Definition
		value any
		ok    bool
	}{
		{"string ok", Definition{Key: "k", Kind: KindString}, "hello", true},
		{"string blank", Definition{Key: "k", Kind: KindString}, "  ", false},
		{"string wrong type", Definition{Key: "k", Kind: KindString}, 3, false},
		{"string too long", Definition{Key: "k", Kind: KindString, MaxLength: 2}, "abc", false},
		{"bool ok", Definition{Key: "k", Kind: KindBool}, true, true},
		{"bool wrong", Definition{Key: "k", Kind: KindBool}, "true", false},
		{"int ok", Definition{Key: "k", Kind: KindInt}, float64(24), true},
		{"int fractional", Definition{Key: "k", Kind: KindInt}, 1.5, false},
		{"int wrong", Definition{Key: "k", Kind: KindInt}, "24", false},
		{"enum ok", Definition{Key: "k", Kind: KindEnum, Enum: []string{"KES", "USD"}}, "USD", true},
		{"enum wrong", Definition{Key: "k", Kind: KindEnum, Enum: []string{"KES"}}, "EUR", false},
		{"list ok", Definition{Key: "k", Kind: KindStringList}, []any{"a", "b"}, true},
		{"list bad item", Definition{Key: "k", Kind: KindStringList}, []any{"a", 1}, false},
		{"list wrong shape", Definition{Key: "k", Kind: KindStringList}, "a,b", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.def.Validate(tc.value)
			if tc.ok && err != nil {
				t.Fatalf("Validate(%v) = %v, want nil", tc.value, err)
			}
			if !tc.ok && !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate(%v) = %v, want ErrInvalid", tc.value, err)
			}
		})
	}
}

func TestDefinitionsAreSortedAndResolvable(t *testing.T) {
	defs := Definitions()
	if len(defs) == 0 {
		t.Fatal("no definitions")
	}
	if !sort.SliceIsSorted(defs, func(i, j int) bool { return defs[i].Key < defs[j].Key }) {
		t.Fatal("Definitions() is not sorted by key")
	}
	for _, d := range defs {
		got, ok := DefinitionFor(d.Key)
		if !ok || got.Key != d.Key {
			t.Fatalf("DefinitionFor(%q) = %v, %v", d.Key, got, ok)
		}
		if d.Description == "" {
			t.Fatalf("definition %q has no description", d.Key)
		}
	}
	if _, ok := DefinitionFor("does.not.exist"); ok {
		t.Fatal("DefinitionFor returned an unknown key")
	}
}

func TestServiceListFillsDefaults(t *testing.T) {
	store := &fakeStore{list: []Setting{{Key: "company.currency", Value: "USD", Version: 3}}}
	svc := NewService(store, nil, nil)

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	byKey := map[string]Setting{}
	for _, s := range got {
		byKey[s.Key] = s
	}

	cur, ok := byKey["company.currency"]
	if !ok {
		t.Fatal("List: company.currency missing")
	}
	if cur.Value != "USD" || cur.Version != 3 || cur.Kind != KindEnum {
		t.Fatalf("stored setting not merged through the definition: %+v", cur)
	}

	name := byKey["company.name"]
	if name.Value != "Atlas" || name.Version != 0 {
		t.Fatalf("default not applied: %+v", name)
	}
}

func TestServiceSetRejects(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store, nil, nil)
	actor := uuid.New()

	if _, err := svc.Set(context.Background(), SetInput{Key: "does.not.exist", Value: true, ActorID: actor}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown key: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Set(context.Background(), SetInput{Key: "company.currency", Value: "EUR", ActorID: actor}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad value: err = %v, want ErrInvalid", err)
	}
	if len(store.setCalls) != 0 {
		t.Fatalf("store was written %d times for rejected input", len(store.setCalls))
	}
}

func TestServiceSetAuditsAndDescribes(t *testing.T) {
	actor := uuid.New()
	store := &fakeStore{
		setResult: &Setting{Key: "company.name", Value: "Acme", Version: 1},
		setChange: &Change{Key: "company.name", Version: 1, OldValue: "Atlas", NewValue: "Acme", ChangedBy: &actor},
	}
	exec := &fakeExecer{}
	svc := NewService(store, audit.NewWriter(exec), nil)

	got, err := svc.Set(context.Background(), SetInput{Key: "company.name", Value: "Acme", ActorID: actor, Reason: "rebrand"})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got.Kind != KindString {
		t.Fatalf("returned setting kind = %q, want string", got.Kind)
	}
	if len(store.setCalls) != 1 {
		t.Fatalf("store.Set calls = %d, want 1", len(store.setCalls))
	}
	if store.setCalls[0].Description == "" {
		t.Fatal("service did not pass the definition description to the store")
	}
	if exec.execs != 1 {
		t.Fatalf("audit writes = %d, want 1", exec.execs)
	}
}
