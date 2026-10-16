package files

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUploadRequestNormalized(t *testing.T) {
	entity := uuid.New()
	tests := []struct {
		name    string
		in      UploadRequest
		wantErr bool
		check   func(t *testing.T, got UploadRequest)
	}{
		{
			name: "valid with defaults",
			in:   UploadRequest{Filename: "report.pdf", ContentType: "application/pdf", SizeBytes: 1024},
			check: func(t *testing.T, got UploadRequest) {
				if got.Classification != Internal {
					t.Errorf("classification = %q, want internal", got.Classification)
				}
			},
		},
		{
			name: "path segments are stripped",
			in:   UploadRequest{Filename: `..\..\etc\passwd`, ContentType: "text/plain", SizeBytes: 1},
			check: func(t *testing.T, got UploadRequest) {
				if got.Filename != "passwd" {
					t.Errorf("filename = %q, want passwd", got.Filename)
				}
			},
		},
		{
			name: "content type defaults",
			in:   UploadRequest{Filename: "blob", SizeBytes: 10},
			check: func(t *testing.T, got UploadRequest) {
				if got.ContentType != "application/octet-stream" {
					t.Errorf("content_type = %q", got.ContentType)
				}
			},
		},
		{
			name: "entity pair is accepted",
			in:   UploadRequest{Filename: "a.txt", SizeBytes: 1, EntityType: "project", EntityID: &entity},
		},
		{name: "missing filename", in: UploadRequest{SizeBytes: 1}, wantErr: true},
		{name: "dot filename", in: UploadRequest{Filename: "..", SizeBytes: 1}, wantErr: true},
		{name: "zero size", in: UploadRequest{Filename: "a.txt"}, wantErr: true},
		{name: "too large", in: UploadRequest{Filename: "a.txt", SizeBytes: MaxUploadBytes + 1}, wantErr: true},
		{name: "bad classification", in: UploadRequest{Filename: "a.txt", SizeBytes: 1, Classification: "secret"}, wantErr: true},
		{name: "entity type without id", in: UploadRequest{Filename: "a.txt", SizeBytes: 1, EntityType: "project"}, wantErr: true},
		{name: "entity id without type", in: UploadRequest{Filename: "a.txt", SizeBytes: 1, EntityID: &entity}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Normalized()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

func TestObjectKeyIsDateIDAndSafeName(t *testing.T) {
	id := uuid.New()
	at := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

	key := ObjectKey(id, "Q3 Report (final).pdf", at)

	if want := "2026/10/16/" + id.String() + "/Q3_Report__final_.pdf"; key != want {
		t.Fatalf("ObjectKey = %q, want %q", key, want)
	}
}

func TestSafeNameFallsBack(t *testing.T) {
	for _, in := range []string{"", "   ", "///", "..."} {
		if got := safeName(in); got == "" {
			t.Fatalf("safeName(%q) returned empty", in)
		}
	}
}
