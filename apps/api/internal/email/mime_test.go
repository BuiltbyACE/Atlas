package email

import (
	"errors"
	"io"
	"mime"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestBuildMIMETextOnly(t *testing.T) {
	raw, err := buildMIME(outboundMail{
		From: "ada@safaristacksolutions.co.ke", To: []string{"c@client.com"}, Cc: []string{"d@client.com"},
		Subject: "Invoice ✓", Text: "Hello, world — é", MessageID: "<m1@safaristacksolutions.co.ke>",
		InReplyTo: "<p@x>", Date: time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("output is not a valid RFC 5322 message: %v", err)
	}
	if got := msg.Header.Get("To"); got != "c@client.com" {
		t.Errorf("To = %q", got)
	}
	subj, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if subj != "Invoice ✓" {
		t.Errorf("Subject decoded = %q", subj)
	}
	if msg.Header.Get("In-Reply-To") != "<p@x>" || msg.Header.Get("Message-ID") == "" {
		t.Error("threading headers missing")
	}
	body, _ := io.ReadAll(msg.Body)
	if !strings.Contains(string(body), "Hello, world") {
		t.Errorf("body = %q", body)
	}
}

func TestBuildMIMEHTMLIsMultipartAndBccNeverAppears(t *testing.T) {
	raw, err := buildMIME(outboundMail{
		From: "a@x.com", To: []string{"b@x.com"}, Subject: "s", Text: "plain", HTML: "<p>html</p>",
		MessageID: "<m@x.com>", Date: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "multipart/alternative") || !strings.Contains(s, "text/html") {
		t.Errorf("expected multipart/alternative with an html part:\n%s", s)
	}
	if strings.Contains(strings.ToLower(s), "\r\nbcc:") {
		t.Error("Bcc must never be written as a header")
	}
}

func TestBuildMIMEBlocksHeaderInjectionAndEmptyBodies(t *testing.T) {
	raw, err := buildMIME(outboundMail{
		From: "a@x.com", To: []string{"b@x.com"}, Subject: "hi\r\nBcc: evil@x.com", Text: "t",
		MessageID: "<m@x.com>", Date: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := mail.ReadMessage(strings.NewReader(string(raw)))
	if msg.Header.Get("Bcc") != "" {
		t.Error("a CRLF in the subject must not create a Bcc header")
	}
	if _, err := buildMIME(outboundMail{From: "a@x.com", To: []string{"b@x.com"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty body: err = %v, want ErrInvalid", err)
	}
}
