package contracts

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustTime(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, time.October, 19, 10, 0, 0, 0, time.UTC)
}

// TestContractTransitions walks the §8.3 lifecycle diagram: draft → internal
// review → approved → sent → signed → active → (renewal due → renewed) /
// completed / terminated.
func TestContractTransitions(t *testing.T) {
	cases := []struct {
		from, to ContractStatus
		want     bool
	}{
		{StatusDraft, StatusInternalReview, true},
		{StatusDraft, StatusApproved, false},
		{StatusInternalReview, StatusApproved, true},
		{StatusInternalReview, StatusDraft, true},
		{StatusApproved, StatusSent, true},
		{StatusSent, StatusSigned, true},
		{StatusSigned, StatusActive, true},
		{StatusActive, StatusRenewalDue, true},
		{StatusActive, StatusCompleted, true},
		{StatusActive, StatusTerminated, true},
		{StatusRenewalDue, StatusRenewed, true},
		{StatusRenewalDue, StatusTerminated, true},
		{StatusCompleted, StatusActive, false},
		{StatusTerminated, StatusActive, false},
		{StatusSent, StatusActive, false},
	}
	for _, c := range cases {
		if got := allowedContractTransition(c.from, c.to); got != c.want {
			t.Errorf("allowedContractTransition(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestValidateContract(t *testing.T) {
	svc := &Service{}
	org := uuid.Must(uuid.NewV7())

	// A well-formed draft validates.
	if err := svc.validateContract(ContractInput{
		OrganizationID: org, ContractType: TypeFixedPriceProject, Currency: "KES",
	}, false); err != nil {
		t.Fatalf("valid contract: %v", err)
	}

	// Currency is normalised, so a lowercase code is accepted and the empty code
	// falls back to KES.
	if err := svc.validateContract(ContractInput{ContractType: TypeRetainer, Currency: "kes"}, false); err != nil {
		t.Fatalf("lowercase currency: %v", err)
	}
	if err := svc.validateContract(ContractInput{ContractType: TypeRetainer, Currency: ""}, false); err != nil {
		t.Fatalf("default currency: %v", err)
	}
	if err := svc.validateContract(ContractInput{ContractType: TypeRetainer, Currency: "KE"}, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("short currency: err = %v, want ErrInvalid", err)
	}

	// An unknown contract type is rejected.
	if err := svc.validateContract(ContractInput{ContractType: "lease", Currency: "KES"}, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown type: err = %v, want ErrInvalid", err)
	}

	// A negative value is rejected.
	neg := int64(-1)
	if err := svc.validateContract(ContractInput{ContractType: TypeRetainer, ValueMinor: &neg}, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative value: err = %v, want ErrInvalid", err)
	}

	// The end date cannot precede the start.
	start := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	if err := svc.validateContract(ContractInput{
		ContractType: TypeRetainer, StartAt: &start, EndAt: &end,
	}, false); !errors.Is(err, ErrInvalid) {
		t.Fatalf("reversed dates: err = %v, want ErrInvalid", err)
	}

	// requireOrg demands a non-nil organization.
	if err := svc.validateContract(ContractInput{ContractType: TypeRetainer}, true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing org: err = %v, want ErrInvalid", err)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	at := mustTime(t)
	id := uuid.Must(uuid.NewV7())

	encoded := encodeCursor(at, id)
	got, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if got.ID != id {
		t.Fatalf("cursor id = %s, want %s", got.ID, id)
	}
	if !got.At.Equal(at) {
		t.Fatalf("cursor at = %s, want %s", got.At, at)
	}

	if _, err := decodeCursor("not-a-cursor"); err == nil {
		t.Fatal("malformed cursor should not decode")
	}
}

func TestFieldErrorMapsToInvalid(t *testing.T) {
	if !errors.Is(errInvalidField("start_at"), ErrInvalid) {
		t.Fatal("a field error should satisfy errors.Is(ErrInvalid)")
	}
}
