package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONSetsContentTypeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]string{"id": "abc"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"id":"abc"}` {
		t.Errorf("body = %q", body)
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	rec := httptest.NewRecorder()
	req := jsonRequest(`{"email":"a@b.c","nope":1}`)

	var dst struct {
		Email string `json:"email"`
	}
	if err := DecodeJSON(rec, req, &dst); err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}

func TestDecodeJSONRejectsTrailingDocument(t *testing.T) {
	rec := httptest.NewRecorder()
	req := jsonRequest(`{} {}`)

	var dst map[string]any
	if err := DecodeJSON(rec, req, &dst); err == nil {
		t.Fatal("expected an error for trailing content")
	}
}

func TestDecodeJSONRejectsEmptyBody(t *testing.T) {
	rec := httptest.NewRecorder()
	req := jsonRequest("")

	var dst map[string]any
	err := DecodeJSON(rec, req, &dst)
	var p Problem
	if !errors.As(err, &p) || p.Status != http.StatusBadRequest {
		t.Fatalf("err = %v, want a 400 Problem", err)
	}
}

func TestDecodeJSONAcceptsSnakeCaseObject(t *testing.T) {
	rec := httptest.NewRecorder()
	req := jsonRequest(`{"email":"a@b.c"}`)

	var dst struct {
		Email string `json:"email"`
	}
	if err := DecodeJSON(rec, req, &dst); err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}
	if dst.Email != "a@b.c" {
		t.Errorf("Email = %q", dst.Email)
	}
}

func TestDecodeJSONRejectsOversizedBody(t *testing.T) {
	rec := httptest.NewRecorder()
	big := `{"email":"` + strings.Repeat("x", MaxRequestBytes+16) + `"}`
	req := jsonRequest(big)

	var dst struct {
		Email string `json:"email"`
	}
	err := DecodeJSON(rec, req, &dst)
	var p Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected Problem, got %v", err)
	}
	if p.Status != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", p.Status)
	}
}

// jsonRequest builds a POST carrying body as application/json.
func jsonRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestDecodeJSONAcceptsJSONWithCharset(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"a@b.c"}`))
	req.Header.Set("Content-Type", "Application/JSON; charset=utf-8")

	var dst struct {
		Email string `json:"email"`
	}
	if err := DecodeJSON(rec, req, &dst); err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}
}

// A cross-site HTML form cannot send application/json, but text/plain can
// still carry a well-formed JSON document. Rejecting it on media type is what
// keeps credential endpoints out of reach of form-based CSRF.
func TestDecodeJSONRejectsNonJSONMediaTypes(t *testing.T) {
	for _, ct := range []string{"", "text/plain", "text/plain; charset=utf-8", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
		t.Run(ct, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"a@b.c"}`))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}

			var dst struct {
				Email string `json:"email"`
			}
			err := DecodeJSON(rec, req, &dst)
			var p Problem
			if !errors.As(err, &p) || p.Status != http.StatusUnsupportedMediaType || p.Code != CodeUnsupportedMediaType {
				t.Fatalf("err = %v, want a 415 %s Problem", err, CodeUnsupportedMediaType)
			}
			if dst.Email != "" {
				t.Errorf("body was decoded despite the media type: %q", dst.Email)
			}
		})
	}
}
