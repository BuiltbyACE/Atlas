package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Integration event types (docs/ATLAS.md §7.4, §12). The outbound adapters live
// in the orchestrator; Go records what happened at the edge so every consumer
// sees the same fact (ADR-002).
const (
	TypeGitPullRequestMergedV1 = "integrations.git.pull_request.merged.v1"
)

// Aggregate types for integration events.
const (
	AggregatePullRequest = "pull_request"
)

// GitPullRequestMergedPayload is emitted when a merged GitHub pull request
// links to an Atlas task (decision D6, docs/adr/ADR-018). It carries the
// identifiers a consumer needs to invalidate the task's views; the evidence row
// itself is read from Go (ADR-009).
type GitPullRequestMergedPayload struct {
	EvidenceID uuid.UUID  `json:"evidence_id"`
	TaskID     uuid.UUID  `json:"task_id"`
	ProjectID  uuid.UUID  `json:"project_id"`
	Provider   string     `json:"provider"`
	Repository string     `json:"repository"`
	Number     int        `json:"number"`
	URL        string     `json:"pull_request_url"`
	Title      string     `json:"title,omitempty"`
	CommitSHA  string     `json:"merge_commit_sha,omitempty"`
	Author     string     `json:"author,omitempty"`
	MergedAt   *time.Time `json:"merged_at,omitempty"`
	LinkedAt   time.Time  `json:"linked_at"`
}

// PullRequestLink is the minimal view the merged-PR event builder needs.
type PullRequestLink struct {
	EvidenceID uuid.UUID
	TaskID     uuid.UUID
	ProjectID  uuid.UUID
	Provider   string
	Repository string
	Number     int
	Title      string
	URL        string
	CommitSHA  string
	Author     string
	MergedAt   *time.Time
	LinkedAt   time.Time
}

// NewGitPullRequestMerged builds the D6 integrations event. Its aggregate is
// the evidence row the link created: the pull request is external and has no
// Atlas id of its own.
func NewGitPullRequestMerged(link PullRequestLink) (Event, error) {
	return buildEvent(TypeGitPullRequestMergedV1, AggregatePullRequest, link.EvidenceID, nil, nil, GitPullRequestMergedPayload{
		EvidenceID: link.EvidenceID,
		TaskID:     link.TaskID,
		ProjectID:  link.ProjectID,
		Provider:   link.Provider,
		Repository: link.Repository,
		Number:     link.Number,
		URL:        link.URL,
		Title:      link.Title,
		CommitSHA:  link.CommitSHA,
		Author:     link.Author,
		MergedAt:   link.MergedAt,
		LinkedAt:   link.LinkedAt,
	})
}
