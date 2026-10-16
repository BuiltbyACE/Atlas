package outbox

import (
	"time"

	"github.com/google/uuid"
)

// File event types (§6.5, §7.4). An upload and a deletion are recorded facts;
// the orchestrator reacts (for example, notifying a watcher) without Go calling
// it directly (ADR-009).
const (
	TypeFileUploadedV1 = "core.file.uploaded.v1"
	TypeFileDeletedV1  = "core.file.deleted.v1"
)

const AggregateFile = "file"

// FileUploadedPayload is emitted when an upload is confirmed and the object is
// available for download.
type FileUploadedPayload struct {
	FileID         uuid.UUID `json:"file_id"`
	Filename       string    `json:"filename"`
	ContentType    string    `json:"content_type"`
	SizeBytes      int64     `json:"size_bytes"`
	Classification string    `json:"classification"`
	EntityType     string    `json:"entity_type,omitempty"`
	EntityID       uuid.UUID `json:"entity_id,omitempty"`
	UploadedBy     uuid.UUID `json:"uploaded_by,omitempty"`
	UploadedAt     time.Time `json:"uploaded_at"`
}

// FileDeletedPayload is emitted when a file is archived.
type FileDeletedPayload struct {
	FileID    uuid.UUID `json:"file_id"`
	DeletedBy uuid.UUID `json:"deleted_by,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	DeletedAt time.Time `json:"deleted_at"`
}

// NewFileUploaded builds a core.file.uploaded.v1 event from a minimal view. The
// outbox package must not import the files domain, so the caller passes
// primitives.
func NewFileUploaded(actorID *uuid.UUID, f FileRef) (Event, error) {
	return buildEvent(TypeFileUploadedV1, AggregateFile, f.ID, actorID, nil, FileUploadedPayload{
		FileID:         f.ID,
		Filename:       f.Filename,
		ContentType:    f.ContentType,
		SizeBytes:      f.SizeBytes,
		Classification: f.Classification,
		EntityType:     f.EntityType,
		EntityID:       f.EntityID,
		UploadedBy:     f.UploadedBy,
		UploadedAt:     time.Now().UTC(),
	})
}

// NewFileDeleted builds a core.file.deleted.v1 event.
func NewFileDeleted(actorID *uuid.UUID, id uuid.UUID, reason string, deletedBy uuid.UUID) (Event, error) {
	return buildEvent(TypeFileDeletedV1, AggregateFile, id, actorID, nil, FileDeletedPayload{
		FileID:    id,
		DeletedBy: deletedBy,
		Reason:    reason,
		DeletedAt: time.Now().UTC(),
	})
}

// FileRef is the minimal file view the event builders need.
type FileRef struct {
	ID             uuid.UUID
	Filename       string
	ContentType    string
	SizeBytes      int64
	Classification string
	EntityType     string
	EntityID       uuid.UUID
	UploadedBy     uuid.UUID
}
