package audit

import (
	"encoding/json"
	"testing"
)

func TestRedactSensitiveFields(t *testing.T) {
	r := NewRedactor()

	input := map[string]any{
		"full_name":    "Asha Mwangi",
		"phone_e164":   "+254700000000",
		"salary_minor": 4500000,
		"department":   map[string]any{"name": "Delivery"},
	}
	out, ok := r.Redact("person", input).(map[string]any)
	if !ok {
		t.Fatalf("unexpected type %T", r.Redact("person", input))
	}
	if out["phone_e164"] != RedactedMarker || out["salary_minor"] != RedactedMarker {
		t.Errorf("sensitive fields not redacted: %+v", out)
	}
	if out["full_name"] != "Asha Mwangi" {
		t.Errorf("non-sensitive field changed: %v", out["full_name"])
	}
}

func TestRedactLeavesOtherEntitiesAlone(t *testing.T) {
	r := NewRedactor()
	value := map[string]any{"phone_e164": "+254700000000"}
	out := r.Redact("lead", value).(map[string]any)
	if out["phone_e164"] == RedactedMarker {
		t.Error("rules for one entity leaked into another")
	}
}

func TestRedactWalksSlices(t *testing.T) {
	r := NewRedactor()
	input := []any{
		map[string]any{"value_minor": 100},
		map[string]any{"value_minor": 200},
	}
	out := r.Redact("contract", input).([]any)
	for i, item := range out {
		m := item.(map[string]any)
		if m["value_minor"] != RedactedMarker {
			t.Errorf("item %d not redacted: %+v", i, m)
		}
	}
}

func TestAddRule(t *testing.T) {
	r := NewRedactor()
	r.Add("custom", "secret_field")
	out := r.Redact("custom", map[string]any{"secret": "x", "secret_field": "y"}).(map[string]any)
	if out["secret"] != "x" {
		t.Errorf("unexpected change to non-suspended field: %v", out)
	}
	if out["secret_field"] != RedactedMarker {
		t.Errorf("added field not redacted: %v", out["secret_field"])
	}
}

func TestJSONOrNil(t *testing.T) {
	if got := jsonOrNil(nil); got != nil {
		t.Errorf("jsonOrNil(nil) = %v, want nil", got)
	}
	got, ok := jsonOrNil(map[string]string{"a": "b"}).(json.RawMessage)
	if !ok {
		t.Fatalf("unexpected type %T", got)
	}
	if string(got) != `{"a":"b"}` {
		t.Errorf("jsonOrNil = %s", got)
	}
}
