package email

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strings"
	"time"
)

// outboundMail is a message ready to be turned into RFC 5322 text. Every address
// has been normalised and every header value stripped of line breaks before it
// gets here, so building cannot be used to inject headers.
type outboundMail struct {
	From       string
	To, Cc     []string
	Subject    string
	Text, HTML string
	MessageID  string // with angle brackets
	InReplyTo  string
	References string
	Date       time.Time
}

// buildMIME renders the message. A text-only message is one text/plain part; with
// HTML it becomes multipart/alternative, text first, as RFC 2046 asks. Bcc is not
// a header at all: blind recipients exist only in the SMTP envelope.
func buildMIME(m outboundMail) ([]byte, error) {
	var b bytes.Buffer
	h := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "%s: %s\r\n", name, headerValue(value))
		}
	}
	h("From", m.From)
	h("To", strings.Join(m.To, ", "))
	h("Cc", strings.Join(m.Cc, ", "))
	h("Subject", mime.QEncoding.Encode("utf-8", headerValue(m.Subject)))
	h("Date", m.Date.UTC().Format(time.RFC1123Z))
	h("Message-ID", m.MessageID)
	h("In-Reply-To", m.InReplyTo)
	h("References", m.References)
	h("MIME-Version", "1.0")

	text := m.Text
	if text == "" && m.HTML == "" {
		return nil, fmt.Errorf("%w: the message has no body", ErrInvalid)
	}

	if m.HTML == "" {
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		if err := writeQP(&b, text); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}

	mw := multipart.NewWriter(&b)
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", mw.Boundary())
	if text == "" {
		text = "This message is HTML only."
	}
	for _, part := range []struct{ ctype, body string }{
		{"text/plain", text},
		{"text/html", m.HTML},
	} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.ctype + `; charset="utf-8"`},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		if err := writeQP(pw, part.body); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeQP(w interface{ Write([]byte) (int, error) }, body string) error {
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(body)); err != nil {
		return err
	}
	return qp.Close()
}

// headerValue removes CR and LF, the only characters that can start a new header.
func headerValue(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(strings.TrimSpace(s))
}

// domainOf returns the domain part of an address, for building a Message-ID.
func domainOf(address string) string {
	if i := strings.LastIndex(address, "@"); i >= 0 && i+1 < len(address) {
		return address[i+1:]
	}
	return "atlas.local"
}
