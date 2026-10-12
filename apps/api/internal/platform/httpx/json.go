package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
)

// MaxRequestBytes caps request bodies so a malformed or hostile client cannot
// exhaust memory (§11). 1 MiB is generous for JSON; uploads go to object
// storage through a separate flow, not through the JSON API.
const MaxRequestBytes = 1 << 20

// WriteJSON writes v as JSON with the given status code. The response is
// small enough that Atlas encodes it in one go; encode failures are logged by
// the request logger, never leaked to the caller.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// DecodeJSON reads a single JSON object into dst. Unknown fields are rejected
// and the body is size-limited, so the wire contract stays explicit (§11).
// On failure it returns a Problem the handler can pass to WriteError.
//
// The body must be declared `application/json`. That is load-bearing for the
// CSRF posture (docs/FRONTEND.md §3): a cross-site HTML form can only send
// form-encoded, multipart or text/plain bodies, and text/plain can still
// carry a well-formed JSON document — so the media type, not the bytes, is
// what a browser form cannot forge without a CORS preflight.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if !IsJSONContentType(r.Header.Get("Content-Type")) {
		return New(http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "The request body must be sent as application/json.")
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return New(http.StatusRequestEntityTooLarge, CodeValidationFailed, "The request body is too large.")
		}
		if errors.Is(err, io.EOF) {
			return BadRequest("A JSON request body is required.")
		}
		return BadRequest("The request body is not valid JSON: " + err.Error())
	}

	// Reject trailing content so "{} {}" cannot smuggle a second document.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BadRequest("The request body must contain a single JSON object.")
	}
	return nil
}

// IsJSONContentType reports whether a Content-Type header names
// application/json, ignoring parameters such as charset.
func IsJSONContentType(header string) bool {
	mediaType, _, err := mime.ParseMediaType(header)
	return err == nil && mediaType == "application/json"
}
