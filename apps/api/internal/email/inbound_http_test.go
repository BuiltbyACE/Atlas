package email

import (
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func inboundRequest(body string, headers map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/email/inbound", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func TestInboundWebhookIsClosedWithoutASecret(t *testing.T) {
	h := NewHandler(nil, slog.Default())
	rec := httptest.NewRecorder()
	h.ReceiveInbound(rec, inboundRequest(`{"from":"a@b.com","message_id":"x"}`, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no secret configured: got %d, want 503", rec.Code)
	}
}

func TestInboundWebhookRejectsUnsignedAndBadlySigned(t *testing.T) {
	secretB64 := base64.StdEncoding.EncodeToString([]byte("a-test-signing-key-0123456789"))
	h := NewHandler(nil, slog.Default()).WithInboundSecret("whsec_" + secretB64)

	rec := httptest.NewRecorder()
	h.ReceiveInbound(rec, inboundRequest(`{"from":"a@b.com","message_id":"x"}`, nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: got %d, want 401", rec.Code)
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	rec = httptest.NewRecorder()
	h.ReceiveInbound(rec, inboundRequest(`{"from":"a@b.com","message_id":"x"}`, map[string]string{
		headerSvixID: "m1", headerSvixTimestamp: ts, headerSvixSignature: "v1,AAAA",
	}))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: got %d, want 401", rec.Code)
	}
}

func TestInboundWebhookAcknowledgesSignedNonMessageEvents(t *testing.T) {
	secretB64 := base64.StdEncoding.EncodeToString([]byte("a-test-signing-key-0123456789"))
	h := NewHandler(nil, slog.Default()).WithInboundSecret("whsec_" + secretB64)
	body := `{"type":"email.delivered","data":{}}`
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	rec := httptest.NewRecorder()
	h.ReceiveInbound(rec, inboundRequest(body, map[string]string{
		headerSvixID: "m1", headerSvixTimestamp: ts, headerSvixSignature: sign(secretB64, "m1", ts, []byte(body)),
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("signed non-message event: got %d, want 200", rec.Code)
	}
}
