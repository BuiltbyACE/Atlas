package email

import (
	"fmt"
	"net/mail"
	"strings"
)

// maxAddressLen is the RFC 5321 limit for a mailbox path.
const maxAddressLen = 254

// normalizeAddress returns the bare, lower-cased address from user input such as
// "Ada Lovelace <Ada@Example.com>". Anything that is not a single valid mailbox,
// or that carries line breaks (header injection), is rejected as ErrInvalid.
func normalizeAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return "", fmt.Errorf("%w: a valid email address is required", ErrInvalid)
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil || len(parsed.Address) > maxAddressLen {
		return "", fmt.Errorf("%w: %q is not a valid email address", ErrInvalid, truncate(raw, 60))
	}
	return strings.ToLower(parsed.Address), nil
}

// normalizeAddressList normalises each entry, drops blanks and duplicates, and
// enforces a ceiling so one request cannot fan out to thousands of recipients.
func normalizeAddressList(raw []string, max int) ([]string, error) {
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r) == "" {
			continue
		}
		a, err := normalizeAddress(r)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[a]; dup {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	if len(out) > max {
		return nil, fmt.Errorf("%w: at most %d recipients are allowed", ErrInvalid, max)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
