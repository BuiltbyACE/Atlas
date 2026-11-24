package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"testing"
	"time"
)

func sign(secretB64, id, ts string, body []byte) string {
	key, _ := base64.StdEncoding.DecodeString(secretB64)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhook(t *testing.T) {
	secretB64 := base64.StdEncoding.EncodeToString([]byte("a-test-signing-key-0123456789"))
	secret := "whsec_" + secretB64
	now := time.Unix(1_800_000_000, 0)
	ts := strconv.FormatInt(now.Unix(), 10)
	body := []byte(`{"type":"email.received"}`)
	good := sign(secretB64, "msg_1", ts, body)

	cases := []struct {
		name    string
		secret  string
		id, ts  string
		sig     string
		body    []byte
		now     time.Time
		wantErr bool
	}{
		{"valid", secret, "msg_1", ts, good, body, now, false},
		{"valid during rotation (second signature)", secret, "msg_1", ts, "v1,AAAA " + good, body, now, false},
		{"tampered body", secret, "msg_1", ts, good, []byte(`{"type":"x"}`), now, true},
		{"wrong id", secret, "msg_2", ts, good, body, now, true},
		{"wrong secret", "whsec_" + base64.StdEncoding.EncodeToString([]byte("other-key")), "msg_1", ts, good, body, now, true},
		{"empty secret fails closed", "", "msg_1", ts, good, body, now, true},
		{"missing signature", secret, "msg_1", ts, "", body, now, true},
		{"unknown version", secret, "msg_1", ts, "v2," + good[3:], body, now, true},
		{"replayed after the window", secret, "msg_1", ts, good, body, now.Add(6 * time.Minute), true},
		{"timestamp from the future", secret, "msg_1", ts, good, body, now.Add(-6 * time.Minute), true},
		{"non-numeric timestamp", secret, "msg_1", "yesterday", good, body, now, true},
	}
	for _, c := range cases {
		err := verifyWebhook(c.secret, c.id, c.ts, c.sig, c.body, c.now)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

func TestParseInboundEnvelope(t *testing.T) {
	body := []byte(`{"type":"email.received","data":{"email_id":"e1","from":"Ada <ADA@Client.com>",
		"to":["info@safaristacksolutions.co.ke","bad address"],"subject":" Hello ","message_id":"<m1@x>",
		"attachments":[{"id":"a1","filename":"f.pdf","content_type":"application/pdf","size":10}]}}`)
	msg, ignored, err := parseInbound(body)
	if err != nil || ignored {
		t.Fatalf("err=%v ignored=%v", err, ignored)
	}
	if msg.FromAddress != "ada@client.com" || msg.MessageID != "<m1@x>" || msg.Subject != "Hello" {
		t.Errorf("unexpected message: %+v", msg)
	}
	if len(msg.ToAddresses) != 1 || msg.ToAddresses[0] != "info@safaristacksolutions.co.ke" {
		t.Errorf("recipients = %v; the malformed one must be dropped", msg.ToAddresses)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].StorageKey != "a1" {
		t.Errorf("attachments = %+v", msg.Attachments)
	}
}

func TestParseInboundFlatAndFallbackID(t *testing.T) {
	msg, _, err := parseInbound([]byte(`{"from":"a@b.com","to":["c@d.com"],"email_id":"E9"}`))
	if err != nil || msg.MessageID != "resend:E9" {
		t.Fatalf("msg=%+v err=%v", msg, err)
	}
}

func TestParseInboundIgnoresOtherEventsAndRejectsJunk(t *testing.T) {
	if _, ignored, err := parseInbound([]byte(`{"type":"email.delivered","data":{}}`)); err != nil || !ignored {
		t.Errorf("other events must be acknowledged and ignored: ignored=%v err=%v", ignored, err)
	}
	for _, body := range []string{`not json`, `{"from":"nope","message_id":"x"}`, `{"from":"a@b.com"}`} {
		if _, _, err := parseInbound([]byte(body)); err == nil {
			t.Errorf("parseInbound(%s) should fail", body)
		}
	}
}
