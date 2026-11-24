package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// InstallationType is the kind of GitHub account the App is installed on.
type InstallationType string

const (
	InstallationTypePersonal    InstallationType = "personal"
	InstallationTypeOrganization InstallationType = "organization"
)

// InstallationStatus is the operational status of a GitHub App installation.
type InstallationStatus string

const (
	InstallationActive    InstallationStatus = "active"
	InstallationSuspended InstallationStatus = "suspended"
	InstallationRevoked   InstallationStatus = "revoked"
)

// validInstallationType reports whether t is a known installation type.
func validInstallationType(t InstallationType) bool {
	switch t {
	case InstallationTypePersonal, InstallationTypeOrganization:
		return true
	}
	return false
}

// validInstallationStatus reports whether s is a known installation status.
func validInstallationStatus(s InstallationStatus) bool {
	switch s {
	case InstallationActive, InstallationSuspended, InstallationRevoked:
		return true
	}
	return false
}

// Installation is a GitHub App installation on a user or organization account.
// One row per installation; person_id links the installation to the SCOS
// employee who installed it.
type Installation struct {
	ID               uuid.UUID
	InstallationID   int64
	AccountType      InstallationType
	AccountLogin     string
	AccountID        int64
	PersonID         *uuid.UUID
	Status           InstallationStatus
	AuthorizedRepos  []string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	RevokedAt        *time.Time
}

// InstallationInput is the payload for creating or updating an installation.
type InstallationInput struct {
	InstallationID  int64
	AccountType     InstallationType
	AccountLogin    string
	AccountID       int64
	PersonID        *uuid.UUID
	Status          InstallationStatus
	AuthorizedRepos []string
}

// InstallationQuery narrows the installations list.
type InstallationQuery struct {
	Status     *InstallationStatus
	PersonID   *uuid.UUID
	AccountType *InstallationType
	Search     string
}

func scanInstallation(row rowScanner) (Installation, error) {
	var i Installation
	var personID *uuid.UUID
	var revokedAt *time.Time
	var repos []byte
	var accountType, status string
	err := row.Scan(&i.ID, &i.InstallationID, &accountType, &i.AccountLogin,
		&i.AccountID, &personID, &status, &repos, &i.CreatedAt, &i.UpdatedAt, &revokedAt)
	if err != nil {
		return Installation{}, err
	}
	i.AccountType = InstallationType(accountType)
	i.Status = InstallationStatus(status)
	i.PersonID = personID
	i.RevokedAt = revokedAt
	if err := json.Unmarshal(repos, &i.AuthorizedRepos); err != nil {
		return Installation{}, err
	}
	return i, nil
}

const installationCols = `
	i.id, i.installation_id, i.account_type, i.account_login, i.account_id,
	i.person_id, i.status, i.authorized_repos, i.created_at, i.updated_at, i.revoked_at`

// CreateInstallation inserts a new GitHub App installation.
func (s *PGStore) CreateInstallation(ctx context.Context, in InstallationInput) (Installation, error) {
	if !validInstallationType(in.AccountType) {
		return Installation{}, errInvalidField("account_type")
	}
	if !validInstallationStatus(in.Status) {
		in.Status = InstallationActive
	}
	repos, err := json.Marshal(in.AuthorizedRepos)
	if err != nil {
		return Installation{}, fmt.Errorf("marshal authorized_repos: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Installation{}, fmt.Errorf("new id: %w", err)
	}
	var accountType, status string
	var i Installation
	err = s.db.QueryRow(ctx, `
		INSERT INTO integrations.github_installations (
			id, installation_id, account_type, account_login, account_id,
			person_id, status, authorized_repos, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		RETURNING `+installationCols,
		id, in.InstallationID, string(in.AccountType), in.AccountLogin, in.AccountID,
		in.PersonID, string(in.Status), repos, time.Now().UTC()).
		Scan(&i.ID, &i.InstallationID, &accountType, &i.AccountLogin, &i.AccountID,
			&i.PersonID, &status, &i.AuthorizedRepos, &i.CreatedAt, &i.UpdatedAt, &i.RevokedAt)
	if err != nil {
		return Installation{}, fmt.Errorf("create installation: %w", err)
	}
	i.AccountType = InstallationType(accountType)
	i.Status = InstallationStatus(status)
	return i, nil
}

// GetInstallation returns one installation by its SCOS id.
func (s *PGStore) GetInstallation(ctx context.Context, id uuid.UUID) (*Installation, error) {
	i, err := scanInstallation(s.db.QueryRow(ctx, `SELECT `+installationCols+` FROM integrations.github_installations i WHERE i.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// GetInstallationByGitHubID returns one installation by GitHub's numeric id.
func (s *PGStore) GetInstallationByGitHubID(ctx context.Context, installationID int64) (*Installation, error) {
	i, err := scanInstallation(s.db.QueryRow(ctx, `SELECT `+installationCols+` FROM integrations.github_installations i WHERE i.installation_id = $1`, installationID))
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// ListInstallations returns a keyset page of installations.
func (s *PGStore) ListInstallations(ctx context.Context, q InstallationQuery, limit int, before *Cursor) ([]Installation, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.Status != nil {
		add("i.status = $%d", string(*q.Status))
	}
	if q.PersonID != nil {
		add("i.person_id = $%d", *q.PersonID)
	}
	if q.AccountType != nil {
		add("i.account_type = $%d", string(*q.AccountType))
	}
	if search := trim(q.Search); search != "" {
		add("(i.account_login ILIKE '%%' || $%[1]d || '%%' OR i.installation_id::text ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (i.created_at, i.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+installationCols+` FROM integrations.github_installations i WHERE `+where+
		` ORDER BY i.created_at DESC, i.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list installations: %w", err)
	}
	defer rows.Close()
	out := []Installation{}
	for rows.Next() {
		i, err := scanInstallation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// UpdateInstallationStatus changes an installation's status.
func (s *PGStore) UpdateInstallationStatus(ctx context.Context, id uuid.UUID, status InstallationStatus) (Installation, error) {
	if !validInstallationStatus(status) {
		return Installation{}, errInvalidField("status")
	}
	now := time.Now().UTC()
	var revokedAt *time.Time
	if status == InstallationRevoked {
		revokedAt = &now
	}
	i, err := scanInstallation(s.db.QueryRow(ctx, `
		UPDATE integrations.github_installations
		   SET status = $2, revoked_at = $3, updated_at = $4
		 WHERE id = $1
		RETURNING `+installationCols, id, string(status), revokedAt, now))
	if err != nil {
		return Installation{}, err
	}
	return i, nil
}

// LinkPersonToInstallation associates a SCOS person with an installation.
func (s *PGStore) LinkPersonToInstallation(ctx context.Context, id uuid.UUID, personID uuid.UUID) (Installation, error) {
	i, err := scanInstallation(s.db.QueryRow(ctx, `
		UPDATE integrations.github_installations
		   SET person_id = $2, updated_at = $3
		 WHERE id = $1
		RETURNING `+installationCols, id, personID, time.Now().UTC()))
	if err != nil {
		return Installation{}, err
	}
	return i, nil
}
