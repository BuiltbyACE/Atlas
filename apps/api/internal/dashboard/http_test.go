package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSummaryReturnsTiles(t *testing.T) {
	store := &fakeStore{
		people:    map[string]int{"active": 2},
		approvals: ApprovalsSummary{Pending: 1, PendingByRole: map[string]int{"ceo_finance": 1}},
	}
	h := NewHandler(NewService(store, &fakeAuditStore{}, nil), nil)

	rec := httptest.NewRecorder()
	h.Summary(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/executive", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body Summary
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if body.People.Total != 2 || body.Approvals.Pending != 1 {
		t.Fatalf("body = %+v", body)
	}
	if body.People.ByStatus == nil || body.RecentAudit == nil {
		t.Fatalf("nil tile in %+v; want empty collections serialised", body)
	}
	// The staff tile must always carry every known status.
	if _, ok := body.People.ByStatus["exited"]; !ok {
		t.Fatalf("by_status missing exited: %+v", body.People.ByStatus)
	}
}

func TestSummaryErrorIsInternal(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{peopleErr: errors.New("boom")}, &fakeAuditStore{}, nil), nil)

	rec := httptest.NewRecorder()
	h.Summary(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/executive", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
