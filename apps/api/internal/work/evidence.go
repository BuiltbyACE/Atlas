package work

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// AttachEvidence links a stored file to a task or project. The file must be
// available (not pending, quarantined or deleted); when a task is named the
// project is derived from it so project-level listings see the evidence too.
// Emits work.evidence.added.v1 and writes an audit row.
func (s *Service) AttachEvidence(ctx context.Context, actor uuid.UUID, in EvidenceInput) (Evidence, error) {
	if in.FileID == uuid.Nil {
		return Evidence{}, fmt.Errorf("%w: file_id is required", ErrInvalid)
	}
	if (in.TaskID == nil) == (in.ProjectID == nil) {
		return Evidence{}, fmt.Errorf("%w: provide exactly one of task_id or project_id", ErrInvalid)
	}

	var projectID uuid.UUID
	if in.TaskID != nil {
		task, err := s.GetTask(ctx, *in.TaskID)
		if err != nil {
			return Evidence{}, err
		}
		projectID = task.ProjectID
	} else {
		p, err := s.GetProject(ctx, *in.ProjectID)
		if err != nil {
			return Evidence{}, err
		}
		projectID = p.ID
	}

	if s.files != nil {
		file, err := s.files.ResolveFile(ctx, in.FileID)
		if err != nil {
			return Evidence{}, err
		}
		if file == nil {
			return Evidence{}, fmt.Errorf("%w: file %s", ErrNotFound, in.FileID)
		}
		if file.Status != FileStatusAvailable {
			return Evidence{}, fmt.Errorf("%w: file is %s, only available files can be evidence", ErrConflict, file.Status)
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Evidence{}, fmt.Errorf("work: new id: %w", err)
	}
	e := Evidence{
		ID:          id,
		TaskID:      in.TaskID,
		ProjectID:   &projectID,
		FileID:      in.FileID,
		UploaderID:  actor,
		Kind:        trim(in.Kind),
		Description: trim(in.Description),
		CreatedAt:   s.now(),
	}

	ev, err := outbox.NewEvidenceAdded(&actor, e.ID, e.FileID, uuidOrZero(e.TaskID), uuidOrZero(e.ProjectID), e.Kind, actor)
	if err != nil {
		return Evidence{}, err
	}
	created, err := s.store.CreateEvidence(ctx, e, &ev)
	if err != nil {
		return Evidence{}, err
	}
	s.writeAudit(ctx, actor, "work.evidence.added", EntityEvidence, created.ID, nil, evidenceSnapshot(created))
	return created, nil
}

// GetEvidence returns evidence or ErrNotFound.
func (s *Service) GetEvidence(ctx context.Context, id uuid.UUID) (Evidence, error) {
	e, err := s.store.GetEvidence(ctx, id)
	if err != nil {
		return Evidence{}, err
	}
	if e == nil {
		return Evidence{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *e, nil
}

// ListEvidence returns the evidence attached to a task or project, newest first.
func (s *Service) ListEvidence(ctx context.Context, q ListEvidenceQuery) ([]Evidence, error) {
	if q.TaskID == nil && q.ProjectID == nil {
		return nil, fmt.Errorf("%w: task_id or project_id is required", ErrInvalid)
	}
	return s.store.ListEvidence(ctx, q)
}

// VerifyEvidence marks evidence as reviewed by a distinct person (a submitter
// cannot verify their own evidence; that is separation of duties, §3.3).
func (s *Service) VerifyEvidence(ctx context.Context, actor, evidenceID uuid.UUID) (Evidence, error) {
	current, err := s.GetEvidence(ctx, evidenceID)
	if err != nil {
		return Evidence{}, err
	}
	if current.UploaderID == actor {
		return Evidence{}, fmt.Errorf("%w: you cannot verify evidence you submitted", ErrForbidden)
	}
	verified, err := s.store.VerifyEvidence(ctx, evidenceID, actor)
	if err != nil {
		return Evidence{}, err
	}
	s.writeAudit(ctx, actor, "work.evidence.verified", EntityEvidence, verified.ID,
		evidenceSnapshot(current), evidenceSnapshot(verified))
	return verified, nil
}

// DeleteEvidence removes an evidence link. The underlying file is untouched
// (deleting it is the files module's concern).
func (s *Service) DeleteEvidence(ctx context.Context, actor, evidenceID uuid.UUID) error {
	current, err := s.GetEvidence(ctx, evidenceID)
	if err != nil {
		return err
	}
	if err := s.store.DeleteEvidence(ctx, evidenceID); err != nil {
		return err
	}
	s.writeAudit(ctx, actor, "work.evidence.removed", EntityEvidence, evidenceID, evidenceSnapshot(current), nil)
	return nil
}

func evidenceSnapshot(e Evidence) map[string]any {
	return map[string]any{
		"task_id":     uuidOrZero(e.TaskID).String(),
		"project_id":  uuidOrZero(e.ProjectID).String(),
		"file_id":     e.FileID.String(),
		"kind":        e.Kind,
		"description": e.Description,
		"verified":    e.VerifiedAt != nil,
	}
}
