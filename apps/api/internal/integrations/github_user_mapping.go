package integrations

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// UserMapping links a SCOS person to a GitHub numeric user id.
// A person may have multiple GitHub identities; one may be marked primary.
type UserMapping struct {
	ID             uuid.UUID
	PersonID       uuid.UUID
	GitHubUserID   int64
	GitHubLogin    string
	InstallationID *int64
	IsPrimary      bool
	CreatedAt      time.Time
}

// UserMappingInput is the payload for creating a user mapping.
type UserMappingInput struct {
	PersonID       uuid.UUID
	GitHubUserID   int64
	GitHubLogin    string
	InstallationID *int64
	IsPrimary      bool
}

// UserMappingQuery narrows the user mapping list.
type UserMappingQuery struct {
	PersonID       *uuid.UUID
	GitHubUserID   *int64
	InstallationID *int64
}

func scanUserMapping(row rowScanner) (UserMapping, error) {
	var u UserMapping
	var installationID *int64
	err := row.Scan(&u.ID, &u.PersonID, &u.GitHubUserID, &u.GitHubLogin,
		&installationID, &u.IsPrimary, &u.CreatedAt)
	if err != nil {
		return UserMapping{}, err
	}
	u.InstallationID = installationID
	return u, nil
}

const userMappingCols = `
	u.id, u.person_id, u.github_user_id, u.github_login,
	u.installation_id, u.is_primary, u.created_at`

// CreateUserMapping inserts a new GitHub user-to-person mapping.
func (s *PGStore) CreateUserMapping(ctx context.Context, in UserMappingInput) (UserMapping, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return UserMapping{}, fmt.Errorf("new id: %w", err)
	}
	// If this is marked primary, unmark any existing primary for this person.
	if in.IsPrimary {
		if _, err := s.db.Exec(ctx, `
			UPDATE integrations.github_user_mappings
			   SET is_primary = false
			 WHERE person_id = $1`, in.PersonID); err != nil {
			return UserMapping{}, fmt.Errorf("unmark primary: %w", err)
		}
	}
	var u UserMapping
	err = s.db.QueryRow(ctx, `
		INSERT INTO integrations.github_user_mappings (
			id, person_id, github_user_id, github_login,
			installation_id, is_primary, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING `+userMappingCols,
		id, in.PersonID, in.GitHubUserID, in.GitHubLogin,
		in.InstallationID, in.IsPrimary, time.Now().UTC()).
		Scan(&u.ID, &u.PersonID, &u.GitHubUserID, &u.GitHubLogin,
			&u.InstallationID, &u.IsPrimary, &u.CreatedAt)
	if err != nil {
		return UserMapping{}, fmt.Errorf("create user mapping: %w", err)
	}
	return u, nil
}

// GetUserMapping returns one mapping by its SCOS id.
func (s *PGStore) GetUserMapping(ctx context.Context, id uuid.UUID) (*UserMapping, error) {
	u, err := scanUserMapping(s.db.QueryRow(ctx, `SELECT `+userMappingCols+` FROM integrations.github_user_mappings u WHERE u.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserMappingByGitHubID returns the mapping for a GitHub numeric user id.
func (s *PGStore) GetUserMappingByGitHubID(ctx context.Context, githubUserID int64) (*UserMapping, error) {
	u, err := scanUserMapping(s.db.QueryRow(ctx, `SELECT `+userMappingCols+` FROM integrations.github_user_mappings u WHERE u.github_user_id = $1`, githubUserID))
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUserMappings returns mappings matching the query.
func (s *PGStore) ListUserMappings(ctx context.Context, q UserMappingQuery) ([]UserMapping, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.PersonID != nil {
		add("u.person_id = $%d", *q.PersonID)
	}
	if q.GitHubUserID != nil {
		add("u.github_user_id = $%d", *q.GitHubUserID)
	}
	if q.InstallationID != nil {
		add("u.installation_id = $%d", *q.InstallationID)
	}
	rows, err := s.db.Query(ctx, `SELECT `+userMappingCols+` FROM integrations.github_user_mappings u WHERE `+where+
		` ORDER BY u.is_primary DESC, u.created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list user mappings: %w", err)
	}
	defer rows.Close()
	out := []UserMapping{}
	for rows.Next() {
		u, err := scanUserMapping(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetPrimaryMapping marks one mapping as primary for a person, unmarking others.
func (s *PGStore) SetPrimaryMapping(ctx context.Context, id uuid.UUID) (UserMapping, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UserMapping{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		UPDATE integrations.github_user_mappings
		   SET is_primary = false`); err != nil {
		return UserMapping{}, fmt.Errorf("unmark primaries: %w", err)
	}
	var u UserMapping
	u, err = scanUserMapping(tx.QueryRow(ctx, `
		UPDATE integrations.github_user_mappings
		   SET is_primary = true
		 WHERE id = $1
		RETURNING `+userMappingCols, id))
	if err != nil {
		return UserMapping{}, fmt.Errorf("set primary: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return UserMapping{}, fmt.Errorf("commit: %w", err)
	}
	return u, nil
}

// DeleteUserMapping removes a mapping by its SCOS id.
func (s *PGStore) DeleteUserMapping(ctx context.Context, id uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM integrations.github_user_mappings WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user mapping: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
