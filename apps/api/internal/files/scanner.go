package files

import "context"

// ScanResult is a scanner's verdict on an uploaded object.
type ScanResult struct {
	// Clean is true when the object may be served to users.
	Clean bool
	// Detail explains a non-clean verdict; shown to administrators only.
	Detail string
}

// Scanner inspects an object before it becomes available. §6.5 leaves the
// concrete engine to a later decision [DECIDE]; the port is here so the Phase 1
// no-op can be replaced without touching the service.
type Scanner interface {
	Scan(ctx context.Context, key string) (ScanResult, error)
}

// NoopScanner trusts every upload. It is the Phase 1 implementation: the bucket
// is private and files are reachable only through a signed, permission-checked
// URL. A real engine (ClamAV, or a provider scan) implements Scanner and is
// wired in its place.
type NoopScanner struct{}

// Scan always reports a clean object.
func (NoopScanner) Scan(context.Context, string) (ScanResult, error) {
	return ScanResult{Clean: true}, nil
}
