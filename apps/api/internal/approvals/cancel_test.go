package approvals

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// getOnlyStore answers Get and panics on anything else, so a test proves the
// authorisation decision is made before any write is attempted.
type getOnlyStore struct {
	Store
	approval *Approval
}

func (s getOnlyStore) Get(context.Context, uuid.UUID) (*Approval, error) { return s.approval, nil }

func TestCancelRefusesAnotherPersonWithoutDecidePermission(t *testing.T) {
	requester, other := uuid.New(), uuid.New()
	svc := &Service{store: getOnlyStore{approval: &Approval{ID: uuid.New(), RequestedBy: requester}}}

	err := svc.Cancel(context.Background(), Cancel{ApprovalID: uuid.New(), ActorID: other})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("a bystander cancelling someone else's approval: got %v, want ErrForbidden", err)
	}
}

func TestCancelUnknownApprovalIsNotFound(t *testing.T) {
	svc := &Service{store: getOnlyStore{}}
	err := svc.Cancel(context.Background(), Cancel{ApprovalID: uuid.New(), ActorID: uuid.New(), Privileged: true})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
