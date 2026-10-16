package files

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

func pendingFile(uploader uuid.UUID) File {
	id, _ := uuid.NewV7()
	return File{
		ID:             id,
		Bucket:         "atlas-files",
		ObjectKey:      "2026/10/16/" + id.String() + "/report.pdf",
		Filename:       "report.pdf",
		ContentType:    "application/pdf",
		SizeBytes:      2048,
		Classification: Internal,
		Status:         StatusPending,
		UploadedBy:     &uploader,
	}
}

func TestRequestUploadCreatesPendingFile(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	store := newFakeStore()
	objects := &fakeObjects{putURL: "https://r2.example/put", headers: map[string]string{"Content-Type": "application/pdf"}}
	svc := NewService(store, objects, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)

	f, ticket, err := svc.RequestUpload(ctx, actor, UploadRequest{
		Filename: "report.pdf", ContentType: "application/pdf", SizeBytes: 2048,
	})
	if err != nil {
		t.Fatalf("RequestUpload: %v", err)
	}
	if f.Status != StatusPending {
		t.Fatalf("status = %q, want pending", f.Status)
	}
	if store.created != 1 {
		t.Fatalf("store.Create calls = %d, want 1", store.created)
	}
	if ticket.UploadURL != "https://r2.example/put" || ticket.Method != http.MethodPut {
		t.Fatalf("ticket = %+v", ticket)
	}
	if f.ObjectKey == "" || f.UploadedBy == nil || *f.UploadedBy != actor {
		t.Fatalf("file not recorded against the actor: %+v", f)
	}
}

func TestRequestUploadRejectsInvalid(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "b"}, nil)
	if _, _, err := svc.RequestUpload(context.Background(), uuid.New(), UploadRequest{Filename: "", SizeBytes: 1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestCompleteUploadMarksAvailableAndEmitsEvent(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	pending := pendingFile(actor)
	store := newFakeStore(pending)
	objects := &fakeObjects{info: ObjectInfo{SizeBytes: 4096, ContentType: "application/pdf"}}
	svc := NewService(store, objects, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)

	got, err := svc.CompleteUpload(ctx, actor, pending.ID)
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if got.Status != StatusAvailable {
		t.Fatalf("status = %q, want available", got.Status)
	}
	if got.SizeBytes != 4096 {
		t.Fatalf("size = %d, want the verified 4096", got.SizeBytes)
	}
	if store.complete != 1 {
		t.Fatalf("store.Complete calls = %d, want 1", store.complete)
	}
	if store.lastEv == nil || store.lastEv.Type != outbox.TypeFileUploadedV1 {
		t.Fatalf("event = %+v, want %s", store.lastEv, outbox.TypeFileUploadedV1)
	}
	var payload outbox.FileUploadedPayload
	if err := store.lastEv.DecodePayload(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.FileID != pending.ID || payload.SizeBytes != 4096 || payload.UploadedBy != actor {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestCompleteUploadObjectMissing(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	pending := pendingFile(actor)
	store := newFakeStore(pending)
	svc := NewService(store, &fakeObjects{statErr: ErrObjectMissing}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)

	if _, err := svc.CompleteUpload(ctx, actor, pending.ID); !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("err = %v, want ErrObjectMissing", err)
	}
	if store.complete != 0 {
		t.Fatalf("store.Complete called for a missing object")
	}
}

func TestCompleteUploadQuarantinesUncleanObject(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	pending := pendingFile(actor)
	store := newFakeStore(pending)
	objects := &fakeObjects{info: ObjectInfo{SizeBytes: 10, ContentType: "application/pdf"}}
	svc := NewService(store, objects, fakeScanner{res: ScanResult{Clean: false, Detail: "EICAR"}}, nil, Config{Bucket: "atlas-files"}, nil)

	got, err := svc.CompleteUpload(ctx, actor, pending.ID)
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if got.Status != StatusQuarantined {
		t.Fatalf("status = %q, want quarantined", got.Status)
	}
	if store.lastEv != nil {
		t.Fatalf("a quarantined upload must not emit an uploaded event: %+v", store.lastEv)
	}
}

func TestCompleteUploadIsIdempotent(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	file := pendingFile(actor)
	file.Status = StatusAvailable
	store := newFakeStore(file)
	svc := NewService(store, &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)

	got, err := svc.CompleteUpload(ctx, actor, file.ID)
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if got.Status != StatusAvailable || store.complete != 0 {
		t.Fatalf("repeat confirm changed state: %+v (calls=%d)", got, store.complete)
	}
}

func TestCompleteUploadUnknownFile(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	if _, err := svc.CompleteUpload(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDownloadURLRequiresAvailableFile(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	pending := pendingFile(actor)
	store := newFakeStore(pending)
	svc := NewService(store, &fakeObjects{getURL: "https://r2.example/get"}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)

	if _, err := svc.DownloadURL(ctx, pending.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("pending download: err = %v, want ErrConflict", err)
	}

	available := pending
	available.Status = StatusAvailable
	store.rows[available.ID] = available

	ticket, err := svc.DownloadURL(ctx, available.ID)
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if ticket.URL != "https://r2.example/get" {
		t.Fatalf("url = %q", ticket.URL)
	}
}

func TestArchiveOnlyUploaderMayDelete(t *testing.T) {
	ctx := context.Background()
	actor := uuid.New()
	pending := pendingFile(actor)
	store := newFakeStore(pending)
	svc := NewService(store, &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)

	if err := svc.Archive(ctx, uuid.New(), pending.ID, "not mine"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-uploader delete: err = %v, want ErrForbidden", err)
	}
	if store.archived != 0 {
		t.Fatalf("store.Archive called for a non-uploader")
	}

	if err := svc.Archive(ctx, actor, pending.ID, "cleanup"); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if store.archived != 1 {
		t.Fatalf("store.Archive calls = %d, want 1", store.archived)
	}
	if store.lastEv == nil || store.lastEv.Type != outbox.TypeFileDeletedV1 {
		t.Fatalf("event = %+v, want %s", store.lastEv, outbox.TypeFileDeletedV1)
	}
}

func TestGetUnknownFile(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	if _, err := svc.Get(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
