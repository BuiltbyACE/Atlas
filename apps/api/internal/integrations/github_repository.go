package integrations

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository is a GitHub repository authorized by an installation.
// It may optionally be linked to a SCOS project for task-to-code traceability.
type Repository struct {
	ID            uuid.UUID
	InstallationID int64
	GitHubRepoID  int64
	FullName      string
	Name          string
	IsPrivate     bool
	ProjectID     *uuid.UUID
	LastPushedAt  *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// RepositoryInput is the payload for creating or updating a repository.
type RepositoryInput struct {
	InstallationID int64
	GitHubRepoID   int64
	FullName       string
	Name           string
	IsPrivate      bool
	ProjectID      *uuid.UUID
}

// RepositoryQuery narrows the repository list.
type RepositoryQuery struct {
	InstallationID *int64
	ProjectID      *uuid.UUID
	FullName       string
}

func scanRepository(row rowScanner) (Repository, error) {
	var r Repository
	var projectID *uuid.UUID
	var lastPushedAt *time.Time
	err := row.Scan(&r.ID, &r.InstallationID, &r.GitHubRepoID, &r.FullName,
		&r.Name, &r.IsPrivate, &projectID, &lastPushedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Repository{}, err
	}
	r.ProjectID = projectID
	r.LastPushedAt = lastPushedAt
	return r, nil
}

const repositoryCols = `
	r.id, r.installation_id, r.github_repo_id, r.full_name, r.name,
	r.is_private, r.project_id, r.last_pushed_at, r.created_at, r.updated_at`

// CreateRepository inserts a new authorized repository.
func (s *PGStore) CreateRepository(ctx context.Context, in RepositoryInput) (Repository, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return Repository{}, fmt.Errorf("new id: %w", err)
	}
	var r Repository
	err = s.db.QueryRow(ctx, `
		INSERT INTO integrations.github_repositories (
			id, installation_id, github_repo_id, full_name, name, is_private,
			project_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		RETURNING `+repositoryCols,
		id, in.InstallationID, in.GitHubRepoID, in.FullName, in.Name,
		in.IsPrivate, in.ProjectID, time.Now().UTC()).
		Scan(&r.ID, &r.InstallationID, &r.GitHubRepoID, &r.FullName, &r.Name,
			&r.IsPrivate, &r.ProjectID, &r.LastPushedAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Repository{}, fmt.Errorf("create repository: %w", err)
	}
	return r, nil
}

// GetRepository returns one repository by its SCOS id.
func (s *PGStore) GetRepository(ctx context.Context, id uuid.UUID) (*Repository, error) {
	r, err := scanRepository(s.db.QueryRow(ctx, `SELECT `+repositoryCols+` FROM integrations.github_repositories r WHERE r.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetRepositoryByGitHubID returns a repository by GitHub's numeric repo id within an installation.
func (s *PGStore) GetRepositoryByGitHubID(ctx context.Context, installationID int64, githubRepoID int64) (*Repository, error) {
	r, err := scanRepository(s.db.QueryRow(ctx, `SELECT `+repositoryCols+` FROM integrations.github_repositories r WHERE r.installation_id = $1 AND r.github_repo_id = $2`, installationID, githubRepoID))
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRepositories returns repositories matching the query.
func (s *PGStore) ListRepositories(ctx context.Context, q RepositoryQuery) ([]Repository, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.InstallationID != nil {
		add("r.installation_id = $%d", *q.InstallationID)
	}
	if q.ProjectID != nil {
		add("r.project_id = $%d", *q.ProjectID)
	}
	if search := trim(q.FullName); search != "" {
		add("(r.full_name ILIKE '%%' || $%[1]d || '%%' OR r.name ILIKE '%%' || $%[1]d || '%%')", search)
	}
	rows, err := s.db.Query(ctx, `SELECT `+repositoryCols+` FROM integrations.github_repositories r WHERE `+where+
		` ORDER BY r.full_name ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	defer rows.Close()
	out := []Repository{}
	for rows.Next() {
		r, err := scanRepository(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LinkRepositoryToProject associates a repository with a SCOS project.
func (s *PGStore) LinkRepositoryToProject(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (Repository, error) {
	r, err := scanRepository(s.db.QueryRow(ctx, `
		UPDATE integrations.github_repositories
		   SET project_id = $2, updated_at = $3
		 WHERE id = $1
		RETURNING `+repositoryCols, id, projectID, time.Now().UTC()))
	if err != nil {
		return Repository{}, err
	}
	return r, nil
}

// UpdateLastPushedAt records the most recent push timestamp for a repository.
func (s *PGStore) UpdateLastPushedAt(ctx context.Context, id uuid.UUID, pushedAt time.Time) (Repository, error) {
	r, err := scanRepository(s.db.QueryRow(ctx, `
		UPDATE integrations.github_repositories
		   SET last_pushed_at = $2, updated_at = $3
		 WHERE id = $1
		RETURNING `+repositoryCols, id, pushedAt, time.Now().UTC()))
	if err != nil {
		return Repository{}, err
	}
	return r, nil
}
