package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LeadSourceSeed is a configured origin for leads (docs/ATLAS.md §8.1 [CONFIG]).
// The rows are the starting set an operator then edits in place.
type LeadSourceSeed struct {
	Key   string
	Label string
	Sort  int
}

// PipelineStageSeed is a configured opportunity stage (§8.1 [CONFIG]). At most
// one stage is the won stage and one the lost stage; the sales UI uses those to
// close a deal.
type PipelineStageSeed struct {
	Key    string
	Label  string
	Sort   int
	IsWon  bool
	IsLost bool
}

// LeadSources is the initial lead-source catalogue from §8.1.
var LeadSources = []LeadSourceSeed{
	{"website_form", "Website form", 10},
	{"referral", "Referral", 20},
	{"event", "Event", 30},
	{"inbound_call", "Inbound call", 40},
	{"whatsapp", "WhatsApp", 50},
	{"outbound", "Outbound", 60},
	{"partner", "Partner", 70},
	{"existing_client", "Existing client", 80},
}

// PipelineStages is the initial pipeline from §8.1: Discovery → Solution design
// → Proposal sent → Negotiation → Contract → Won / Lost.
var PipelineStages = []PipelineStageSeed{
	{"discovery", "Discovery", 10, false, false},
	{"solution_design", "Solution design", 20, false, false},
	{"proposal_sent", "Proposal sent", 30, false, false},
	{"negotiation", "Negotiation", 40, false, false},
	{"contract", "Contract", 50, false, false},
	{"won", "Won", 90, true, false},
	{"lost", "Lost", 100, false, true},
}

// seedCRMConfig inserts the CRM config tables' starting rows. Each insert is
// ON CONFLICT DO NOTHING so a re-seed never overwrites an operator's edit.
func seedCRMConfig(ctx context.Context, tx pgx.Tx) (added int, err error) {
	for _, s := range LeadSources {
		tag, err := tx.Exec(ctx, `
			INSERT INTO crm.lead_sources (key, label, sort_order)
			VALUES ($1, $2, $3) ON CONFLICT (key) DO NOTHING`,
			s.Key, s.Label, s.Sort)
		if err != nil {
			return added, fmt.Errorf("seed lead source %s: %w", s.Key, err)
		}
		added += int(tag.RowsAffected())
	}
	for _, s := range PipelineStages {
		tag, err := tx.Exec(ctx, `
			INSERT INTO crm.pipeline_stages (key, label, sort_order, is_won, is_lost)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (key) DO NOTHING`,
			s.Key, s.Label, s.Sort, s.IsWon, s.IsLost)
		if err != nil {
			return added, fmt.Errorf("seed pipeline stage %s: %w", s.Key, err)
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}
