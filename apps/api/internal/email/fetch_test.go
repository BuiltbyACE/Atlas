package email

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendFetcherReadsBodyAndThreadingHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails/receiving/abc-123" || r.Header.Get("Authorization") != "Bearer re_test" {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"text":"hello","html":"<p>hello</p>","headers":{"In-Reply-To":"<a@x>","References":"<a@x>"}}`))
	}))
	defer srv.Close()

	f := NewResendFetcher("re_test")
	f.BaseURL = srv.URL
	c, err := f.FetchReceived(context.Background(), "abc-123")
	if err != nil {
		t.Fatal(err)
	}
	if c.Text != "hello" || c.HTML != "<p>hello</p>" || c.InReplyTo != "<a@x>" || c.References != "<a@x>" {
		t.Fatalf("unexpected content: %+v", c)
	}
}

func TestResendFetcherReportsProviderFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	f := NewResendFetcher("re_test")
	f.BaseURL = srv.URL
	if _, err := f.FetchReceived(context.Background(), "x"); err == nil {
		t.Fatal("a provider 404 must be an error")
	}
}

func TestInboundEmailIDEnvelopeAndFlat(t *testing.T) {
	if got := inboundEmailID([]byte(`{"type":"email.received","data":{"email_id":"e1"}}`)); got != "e1" {
		t.Fatalf("envelope: %q", got)
	}
	if got := inboundEmailID([]byte(`{"email_id":"e2"}`)); got != "e2" {
		t.Fatalf("flat: %q", got)
	}
}

func TestNewResendFetcherNeedsKey(t *testing.T) {
	if NewResendFetcher("  ") != nil {
		t.Fatal("empty key must give no fetcher")
	}
}
