package audit

import "encoding/json"

// defaultRules suppresses fields the blueprint names as sensitive (§6.2):
// salaries, client contract values and personal phone numbers.
var defaultRules = map[string]map[string]struct{}{
	"person":   fieldSet("salary_minor", "salary", "phone_e164", "national_id", "bank_account", "kra_pin"),
	"contract": fieldSet("value_minor", "value"),
	"contact":  fieldSet("phone_e164"),
}

func fieldSet(names ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}

// Redactor suppresses sensitive fields from audit snapshots (§6.2, §6.3).
type Redactor struct {
	rules map[string]map[string]struct{}
}

// NewRedactor returns a Redactor with the blueprint's default rules.
func NewRedactor() *Redactor {
	r := &Redactor{rules: make(map[string]map[string]struct{}, len(defaultRules))}
	for entity, fields := range defaultRules {
		cp := make(map[string]struct{}, len(fields))
		for f := range fields {
			cp[f] = struct{}{}
		}
		r.rules[entity] = cp
	}
	return r
}

// Add registers additional suppressed fields for an entity type.
func (r *Redactor) Add(entityType string, fields ...string) {
	m := r.rules[entityType]
	if m == nil {
		m = map[string]struct{}{}
		r.rules[entityType] = m
	}
	for _, f := range fields {
		m[f] = struct{}{}
	}
}

// Redact returns a copy of value with the entity's sensitive fields replaced
// by RedactedMarker. Maps and slices are walked; scalars pass through.
func (r *Redactor) Redact(entityType string, value any) any {
	if value == nil {
		return nil
	}
	fields := r.rules[entityType]

	// Normalise through JSON so structs, maps and slices share one path.
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return value
	}
	return redactValue(generic, fields)
}

func redactValue(v any, fields map[string]struct{}) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			if _, hidden := fields[k]; hidden {
				out[k] = RedactedMarker
				continue
			}
			out[k] = redactValue(val, fields)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, val := range typed {
			out[i] = redactValue(val, fields)
		}
		return out
	default:
		return v
	}
}

// jsonOrNil marshals v for a jsonb parameter, or returns nil (SQL NULL).
func jsonOrNil(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}
