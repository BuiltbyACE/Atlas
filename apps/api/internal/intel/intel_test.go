package intel

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidDecisionOutcome(t *testing.T) {
	if !validDecisionOutcome(DecisionOpen) || !validDecisionOutcome(DecisionApproved) ||
		!validDecisionOutcome(DecisionRejected) || !validDecisionOutcome(DecisionDeferred) {
		t.Fatal("validDecisionOutcome rejected a valid outcome")
	}
	if validDecisionOutcome("bogus") {
		t.Fatal("validDecisionOutcome accepted an invalid outcome")
	}
}

func TestValidRiskKind(t *testing.T) {
	if !validRiskKind(RiskClient) || !validRiskKind(RiskDelivery) ||
		!validRiskKind(RiskFinancial) || !validRiskKind(RiskPeople) || !validRiskKind(RiskSystems) {
		t.Fatal("validRiskKind rejected a valid kind")
	}
	if validRiskKind("bogus") {
		t.Fatal("validRiskKind accepted an invalid kind")
	}
}

func TestValidRiskLikelihood(t *testing.T) {
	if !validRiskLikelihood(LikelihoodLow) || !validRiskLikelihood(LikelihoodMedium) ||
		!validRiskLikelihood(LikelihoodHigh) || !validRiskLikelihood(LikelihoodCritical) {
		t.Fatal("validRiskLikelihood rejected a valid likelihood")
	}
}

func TestValidRiskImpact(t *testing.T) {
	if !validRiskImpact(ImpactLow) || !validRiskImpact(ImpactMedium) ||
		!validRiskImpact(ImpactHigh) || !validRiskImpact(ImpactCritical) {
		t.Fatal("validRiskImpact rejected a valid impact")
	}
}

func TestValidRiskStatus(t *testing.T) {
	if !validRiskStatus(RiskOpen) || !validRiskStatus(RiskMonitoring) ||
		!validRiskStatus(RiskClosed) || !validRiskStatus(RiskMitigated) {
		t.Fatal("validRiskStatus rejected a valid status")
	}
}

func TestDecisionEditable(t *testing.T) {
	if !DecisionEditable(DecisionOpen) {
		t.Fatal("DecisionEditable(open) should be true")
	}
	if DecisionEditable(DecisionApproved) {
		t.Fatal("DecisionEditable(approved) should be false")
	}
}

func TestRiskEditable(t *testing.T) {
	if !RiskEditable(RiskOpen) || !RiskEditable(RiskMonitoring) {
		t.Fatal("RiskEditable(open|monitoring) should be true")
	}
	if RiskEditable(RiskClosed) || RiskEditable(RiskMitigated) {
		t.Fatal("RiskEditable(closed|mitigated) should be false")
	}
}

func TestSortDecisions(t *testing.T) {
	t1 := parseTime(t, "2026-01-01T10:00:00Z")
	t2 := parseTime(t, "2026-01-02T10:00:00Z")
	d1 := Decision{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), CreatedAt: t1}
	d2 := Decision{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000002")), CreatedAt: t2}

	decisions := []Decision{d2, d1}
	sortDecisions(decisions)

	if decisions[0].ID != d2.ID {
		t.Fatal("sortDecisions ordering wrong")
	}
	if decisions[1].ID != d1.ID {
		t.Fatal("sortDecisions ordering wrong")
	}
}

func TestSortRisks(t *testing.T) {
	t1 := parseTime(t, "2026-01-01T10:00:00Z")
	t2 := parseTime(t, "2026-01-02T10:00:00Z")
	r1 := Risk{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), CreatedAt: t1}
	r2 := Risk{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000002")), CreatedAt: t2}

	risks := []Risk{r2, r1}
	sortRisks(risks)

	if risks[0].ID != r2.ID {
		t.Fatal("sortRisks ordering wrong")
	}
	if risks[1].ID != r1.ID {
		t.Fatal("sortRisks ordering wrong")
	}
}

func parseTime(t *testing.T, s string) time.Time {
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
