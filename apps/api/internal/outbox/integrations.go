package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Integration event types (docs/ATLAS.md §7.4, §12). The outbound adapters live
// in the orchestrator; Go records what happened at the edge so every consumer
// sees the same fact (ADR-002).
const (
	TypeGitPullRequestMergedV1  = "integrations.git.pull_request.merged.v1"
	TypeGitHubPushV1            = "integrations.github.push.v1"
	TypeGitHubPullRequestV1     = "integrations.github.pull_request.v1"
	TypeGitHubCheckRunV1        = "integrations.github.check_run.v1"
	TypeGitHubDeploymentStatusV1 = "integrations.github.deployment_status.v1"
	TypeGitHubInstallationV1    = "integrations.github.installation.v1"
)

// Aggregate types for integration events.
const (
	AggregatePullRequest = "pull_request"
	AggregateGitHub      = "github"
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

// GitHubPushPayload is emitted when code is pushed to a repository.
type GitHubPushPayload struct {
	InstallationID int64     `json:"installation_id"`
	Repository     string    `json:"repository"`
	Ref            string    `json:"ref"`          // git ref, e.g. refs/heads/feature/x
	Before         string    `json:"before"`       // previous commit SHA
	After          string    `json:"after"`        // new commit SHA
	SenderLogin    string    `json:"sender_login"`
	SenderID       *int64    `json:"sender_id,omitempty"`
	PersonID       *uuid.UUID `json:"person_id,omitempty"` // resolved SCOS person, if known
	Commits        int       `json:"commits"`
	CreatedAt      time.Time `json:"created_at"`
}

// GitHubPushRef is the minimal view the push event builder needs.
type GitHubPushRef struct {
	InstallationID int64
	Repository     string
	Ref            string
	Before         string
	After          string
	SenderLogin    string
	SenderID       *int64
	PersonID       *uuid.UUID
	Commits        int
}

// NewGitHubPush builds the push event.
func NewGitHubPush(ref GitHubPushRef) (Event, error) {
	return buildEvent(TypeGitHubPushV1, AggregateGitHub, uuid.UUID{}, nil, nil, GitHubPushPayload{
		InstallationID: ref.InstallationID,
		Repository:     ref.Repository,
		Ref:            ref.Ref,
		Before:         ref.Before,
		After:          ref.After,
		SenderLogin:    ref.SenderLogin,
		SenderID:       ref.SenderID,
		PersonID:       ref.PersonID,
		Commits:        ref.Commits,
		CreatedAt:      time.Now().UTC(),
	})
}

// GitHubPullRequestPayload is emitted for any pull_request webhook action.
type GitHubPullRequestPayload struct {
	InstallationID int64     `json:"installation_id"`
	Repository     string    `json:"repository"`
	Number         int       `json:"number"`
	Action         string    `json:"action"` // opened, closed, reopened, etc.
	Merged         bool      `json:"merged"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	HeadRef        string    `json:"head_ref"`
	BaseRef        string    `json:"base_ref"`
	AuthorLogin    string    `json:"author_login"`
	SenderLogin    string    `json:"sender_login"`
	PersonID       *uuid.UUID `json:"person_id,omitempty"` // resolved SCOS person, if known
	MergedAt       *time.Time `json:"merged_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// GitHubPullRequestRef is the minimal view the PR event builder needs.
type GitHubPullRequestRef struct {
	InstallationID int64
	Repository     string
	Number         int
	Action         string
	Merged         bool
	Title          string
	URL            string
	HeadRef        string
	BaseRef        string
	AuthorLogin    string
	SenderLogin    string
	PersonID       *uuid.UUID
	MergedAt       *time.Time
}

// NewGitHubPullRequest builds the pull_request event.
func NewGitHubPullRequest(ref GitHubPullRequestRef) (Event, error) {
	return buildEvent(TypeGitHubPullRequestV1, AggregateGitHub, uuid.UUID{}, nil, nil, GitHubPullRequestPayload{
		InstallationID: ref.InstallationID,
		Repository:     ref.Repository,
		Number:         ref.Number,
		Action:         ref.Action,
		Merged:         ref.Merged,
		Title:          ref.Title,
		URL:            ref.URL,
		HeadRef:        ref.HeadRef,
		BaseRef:        ref.BaseRef,
		AuthorLogin:    ref.AuthorLogin,
		SenderLogin:    ref.SenderLogin,
		PersonID:       ref.PersonID,
		MergedAt:       ref.MergedAt,
		CreatedAt:      time.Now().UTC(),
	})
}

// GitHubCheckRunPayload is emitted when a check run completes.
type GitHubCheckRunPayload struct {
	InstallationID int64     `json:"installation_id"`
	Repository     string    `json:"repository"`
	CheckRun       string    `json:"check_run"`
	Status         string    `json:"status"`       // queued, in_progress, completed
	Conclusion     string    `json:"conclusion"`   // success, failure, neutral, cancelled, timed_out, action_required
	SenderLogin    string    `json:"sender_login"`
	PersonID       *uuid.UUID `json:"person_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// GitHubCheckRunRef is the minimal view the check run event builder needs.
type GitHubCheckRunRef struct {
	InstallationID int64
	Repository     string
	CheckRun       string
	Status         string
	Conclusion     string
	SenderLogin    string
	PersonID       *uuid.UUID
}

// NewGitHubCheckRun builds the check_run event.
func NewGitHubCheckRun(ref GitHubCheckRunRef) (Event, error) {
	return buildEvent(TypeGitHubCheckRunV1, AggregateGitHub, uuid.UUID{}, nil, nil, GitHubCheckRunPayload{
		InstallationID: ref.InstallationID,
		Repository:     ref.Repository,
		CheckRun:       ref.CheckRun,
		Status:         ref.Status,
		Conclusion:     ref.Conclusion,
		SenderLogin:    ref.SenderLogin,
		PersonID:       ref.PersonID,
		CreatedAt:      time.Now().UTC(),
	})
}

// GitHubDeploymentStatusPayload is emitted when a deployment status changes.
type GitHubDeploymentStatusPayload struct {
	InstallationID int64     `json:"installation_id"`
	Repository     string    `json:"repository"`
	DeploymentID   int64     `json:"deployment_id"`
	Environment    string    `json:"environment"`
	Status         string    `json:"status"` // success, failure, inactive, pending
	SenderLogin    string    `json:"sender_login"`
	PersonID       *uuid.UUID `json:"person_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// GitHubDeploymentStatusRef is the minimal view the deployment event builder needs.
type GitHubDeploymentStatusRef struct {
	InstallationID int64
	Repository     string
	DeploymentID   int64
	Environment    string
	Status         string
	SenderLogin    string
	PersonID       *uuid.UUID
}

// NewGitHubDeploymentStatus builds the deployment_status event.
func NewGitHubDeploymentStatus(ref GitHubDeploymentStatusRef) (Event, error) {
	return buildEvent(TypeGitHubDeploymentStatusV1, AggregateGitHub, uuid.UUID{}, nil, nil, GitHubDeploymentStatusPayload{
		InstallationID: ref.InstallationID,
		Repository:     ref.Repository,
		DeploymentID:   ref.DeploymentID,
		Environment:    ref.Environment,
		Status:         ref.Status,
		SenderLogin:    ref.SenderLogin,
		PersonID:       ref.PersonID,
		CreatedAt:      time.Now().UTC(),
	})
}

// GitHubInstallationPayload is emitted when an installation is created or deleted.
type GitHubInstallationPayload struct {
	InstallationID int64     `json:"installation_id"`
	AccountLogin   string    `json:"account_login"`
	AccountType    string    `json:"account_type"`
	Action         string    `json:"action"` // created, deleted
	SenderLogin    string    `json:"sender_login"`
	CreatedAt      time.Time `json:"created_at"`
}

// GitHubInstallationRef is the minimal view the installation event builder needs.
type GitHubInstallationRef struct {
	InstallationID int64
	AccountLogin   string
	AccountType    string
	Action         string
	SenderLogin    string
}

// NewGitHubInstallation builds the installation event.
func NewGitHubInstallation(ref GitHubInstallationRef) (Event, error) {
	return buildEvent(TypeGitHubInstallationV1, AggregateGitHub, uuid.UUID{}, nil, nil, GitHubInstallationPayload{
		InstallationID: ref.InstallationID,
		AccountLogin:   ref.AccountLogin,
		AccountType:    ref.AccountType,
		Action:         ref.Action,
		SenderLogin:    ref.SenderLogin,
		CreatedAt:      time.Now().UTC(),
	})
}
