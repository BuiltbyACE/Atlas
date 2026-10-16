package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestS3StoreRoundTrip exercises the real S3 adapter against a running
// S3-compatible endpoint. It is skipped unless ATLAS_TEST_S3_ENDPOINT is set,
// so the default `go test ./...` stays hermetic; point it at SeaweedFS locally
// or R2 in a dedicated job.
//
//	ATLAS_TEST_S3_ENDPOINT   e.g. http://localhost:9000
//	ATLAS_TEST_S3_BUCKET     e.g. atlas-files
//	ATLAS_TEST_S3_REGION     optional
//	ATLAS_TEST_S3_ACCESS_KEY / ATLAS_TEST_S3_SECRET_KEY
func TestS3StoreRoundTrip(t *testing.T) {
	endpoint := os.Getenv("ATLAS_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("ATLAS_TEST_S3_ENDPOINT is not set; skipping S3 round-trip test")
	}
	ctx := context.Background()
	store, err := NewS3Store(ctx, S3Config{
		Endpoint:  endpoint,
		Region:    os.Getenv("ATLAS_TEST_S3_REGION"),
		Bucket:    os.Getenv("ATLAS_TEST_S3_BUCKET"),
		AccessKey: os.Getenv("ATLAS_TEST_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("ATLAS_TEST_S3_SECRET_KEY"),
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}

	key := fmt.Sprintf("smoke/%d.txt", time.Now().UnixNano())
	t.Cleanup(func() { _ = store.Delete(context.Background(), key) })

	const body = "atlas files smoke test"

	putURL, headers, err := store.PresignPut(ctx, key, "text/plain", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new put request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("PUT status = %d, want 2xx", resp.StatusCode)
	}

	info, err := store.Stat(ctx, key)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.SizeBytes != int64(len(body)) {
		t.Fatalf("Stat size = %d, want %d", info.SizeBytes, len(body))
	}

	getURL, err := store.PresignGet(ctx, key, "smoke.txt", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	gresp, err := http.Get(getURL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = gresp.Body.Close() }()
	if gresp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", gresp.StatusCode)
	}
	got, err := io.ReadAll(gresp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != body {
		t.Fatalf("downloaded %q, want %q", got, body)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Stat(ctx, key); !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("Stat after delete: err = %v, want ErrObjectMissing", err)
	}
}
