package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ExpenseCategorySeed is a configured spend category (docs/ATLAS.md §8.4
// [CONFIG]). The rows are the starting set an operator then edits in place.
type ExpenseCategorySeed struct {
	Key   string
	Label string
	Sort  int
}

// ExpenseCategories is the initial spend-category catalogue from §8.4:
// operational, salaries, hosting/cloud, software subscriptions, transport,
// marketing, contractors, equipment.
var ExpenseCategories = []ExpenseCategorySeed{
	{"operational", "Operational", 10},
	{"salaries", "Salaries", 20},
	{"hosting_cloud", "Hosting/cloud", 30},
	{"software_subscriptions", "Software subscriptions", 40},
	{"transport", "Transport", 50},
	{"marketing", "Marketing", 60},
	{"contractors", "Contractors", 70},
	{"equipment", "Equipment", 80},
}

// seedFinanceConfig inserts the finance config tables' starting rows. Each
// insert is ON CONFLICT DO NOTHING so a re-seed never overwrites an operator's
// edit (§8.4 [CONFIG]).
func seedFinanceConfig(ctx context.Context, tx pgx.Tx) (added int, err error) {
	for _, s := range ExpenseCategories {
		tag, err := tx.Exec(ctx, `
			INSERT INTO finance.expense_categories (key, label, sort_order)
			VALUES ($1, $2, $3) ON CONFLICT (key) DO NOTHING`,
			s.Key, s.Label, s.Sort)
		if err != nil {
			return added, fmt.Errorf("seed expense category %s: %w", s.Key, err)
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}
