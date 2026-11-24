package email

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Svix-style webhook signing, which Resend uses for inbound and event webhooks.
// Each delivery carries svix-id, svix-timestamp and svix-signature headers. The
// signature is base64(HMAC-SHA256(secret, id + "." + timestamp + "." + body)),
// sent as one or more space-separated "v1,<signature>" entries (several during
// a secret rotation). The secret is shown as "whsec_<base64>".
const (
	headerSvixID        = "svix-id"
	headerSvixTimestamp = "svix-timestamp"
	headerSvixSignature = "svix-signature"

	// webhookTolerance bounds clock skew and rejects replays of old deliveries.
	webhookTolerance = 5 * time.Minute
)

var errBadSignature = errors.New("email: webhook signature is not valid")

// verifyWebhook checks a delivery's signature and freshness. It fails closed: an
// empty secret never verifies anything.
func verifyWebhook(secret, id, timestamp, signatureHeader string, body []byte, now time.Time) error {
	if strings.TrimSpace(secret) == "" {
		return errBadSignature
	}
	if id == "" || timestamp == "" || signatureHeader == "" {
		return errBadSignature
	}
	sec, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errBadSignature
	}
	age := now.Sub(time.Unix(sec, 0))
	if age > webhookTolerance || age < -webhookTolerance {
		return fmt.Errorf("%w: timestamp outside the %s window", errBadSignature, webhookTolerance)
	}

	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(secret), "whsec_"))
	if err != nil || len(key) == 0 {
		return errBadSignature
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	expected := mac.Sum(nil)

	for _, part := range strings.Fields(signatureHeader) {
		version, sig, found := strings.Cut(part, ",")
		if !found || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(sig)
		if err != nil {
			continue
		}
		if hmac.Equal(got, expected) {
			return nil
		}
	}
	return errBadSignature
}

// inboundData is the message part of an inbound webhook. Resend wraps it in an
// event envelope ({"type":"email.received","data":{...}}); the older flat form
// carried the same fields at the top level, and both are accepted.
type inboundData struct {
	EmailID     string   `json:"email_id"`
	From        string   `json:"from"`
	To          []string `json:"to"`
	Cc          []string `json:"cc"`
	Subject     string   `json:"subject"`
	Text        string   `json:"text"`
	HTML        string   `json:"html"`
	MessageID   string   `json:"message_id"`
	InReplyTo   string   `json:"in_reply_to"`
	References  string   `json:"references"`
	Attachments []struct {
		ID          string `json:"id"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
		URL         string `json:"url"`
	} `json:"attachments"`
}

type inboundEnvelope struct {
	Type string       `json:"type"`
	Data *inboundData `json:"data"`
	inboundData
}

// parseInbound turns a webhook body into a message. ignored is true for events
// that are not an inbound message (delivery notices and the like), which must be
// acknowledged with 200 so the provider does not retry them.
func parseInbound(body []byte) (msg *InboundMessage, ignored bool, err error) {
	var env inboundEnvelope
	if err := jsonUnmarshal(body, &env); err != nil {
		return nil, false, fmt.Errorf("%w: the body is not valid JSON", ErrInvalid)
	}
	if env.Type != "" && env.Type != "email.received" {
		return nil, true, nil
	}
	d := env.inboundData
	if env.Data != nil {
		d = *env.Data
	}

	from := bestEffortAddress(d.From)
	if from == "" {
		return nil, false, fmt.Errorf("%w: the message has no valid sender", ErrInvalid)
	}
	messageID := strings.TrimSpace(d.MessageID)
	if messageID == "" && d.EmailID != "" {
		messageID = "resend:" + d.EmailID
	}
	if messageID == "" {
		return nil, false, fmt.Errorf("%w: the message has no id", ErrInvalid)
	}

	msg = &InboundMessage{
		MessageID:     messageID,
		InReplyTo:     strings.TrimSpace(d.InReplyTo),
		MsgReferences: strings.TrimSpace(d.References),
		FromAddress:   from,
		ToAddresses:   bestEffortList(d.To),
		CCAddresses:   bestEffortList(d.Cc),
		Subject:       truncate(strings.TrimSpace(d.Subject), 500),
		BodyText:      d.Text,
		BodyHTML:      d.HTML,
	}
	for _, a := range d.Attachments {
		key := a.URL
		if key == "" {
			key = a.ID
		}
		msg.Attachments = append(msg.Attachments, InboundAttachment{
			Filename: a.Filename, ContentType: a.ContentType, SizeBytes: a.Size, StorageKey: key,
		})
	}
	return msg, false, nil
}

// bestEffortAddress extracts a bare address, or "" when there is none. Inbound
// mail is not ours to reject for a malformed header.
func bestEffortAddress(raw string) string {
	a, err := normalizeAddress(raw)
	if err != nil {
		return ""
	}
	return a
}

func bestEffortList(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if a := bestEffortAddress(r); a != "" {
			out = append(out, a)
		}
	}
	return out
}

var jsonUnmarshal = json.Unmarshal
