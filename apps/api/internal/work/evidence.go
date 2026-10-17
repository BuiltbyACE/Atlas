package work

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// AttachEvidence links submitted evidence to a task or project on behalf of a
// person. A `file` (the default EvidenceType) must name an available stored
// file; any other §6.5 type must carry a Reference. When a task is named the
// project is derived so project-level listings see the evidence too. Emits
// work.evidence.added.v1 and writes an audit row.
func (s *Service) AttachEvidence(ctx context.Context, actor uuid.UUID, in EvidenceInput) (Evidence, error) {
	return s.attachEvidence(ctx, &actor, in)
}

// AttachIntegrationEvidence records evidence an external system produced (D6: a
// merged pull request). There is no Atlas submitter, so the row's uploader is
// null; a person may still verify it (a nil uploader never equals an actor).
func (s *Service) AttachIntegrationEvidence(ctx context.Context, in EvidenceInput) (Evidence, error) {
	return s.attachEvidence(ctx, nil, in)
}

func (s *Service) attachEvidence(ctx context.Context, uploader *uuid.UUID, in EvidenceInput) (Evidence, error) {
	evidenceType := strings.TrimSpace(in.EvidenceType)
	if evidenceType == "" {
		evidenceType = EvidenceTypeFile
	}
	if !ValidEvidenceType(evidenceType) {
		return Evidence{}, fmt.Errorf("%w: evidence_type must be one of %s", ErrInvalid, strings.Join(EvidenceTypes, ", "))
	}
	reference := strings.TrimSpace(in.Reference)
	if evidenceType == EvidenceTypeFile {
		if in.FileID == uuid.Nil {
			return Evidence{}, fmt.Errorf("%w: file_id is required for file evidence", ErrInvalid)
		}
	} else if reference == "" {
		return Evidence{}, fmt.Errorf("%w: reference is required for %s evidence", ErrInvalid, evidenceType)
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

	var fileID *uuid.UUID
	if evidenceType == EvidenceTypeFile {
		fileID = &in.FileID
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
	}

	// An external link is one row per reference: a redelivered webhook (or a
	// person re-attaching the same PR URL) reuses the row rather than
	// duplicating it. File evidence is keyed by its file, not a reference.
	if evidenceType != EvidenceTypeFile && in.TaskID != nil {
		existing, err := s.store.EvidenceByReference(ctx, *in.TaskID, evidenceType, reference)
		if err != nil {
			return Evidence{}, err
		}
		if existing != nil {
			return *existing, nil
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Evidence{}, fmt.Errorf("work: new id: %w", err)
	}
	e := Evidence{
		ID:           id,
		TaskID:       in.TaskID,
		ProjectID:    &projectID,
		FileID:       fileID,
		UploaderID:   uploader,
		EvidenceType: evidenceType,
		Reference:    reference,
		Kind:         trim(in.Kind),
		Description:  trim(in.Description),
		CreatedAt:    s.now(),
	}

	ev, err := outbox.NewEvidenceAdded(uploader, outbox.EvidenceRef{
		ID:           e.ID,
		TaskID:       uuidOrZero(e.TaskID),
		ProjectID:    projectID,
		FileID:       e.FileID,
		UploaderID:   uploader,
		EvidenceType: evidenceType,
		Reference:    reference,
		Kind:         e.Kind,
	})
	if err != nil {
		return Evidence{}, err
	}
	created, err := s.store.CreateEvidence(ctx, e, &ev)
	if err != nil {
		return Evidence{}, err
	}
	s.writeEvidenceAudit(ctx, uploader, "work.evidence.added", created.ID, nil, evidenceSnapshot(created))
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
	if current.UploaderID != nil && *current.UploaderID == actor {
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

// TaskIDByRef resolves a project/task ref (PRJ-2026-007/T-0142) to its task. It
// is the D6 link: a branch or PR title names the ref and the webhook resolves
// it here. found is false when no task carries the ref.
func (s *Service) TaskIDByRef(ctx context.Context, ref string) (uuid.UUID, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return uuid.Nil, false, nil
	}
	return s.store.TaskIDByRef(ctx, ref)
}

// writeEvidenceAudit records the change against the uploader, or as a service
// when an integration (no person) recorded it — an honest actor trail rather
// than a nil person.
func (s *Service) writeEvidenceAudit(ctx context.Context, uploader *uuid.UUID, action string, entityID uuid.UUID, before, after any) {
	if uploader != nil {
		s.writeAudit(ctx, *uploader, action, EntityEvidence, entityID, before, after)
		return
	}
	s.writeAuditAs(ctx, audit.ActorService, "integration:github", action, EntityEvidence, entityID, before, after)
}

func evidenceSnapshot(e Evidence) map[string]any {
	return map[string]any{
		"task_id":       uuidOrZero(e.TaskID).String(),
		"project_id":    uuidOrZero(e.ProjectID).String(),
		"file_id":       optionalUUID(e.FileID),
		"evidence_type": e.EvidenceType,
		"reference":     e.Reference,
		"kind":          e.Kind,
		"description":   e.Description,
		"uploaded_by":   optionalUUID(e.UploaderID),
		"verified":      e.VerifiedAt != nil,
	}
}

func optionalUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
