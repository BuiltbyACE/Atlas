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

	// EventPush is the GitHub push event type.
	EventPush = "push"
	// EventCheckRun is the GitHub check_run event type.
	EventCheckRun = "check_run"
	// EventDeploymentStatus is the GitHub deployment_status event type.
	EventDeploymentStatus = "deployment_status"
	// EventInstallation is the GitHub installation event type.
	EventInstallation = "installation"

	// ProviderGitHub is the provider id stored with a delivery.
	ProviderGitHub = "github"
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

// githubPush is the subset of GitHub's push payload Atlas reads.
type githubPush struct {
	Ref     string `json:"ref"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"commits"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
}

// ParsePush decodes the push subset of a GitHub delivery.
func ParsePush(body []byte) (Push, error) {
	var payload githubPush
	if err := json.Unmarshal(body, &payload); err != nil {
		return Push{}, fmt.Errorf("%w: decode push: %v", ErrInvalid, err)
	}
	return Push{
		Ref:        payload.Ref,
		Before:     payload.Before,
		After:      payload.After,
		Commits:    len(payload.Commits),
		Repository: payload.Repository.FullName,
		Sender:     payload.Sender.Login,
		SenderID:   payload.Sender.ID,
	}, nil
}

// Push is the subset of a GitHub push payload Atlas acts on.
type Push struct {
	Ref        string
	Before     string
	After      string
	Commits    int
	Repository string
	Sender     string
	SenderID   int64
}

// githubPullRequest is the subset of GitHub's pull_request payload Atlas reads;
// unknown fields are ignored. (Already defined above.)
// githubCheckRun is the subset of GitHub's check_run payload Atlas reads.
type githubCheckRun struct {
	CheckRun struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"check_run"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
}

// ParseCheckRun decodes the check_run subset of a GitHub delivery.
func ParseCheckRun(body []byte) (CheckRun, error) {
	var payload githubCheckRun
	if err := json.Unmarshal(body, &payload); err != nil {
		return CheckRun{}, fmt.Errorf("%w: decode check_run: %v", ErrInvalid, err)
	}
	return CheckRun{
		Name:       payload.CheckRun.Name,
		Status:     payload.CheckRun.Status,
		Conclusion: payload.CheckRun.Conclusion,
		Repository: payload.Repository.FullName,
		Sender:     payload.Sender.Login,
		SenderID:   payload.Sender.ID,
	}, nil
}

// CheckRun is the subset of a GitHub check_run payload Atlas acts on.
type CheckRun struct {
	Name       string
	Status     string
	Conclusion string
	Repository string
	Sender     string
	SenderID   int64
}

// githubDeploymentStatus is the subset of GitHub's deployment_status payload Atlas reads.
type githubDeploymentStatus struct {
	Deployment struct {
		ID   int64  `json:"id"`
		Environment string `json:"environment"`
	} `json:"deployment"`
	DeploymentStatus struct {
		State string `json:"state"`
	} `json:"deployment_status"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
}

// ParseDeploymentStatus decodes the deployment_status subset of a GitHub delivery.
func ParseDeploymentStatus(body []byte) (DeploymentStatus, error) {
	var payload githubDeploymentStatus
	if err := json.Unmarshal(body, &payload); err != nil {
		return DeploymentStatus{}, fmt.Errorf("%w: decode deployment_status: %v", ErrInvalid, err)
	}
	return DeploymentStatus{
		DeploymentID: payload.Deployment.ID,
		Environment:  payload.Deployment.Environment,
		State:        payload.DeploymentStatus.State,
		Repository:   payload.Repository.FullName,
		Sender:       payload.Sender.Login,
		SenderID:     payload.Sender.ID,
	}, nil
}

// DeploymentStatus is the subset of a GitHub deployment_status payload Atlas acts on.
type DeploymentStatus struct {
	DeploymentID  int64
	Environment   string
	State         string
	Repository    string
	Sender        string
	SenderID      int64
}

// githubInstallation is the subset of GitHub's installation payload Atlas reads.
type githubInstallation struct {
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Account struct {
		Login string `json:"login"`
		Type  string `json:"type"`
		ID    int64  `json:"id"`
	} `json:"account"`
	Sender struct {
		Login string `json:"login"`
		ID    int64  `json:"id"`
	} `json:"sender"`
	Action string `json:"action"`
}

// ParseInstallation decodes the installation subset of a GitHub delivery.
func ParseInstallation(body []byte) (InstallationEvent, error) {
	var payload githubInstallation
	if err := json.Unmarshal(body, &payload); err != nil {
		return InstallationEvent{}, fmt.Errorf("%w: decode installation: %v", ErrInvalid, err)
	}
	return InstallationEvent{
		InstallationID: payload.Installation.ID,
		AccountLogin:   payload.Account.Login,
		AccountType:    payload.Account.Type,
		AccountID:      payload.Account.ID,
		Action:         payload.Action,
		Sender:         payload.Sender.Login,
		SenderID:       payload.Sender.ID,
	}, nil
}

// InstallationEvent is the subset of a GitHub installation payload Atlas acts on.
type InstallationEvent struct {
	InstallationID int64
	AccountLogin   string
	AccountType    string
	AccountID      int64
	Action         string
	Sender         string
	SenderID       int64
}
