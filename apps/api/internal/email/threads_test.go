package email

import (
	"strings"
	"testing"
)

// A literal "$%d" in a query reaches Postgres verbatim and fails every call.
// This guards the production bug where the inbox returned a syntax error.
func TestQueriesHaveNoUnformattedPlaceholders(t *testing.T) {
	for _, withCursor := range []bool{false, true} {
		for name, build := range map[string]func(bool) (string, int){
			"inbox":    buildListThreadsQuery,
			"messages": buildThreadMessagesQuery,
		} {
			q, argc := build(withCursor)
			if strings.Contains(q, "%") {
				t.Errorf("%s (cursor=%v) contains an unformatted %% verb:\n%s", name, withCursor, q)
			}
			for i := 1; i <= argc; i++ {
				if !strings.Contains(q, "$"+string(rune('0'+i))) {
					t.Errorf("%s (cursor=%v) never uses placeholder $%d of %d", name, withCursor, i, argc)
				}
			}
			if strings.Contains(q, "$"+string(rune('0'+argc+1))) {
				t.Errorf("%s (cursor=%v) uses a placeholder beyond its %d arguments", name, withCursor, argc)
			}
		}
	}
}

func TestInboxQueryIsScopedToTheCaller(t *testing.T) {
	q, _ := buildListThreadsQuery(false)
	for _, want := range []string{"a.user_id = $1", "a.verified", "vm.to_addresses || vm.cc_addresses"} {
		if !strings.Contains(q, want) {
			t.Errorf("inbox query is missing %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "bcc") {
		t.Error("a blind copy must not make a thread visible to its recipient")
	}
}
