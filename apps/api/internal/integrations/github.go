package integrations

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// GitHub webhook headers and the events Atlas acts on.
const (
	// SignatureHeader carries GitHub's HMAC-SHA256 of the raw body.
	SignatureHeader = "X-Hub-Signature-256"
	// DeliveryHeader is unique per delivery; it is the dedup key.
	DeliveryHeader = "X-GitHub-Delivery"
	// EventHeader names the payload kind.
	EventHeader = "X-GitHub-Event"

	EventPullRequest = "pull_request"
	actionClosed     = "closed"
)

// VerifySignature checks GitHub's `sha256=<hex>` header against an HMAC-SHA256
// of the raw body under secret. It is constant-time and rejects any other
// scheme (legacy `sha1=`, a bare digest).
func VerifySignature(secret string, body []byte, header string) bool {
	const prefix = "sha256="
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimSpace(strings.TrimPrefix(header, prefix)))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(want, mac.Sum(nil))
}

// githubPullRequest is the subset of GitHub's pull_request payload Atlas reads;
// unknown fields are ignored.
type githubPullRequest struct {
	Action     string `json:"action"`
	Number     int    `json:"number"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	PullRequest struct {
		HTMLURL        string     `json:"html_url"`
		Title          string     `json:"title"`
		Merged         bool       `json:"merged"`
		MergedAt       *time.Time `json:"merged_at"`
		MergeCommitSHA string     `json:"merge_commit_sha"`
		Head           struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"pull_request"`
}

// ParsePullRequest decodes the pull_request subset of a GitHub delivery.
func ParsePullRequest(body []byte) (PullRequest, error) {
	var payload githubPullRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		return PullRequest{}, fmt.Errorf("%w: decode pull_request: %v", ErrInvalid, err)
	}
	return PullRequest{
		Action:     payload.Action,
		Merged:     payload.PullRequest.Merged,
		Repository: payload.Repository.FullName,
		Number:     payload.Number,
		Title:      payload.PullRequest.Title,
		URL:        payload.PullRequest.HTMLURL,
		CommitSHA:  payload.PullRequest.MergeCommitSHA,
		HeadRef:    payload.PullRequest.Head.Ref,
		BaseRef:    payload.PullRequest.Base.Ref,
		Author:     payload.PullRequest.User.Login,
		MergedAt:   payload.PullRequest.MergedAt,
	}, nil
}

// taskRefPattern matches an Atlas task ref (PRJ-2026-007/T-0142) inside a
// branch name or PR title. Case-insensitive: a branch is often lowercase.
var taskRefPattern = regexp.MustCompile(`(?i)PRJ-[0-9]{4}-[0-9]+/T-[0-9]+`)

// ExtractTaskRefs returns the distinct task refs named in the texts, in
// first-seen order. The titles and a branch name are both searched so a PR
// either way (branch or title) links.
func ExtractTaskRefs(texts ...string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, text := range texts {
		for _, match := range taskRefPattern.FindAllString(text, -1) {
			ref := strings.ToUpper(match)
			if _, dup := seen[ref]; dup {
				continue
			}
			seen[ref] = struct{}{}
			out = append(out, ref)
		}
	}
	return out
}
