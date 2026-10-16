package approvals

import (
	"time"

	"github.com/google/uuid"
)

// Policy mirrors a row in core.approval_policies (§6.7). Thresholds are
// configuration, editable only with joint CEO approval; the engine only reads
// them to decide who may approve and how many approvals a decision needs.
type Policy struct {
	ID             uuid.UUID
	DecisionType   string
	MinAmountMinor *int64
	MaxAmountMinor *int64
	Currency       string
	ApproverRule   string
	EffectiveFrom  time.Time
	EffectiveTo    *time.Time
}

// PolicyQuery selects the approval policy that governs a decision. DecisionType
// is the approval's entity_type: in Phase 1 the two share one vocabulary
// ('expense', 'invoice', 'discount', …) until policies carry their own key.
type PolicyQuery struct {
	DecisionType string
	AmountMinor  int64
	Currency     string
	At           time.Time
}

// Approver rule tokens (§3.3). Only the tokens the engine understands as a
// single role are listed; anything else is treated as a literal role key.
const (
	// RuleBothCEOs is a two-person decision: two distinct CEOs must approve.
	RuleBothCEOs = "both_ceos"
	// RuleDomainCEO is a domain CEO; which one depends on the decision, so it
	// is not resolvable to a single role key and the caller must be explicit.
	RuleDomainCEO = "domain_ceo"
	// RuleLineManager depends on the requester's reporting line.
	RuleLineManager = "line_manager"
)

// ApprovalsRequired reports how many distinct approvals the rule needs.
func (p Policy) ApprovalsRequired() int {
	if p.ApproverRule == RuleBothCEOs {
		return 2
	}
	return 1
}

// Role returns the role key the rule names, when it names exactly one. Rules
// that depend on context (line_manager, domain_ceo) or that span two roles
// (both_ceos) return "" — the caller must supply approver_role in that case.
func (p Policy) Role() string {
	switch p.ApproverRule {
	case "", RuleLineManager, RuleDomainCEO, RuleBothCEOs:
		return ""
	default:
		return p.ApproverRule
	}
}
