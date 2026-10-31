package systems

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The directory reads: the domains, certificates and credential records behind
// the expiry warnings (§8.6). They were not part of the read-first slice because
// the overview could answer "what expires soon" without them; the write pass
// needs the rows themselves so a record can be edited.
//
// A credential record is the pointer, never the secret: `store_path` names where
// the value lives in the secrets manager and is the only location field that
// exists.

const domainCols = `d.id, d.name, d.system_id, COALESCE(sys.name, ''), COALESCE(d.registrar, ''),
	d.registered_on, d.expires_on, d.auto_renew, d.owner_person_id, COALESCE(owner.full_name, ''),
	COALESCE(d.notes, ''), d.version, d.created_at, d.updated_at`

const domainJoins = `
	LEFT JOIN systems.systems sys ON sys.id = d.system_id
	LEFT JOIN people.persons owner ON owner.id = d.owner_person_id`

func scanDomain(row rowScanner) (Domain, error) {
	var d Domain
	err := row.Scan(&d.ID, &d.Name, &d.SystemID, &d.SystemName, &d.Registrar,
		&d.RegisteredOn, &d.ExpiresOn, &d.AutoRenew, &d.OwnerPersonID, &d.OwnerName,
		&d.Notes, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *PGStore) ListDomains(ctx context.Context, q DomainQuery, limit int, before *Cursor) ([]Domain, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.SystemID != nil {
		add("d.system_id = $%d", *q.SystemID)
	}
	if search := trim(q.Search); search != "" {
		add("(d.name ILIKE '%%' || $%[1]d || '%%' OR COALESCE(d.registrar, '') ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (d.created_at, d.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+domainCols+` FROM systems.domains d`+domainJoins+
		` WHERE `+where+` ORDER BY d.created_at DESC, d.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("systems: list domains: %w", err)
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("systems: scan domain: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PGStore) GetDomain(ctx context.Context, id uuid.UUID) (*Domain, error) {
	d, err := scanDomain(s.db.QueryRow(ctx, `SELECT `+domainCols+` FROM systems.domains d`+
		domainJoins+` WHERE d.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("systems: get domain: %w", err)
	}
	return &d, nil
}

const certificateCols = `c.id, c.hostname, c.system_id, COALESCE(sys.name, ''), COALESCE(c.issuer, ''),
	c.issued_on, c.expires_on, c.auto_renew, c.owner_person_id, COALESCE(owner.full_name, ''),
	COALESCE(c.notes, ''), c.version, c.created_at, c.updated_at`

const certificateJoins = `
	LEFT JOIN systems.systems sys ON sys.id = c.system_id
	LEFT JOIN people.persons owner ON owner.id = c.owner_person_id`

func scanCertificate(row rowScanner) (Certificate, error) {
	var c Certificate
	err := row.Scan(&c.ID, &c.Hostname, &c.SystemID, &c.SystemName, &c.Issuer,
		&c.IssuedOn, &c.ExpiresOn, &c.AutoRenew, &c.OwnerPersonID, &c.OwnerName,
		&c.Notes, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *PGStore) ListCertificates(ctx context.Context, q CertificateQuery, limit int, before *Cursor) ([]Certificate, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.SystemID != nil {
		add("c.system_id = $%d", *q.SystemID)
	}
	if search := trim(q.Search); search != "" {
		add("(c.hostname ILIKE '%%' || $%[1]d || '%%' OR COALESCE(c.issuer, '') ILIKE '%%' || $%[1]d || '%%')", search)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (c.created_at, c.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+certificateCols+` FROM systems.certificates c`+certificateJoins+
		` WHERE `+where+` ORDER BY c.created_at DESC, c.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("systems: list certificates: %w", err)
	}
	defer rows.Close()
	out := []Certificate{}
	for rows.Next() {
		c, err := scanCertificate(rows)
		if err != nil {
			return nil, fmt.Errorf("systems: scan certificate: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PGStore) GetCertificate(ctx context.Context, id uuid.UUID) (*Certificate, error) {
	c, err := scanCertificate(s.db.QueryRow(ctx, `SELECT `+certificateCols+` FROM systems.certificates c`+
		certificateJoins+` WHERE c.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("systems: get certificate: %w", err)
	}
	return &c, nil
}

// ListAllCredentials lists credential records across the register, newest
// first. The per-system read stays the one the system detail uses.
func (s *PGStore) ListAllCredentials(ctx context.Context, q CredentialQuery, limit int, before *Cursor) ([]CredentialRecord, error) {
	args := []any{}
	where := "TRUE"
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND "+cond, len(args))
	}
	if q.SystemID != nil {
		add("cr.system_id = $%d", *q.SystemID)
	}
	if q.Active != nil {
		add("cr.active = $%d", *q.Active)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (cr.created_at, cr.id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+credentialCols+` FROM systems.credentials cr
		LEFT JOIN people.persons owner ON owner.id = cr.owner_person_id
		WHERE `+where+` ORDER BY cr.created_at DESC, cr.id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("systems: list credentials: %w", err)
	}
	defer rows.Close()
	out := []CredentialRecord{}
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("systems: scan credential: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PGStore) GetCredential(ctx context.Context, id uuid.UUID) (*CredentialRecord, error) {
	c, err := scanCredential(s.db.QueryRow(ctx, `SELECT `+credentialCols+` FROM systems.credentials cr
		LEFT JOIN people.persons owner ON owner.id = cr.owner_person_id WHERE cr.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("systems: get credential: %w", err)
	}
	return &c, nil
}
