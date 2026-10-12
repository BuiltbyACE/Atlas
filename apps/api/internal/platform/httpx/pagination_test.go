package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParsePageDefaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	limit, cursor, err := ParsePage(req)
	if err != nil {
		t.Fatalf("ParsePage() error = %v", err)
	}
	if limit != DefaultPageLimit {
		t.Errorf("limit = %d, want %d", limit, DefaultPageLimit)
	}
	if cursor != "" {
		t.Errorf("cursor = %q, want empty", cursor)
	}
}

func TestParsePageExplicit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=25&cursor=abc", nil)
	limit, cursor, err := ParsePage(req)
	if err != nil {
		t.Fatalf("ParsePage() error = %v", err)
	}
	if limit != 25 {
		t.Errorf("limit = %d, want 25", limit)
	}
	if cursor != "abc" {
		t.Errorf("cursor = %q, want abc", cursor)
	}
}

func TestParsePageRejectsInvalid(t *testing.T) {
	for _, q := range []string{"?limit=0", "?limit=-3", "?limit=9999", "?limit=abc"} {
		req := httptest.NewRequest(http.MethodGet, "/"+q, nil)
		_, _, err := ParsePage(req)
		var p Problem
		if !errors.As(err, &p) {
			t.Fatalf("%s: expected a Problem, got %v", q, err)
		}
		if p.Status != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, p.Status)
		}
	}
}

func TestNewPageNormalisesNilItems(t *testing.T) {
	p := NewPage[string](nil, "", 10)
	if p.Items == nil {
		t.Error("items should be an empty slice, not nil")
	}
	if p.HasMore {
		t.Error("HasMore should be false with no cursor")
	}
}
