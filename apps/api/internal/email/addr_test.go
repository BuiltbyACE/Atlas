package email

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeAddress(t *testing.T) {
	ok := map[string]string{
		"Ada@Example.com":         "ada@example.com",
		"Ada Lovelace <ADA@x.io>": "ada@x.io",
		"  grace@navy.mil  ":      "grace@navy.mil",
	}
	for in, want := range ok {
		got, err := normalizeAddress(in)
		if err != nil || got != want {
			t.Errorf("normalizeAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", "not-an-address", "a@b.com\r\nBcc: x@y.com", "a@b.com, c@d.com"} {
		if _, err := normalizeAddress(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("normalizeAddress(%q) err = %v; want ErrInvalid", in, err)
		}
	}
}

func TestNormalizeAddressListDedupesAndCaps(t *testing.T) {
	got, err := normalizeAddressList([]string{"A@x.com", "a@x.com", " ", "b@x.com"}, 5)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v; want 2 unique addresses", got, err)
	}
	many := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		many = append(many, strings.Repeat("a", i+1)+"@x.com")
	}
	if _, err := normalizeAddressList(many, 5); !errors.Is(err, ErrInvalid) {
		t.Fatalf("over the cap: err = %v; want ErrInvalid", err)
	}
}
