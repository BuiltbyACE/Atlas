package files

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// fakeStore is an in-memory Store for service tests.
type fakeStore struct {
	rows     map[uuid.UUID]File
	created  int
	complete int
	archived int
	lastEv   *outbox.Event

	createErr   error
	getErr      error
	completeErr error
	archiveErr  error
}

func newFakeStore(initial ...File) *fakeStore {
	s := &fakeStore{rows: make(map[uuid.UUID]File, len(initial))}
	for _, f := range initial {
		s.rows[f.ID] = f
	}
	return s
}

func (s *fakeStore) Create(_ context.Context, f File) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.rows[f.ID] = f
	s.created++
	return nil
}

func (s *fakeStore) Get(_ context.Context, id uuid.UUID) (*File, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	f, ok := s.rows[id]
	if !ok {
		return nil, nil
	}
	return &f, nil
}

func (s *fakeStore) List(_ context.Context, _ ListQuery, _ int, _ *Cursor) ([]File, error) {
	out := make([]File, 0, len(s.rows))
	for _, f := range s.rows {
		out = append(out, f)
	}
	return out, nil
}

func (s *fakeStore) Complete(_ context.Context, id uuid.UUID, in ObjectInfo, status Status, ev *outbox.Event) (*File, error) {
	if s.completeErr != nil {
		return nil, s.completeErr
	}
	f, ok := s.rows[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if f.Status != StatusPending {
		return nil, fmt.Errorf("%w: file is %s", ErrConflict, f.Status)
	}
	f.Status = status
	f.SizeBytes = in.SizeBytes
	f.ContentType = in.ContentType
	f.ChecksumSHA256 = in.ChecksumSHA256
	s.rows[id] = f
	s.complete++
	s.lastEv = ev
	return &f, nil
}

func (s *fakeStore) Archive(_ context.Context, id uuid.UUID, ev *outbox.Event) (*File, error) {
	if s.archiveErr != nil {
		return nil, s.archiveErr
	}
	f, ok := s.rows[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	f.Status = StatusDeleted
	s.rows[id] = f
	s.archived++
	s.lastEv = ev
	return &f, nil
}

// fakeObjects is an in-memory ObjectStore for service tests.
type fakeObjects struct {
	putURL  string
	getURL  string
	headers map[string]string
	info    ObjectInfo

	putErr    error
	getErr    error
	statErr   error
	deleteErr error
	deleted   []string
}

func (o *fakeObjects) PresignPut(_ context.Context, _, _ string, _ time.Duration) (string, map[string]string, error) {
	if o.putErr != nil {
		return "", nil, o.putErr
	}
	return o.putURL, o.headers, nil
}

func (o *fakeObjects) PresignGet(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	if o.getErr != nil {
		return "", o.getErr
	}
	return o.getURL, nil
}

func (o *fakeObjects) Stat(_ context.Context, _ string) (ObjectInfo, error) {
	if o.statErr != nil {
		return ObjectInfo{}, o.statErr
	}
	return o.info, nil
}

func (o *fakeObjects) Delete(_ context.Context, key string) error {
	o.deleted = append(o.deleted, key)
	return o.deleteErr
}

// fakeScanner returns a fixed verdict.
type fakeScanner struct {
	res ScanResult
	err error
}

func (s fakeScanner) Scan(context.Context, string) (ScanResult, error) { return s.res, s.err }
