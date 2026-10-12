package httpx

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Page limits. §11 fixes cursor pagination for every list endpoint: callers
// pass ?limit=&cursor= and follow next_cursor while has_more is true.
const (
	DefaultPageLimit = 50
	MaxPageLimit     = 200
)

// Page is the standard list envelope (§11). The cursor is opaque to clients;
// its meaning is defined by each endpoint.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Limit      int    `json:"limit"`
}

// NewPage builds an envelope. Pass the keyset cursor for the next page (empty
// when there is none).
func NewPage[T any](items []T, next string, limit int) Page[T] {
	if items == nil {
		items = []T{}
	}
	return Page[T]{Items: items, NextCursor: next, HasMore: next != "", Limit: limit}
}

// ParsePage reads limit and cursor from the query string, applying the default
// and maximum from §11. An invalid limit is a validation error, not a silent
// clamp, so clients discover their mistake immediately.
func ParsePage(r *http.Request) (limit int, cursor string, err error) {
	limit = DefaultPageLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return 0, "", BadRequest("limit must be an integer.")
		}
		if n < 1 || n > MaxPageLimit {
			return 0, "", BadRequest(fmt.Sprintf("limit must be between 1 and %d.", MaxPageLimit))
		}
		limit = n
	}
	return limit, strings.TrimSpace(r.URL.Query().Get("cursor")), nil
}
