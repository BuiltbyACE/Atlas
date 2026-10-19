package crm

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustUUID(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new uuid: %v", err)
	}
	return id
}

func mustTime(t *testing.T) time.Time {
	t.Helper()
	return time.Now().UTC().Add(24 * time.Hour)
}

// TestLeadTransitions exercises the lead lifecycle including the open-state rule
// (§8.1): new leads are unowned; every other open state requires an owner and a
// dated next action, and `converted` is reached only through ConvertLead.
func TestLeadTransitions(t *testing.T) {
	cases := []struct {
		from, to LeadStatus
		want     bool
	}{
		{LeadNew, LeadAssigned, true},
		{LeadNew, LeadDisqualified, true},
		{LeadNew, LeadQualified, false},
		{LeadAssigned, LeadContacted, true},
		{LeadContacted, LeadQualified, true},
		{LeadContacted, LeadNurture, true},
		{LeadNurture, LeadContacted, true},
		{LeadNurture, LeadQualified, false},
		{LeadQualified, LeadConverted, true},
		{LeadDisqualified, LeadAssigned, false},
		{LeadConverted, LeadAssigned, false},
		{LeadConverted, LeadNew, false},
	}
	for _, c := range cases {
		if got := allowedLeadTransition(c.from, c.to); got != c.want {
			t.Errorf("allowedLeadTransition(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestOpenLead(t *testing.T) {
	open := []LeadStatus{LeadAssigned, LeadContacted, LeadQualified, LeadNurture}
	for _, s := range open {
		if !openLead(s) {
			t.Errorf("openLead(%s) = false, want true", s)
		}
	}
	closed := []LeadStatus{LeadNew, LeadDisqualified, LeadConverted}
	for _, s := range closed {
		if openLead(s) {
			t.Errorf("openLead(%s) = true, want false", s)
		}
	}
}

func TestValidateLead(t *testing.T) {
	owner := mustUUID(t)
	due := mustTime(t)

	// A brand-new lead needs a source but no owner and no next action.
	if _, err := validateLead(Lead{SourceKey: "website_form", Status: LeadNew}); err != nil {
		t.Fatalf("new lead: %v", err)
	}

	// An open lead must name an owner and a dated next action.
	if _, err := validateLead(Lead{SourceKey: "website_form", Status: LeadAssigned}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("assigned without owner: err = %v, want ErrInvalid", err)
	}
	if _, err := validateLead(Lead{SourceKey: "website_form", Status: LeadAssigned, OwnerPersonID: &owner}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("assigned without next action: err = %v, want ErrInvalid", err)
	}
	if _, err := validateLead(Lead{
		SourceKey: "website_form", Status: LeadAssigned,
		OwnerPersonID: &owner, NextAction: "Call back", NextActionDueAt: &due,
	}); err != nil {
		t.Fatalf("assigned with owner and next action: %v", err)
	}

	// A source is required, and it is trimmed.
	if _, err := validateLead(Lead{Status: LeadNew}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing source: err = %v, want ErrInvalid", err)
	}
	got, err := validateLead(Lead{SourceKey: "  website_form  ", Status: LeadNew})
	if err != nil {
		t.Fatalf("trim: %v", err)
	}
	if got.SourceKey != "website_form" {
		t.Fatalf("source = %q, want %q", got.SourceKey, "website_form")
	}

	// A disqualified lead needs a lost reason.
	if _, err := validateLead(Lead{SourceKey: "website_form", Status: LeadDisqualified}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("disqualified without reason: err = %v, want ErrInvalid", err)
	}
	reason := LostPrice
	if _, err := validateLead(Lead{SourceKey: "website_form", Status: LeadDisqualified, LostReason: &reason}); err != nil {
		t.Fatalf("disqualified with reason: %v", err)
	}

	// The estimated value cannot be negative.
	neg := int64(-1)
	if _, err := validateLead(Lead{SourceKey: "website_form", Status: LeadNew, EstimatedValueMinor: &neg}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative value: err = %v, want ErrInvalid", err)
	}
}

func TestValidateOrganizationAndContact(t *testing.T) {
	if _, err := validateOrganization(Organization{Name: "  Acme  "}); err != nil {
		t.Fatalf("organization: %v", err)
	}
	if _, err := validateOrganization(Organization{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nameless organization: err = %v, want ErrInvalid", err)
	}

	if _, err := validateContact(Contact{Name: "Jane"}); err != nil {
		t.Fatalf("contact: %v", err)
	}
	if _, err := validateContact(Contact{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nameless contact: err = %v, want ErrInvalid", err)
	}
}

func TestValidateInteractionRequiresOutcome(t *testing.T) {
	lead := mustUUID(t)
	// The outcome is the point of an interaction; an empty one is rejected (§8.1).
	if _, err := validateInteraction(Interaction{Type: InteractionCall, LeadID: &lead}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty outcome: err = %v, want ErrInvalid", err)
	}
	if _, err := validateInteraction(Interaction{Type: InteractionCall, LeadID: &lead, Outcome: "Left a voicemail"}); err != nil {
		t.Fatalf("interaction: %v", err)
	}
	// The interaction must hang off something.
	if _, err := validateInteraction(Interaction{Type: InteractionCall, Outcome: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unlinked interaction: err = %v, want ErrInvalid", err)
	}
}

func TestValidateProposal(t *testing.T) {
	opp := mustUUID(t)
	if _, err := validateProposal(Proposal{OpportunityID: opp, PriceMinor: 1000}); err != nil {
		t.Fatalf("proposal: %v", err)
	}
	if _, err := validateProposal(Proposal{OpportunityID: opp, PriceMinor: -1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative price: err = %v, want ErrInvalid", err)
	}
	if _, err := validateProposal(Proposal{OpportunityID: opp, PriceMinor: 100, DiscountMinor: 200}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("discount over price: err = %v, want ErrInvalid", err)
	}
}
