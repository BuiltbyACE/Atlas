package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// Atlas uses optimistic concurrency on updates (§11): a resource carries a
// version, updates must send If-Match, and a stale version is a 412.

// ETagForVersion returns the strong entity tag for a resource version.
func ETagForVersion(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

// SetETag writes the ETag header for a versioned resource.
func SetETag(w http.ResponseWriter, version int64) {
	w.Header().Set("ETag", ETagForVersion(version))
}

// RequireIfMatch enforces If-Match against the current version. A missing
// header is a 428 (the client must re-read first); a stale value is a 412.
// "If-Match: *" is accepted, as RFC 9110 defines it, meaning "any current
// representation" — useful when a client cannot carry an ETag.
func RequireIfMatch(r *http.Request, version int64) error {
	got := strings.TrimSpace(r.Header.Get("If-Match"))
	if got == "" {
		return PreconditionRequired("If-Match is required for this request; read the resource first.")
	}
	if got == "*" {
		return nil
	}
	for _, candidate := range strings.Split(got, ",") {
		if strings.TrimSpace(candidate) == ETagForVersion(version) {
			return nil
		}
	}
	return PreconditionFailed("The resource changed since you last read it.")
}
