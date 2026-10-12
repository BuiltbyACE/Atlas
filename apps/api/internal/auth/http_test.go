package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// The credential endpoints must refuse a text/plain body even when it is
// well-formed JSON: that is the shape a cross-site form would use to plant a
// login (docs/FRONTEND.md §3, CSRF). The rejection happens before the service
// is consulted, so no service is wired here.
func TestCredentialEndpointsRejectNonJSONBodies(t *testing.T) {
	h := NewHandler(nil, HTTPConfig{CookieName: "atlas_session"}, nil)
	endpoints := map[string]http.HandlerFunc{
		"login":         h.Login,
		"mfa/verify":    h.MFAVerify,
		"mfa/enroll":    h.MFAEnroll,
		"mfa/activate":  h.MFAActivate,
		"invite/accept": h.AcceptInvite,
	}
	for name, handler := range endpoints {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/"+name,
				strings.NewReader(`{"identifier":"a@b.c","password":"x"}`))
			req.Header.Set("Content-Type", "text/plain")
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want 415", rec.Code)
			}
			var p httpx.Problem
			if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if p.Code != httpx.CodeUnsupportedMediaType {
				t.Errorf("code = %q, want %q", p.Code, httpx.CodeUnsupportedMediaType)
			}
			if c := rec.Header().Get("Set-Cookie"); c != "" {
				t.Errorf("unexpected Set-Cookie %q", c)
			}
		})
	}
}
