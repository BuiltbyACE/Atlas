package integrations

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignature(t *testing.T) {
	const secret = "s3cr3t"
	body := []byte(`{"action":"closed"}`)
	valid := signBody(secret, body)

	cases := []struct {
		name   string
		secret string
		body   []byte
		header string
		want   bool
	}{
		{"valid", secret, body, valid, true},
		{"wrong secret", "other", body, valid, false},
		{"tampered body", secret, []byte(`{"action":"opened"}`), valid, false},
		{"missing scheme", secret, body, "deadbeef", false},
		{"legacy sha1 scheme", secret, body, "sha1=deadbeef", false},
		{"bad hex", secret, body, "sha256=not-hex", false},
		{"empty header", secret, body, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifySignature(tc.secret, tc.body, tc.header); got != tc.want {
				t.Fatalf("VerifySignature() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractTaskRefs(t *testing.T) {
	cases := []struct {
		name  string
		texts []string
		want  []string
	}{
		{
			"branch name",
			[]string{"", "prj-2026-007/t-0142-fix-login"},
			[]string{"PRJ-2026-007/T-0142"},
		},
		{
			"title only",
			[]string{"Deliver PRJ-2026-007/T-0142 acceptance criteria", ""},
			[]string{"PRJ-2026-007/T-0142"},
		},
		{
			"deduped across title and branch",
			[]string{"PRJ-2026-007/T-0142 fix", "feat/PRJ-2026-007/T-0142"},
			[]string{"PRJ-2026-007/T-0142"},
		},
		{"none", []string{"no ref here"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractTaskRefs(tc.texts...); !equalStrings(got, tc.want) {
				t.Fatalf("ExtractTaskRefs = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParsePullRequest(t *testing.T) {
	body := []byte(`{
		"action": "closed",
		"number": 42,
		"repository": {"full_name": "safaristack/atlas"},
		"pull_request": {
			"html_url": "https://github.com/safaristack/atlas/pull/42",
			"title": "PRJ-2026-007/T-0142 ship the board",
			"merged": true,
			"merge_commit_sha": "abc123",
			"merged_at": "2026-10-17T10:00:00Z",
			"user": {"login": "octocat"},
			"head": {"ref": "PRJ-2026-007/T-0142-board"},
			"base": {"ref": "main"}
		}
	}`)
	pr, err := ParsePullRequest(body)
	if err != nil {
		t.Fatalf("ParsePullRequest: %v", err)
	}
	if pr.Action != "closed" || !pr.Merged {
		t.Fatalf("action/merged = %q/%v", pr.Action, pr.Merged)
	}
	if pr.Repository != "safaristack/atlas" || pr.Number != 42 {
		t.Fatalf("repo/number = %q/%d", pr.Repository, pr.Number)
	}
	if pr.HeadRef != "PRJ-2026-007/T-0142-board" || pr.Author != "octocat" {
		t.Fatalf("head/author = %q/%q", pr.HeadRef, pr.Author)
	}
	if pr.MergedAt == nil {
		t.Fatal("merged_at not parsed")
	}
}

func TestParsePullRequestRejectsGarbage(t *testing.T) {
	if _, err := ParsePullRequest([]byte("not json")); err == nil {
		t.Fatal("expected an error for a non-JSON body")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
