package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestETagForVersion(t *testing.T) {
	if got := ETagForVersion(7); got != `"7"` {
		t.Errorf("ETagForVersion(7) = %q, want %q", got, `"7"`)
	}
}

func TestRequireIfMatch(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		want    int
		wantNil bool
	}{
		{"matching", `"4"`, 0, true},
		{"wildcard", `*`, 0, true},
		{"among candidates", `"3", "4"`, 0, true},
		{"stale", `"3"`, http.StatusPreconditionFailed, false},
		{"missing", "", http.StatusPreconditionRequired, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/", nil)
			if tc.header != "" {
				req.Header.Set("If-Match", tc.header)
			}
			err := RequireIfMatch(req, 4)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var p Problem
			if !errors.As(err, &p) {
				t.Fatalf("expected Problem, got %v", err)
			}
			if p.Status != tc.want {
				t.Errorf("status = %d, want %d", p.Status, tc.want)
			}
		})
	}
}
