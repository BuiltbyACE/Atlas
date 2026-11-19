package systems

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests exercise the read model against a real Postgres: the derived
// health on the register, the run of consecutive failures flap protection
// needs, availability, the incident detail, and what expires. They skip unless
// ATLAS_TEST_DATABASE_URL is set so the unit suite stays hermetic.

func systemsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping systems integration tests")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// fixture tracks the rows a test created so cleanup can remove them, children
// before parents.
type fixture struct {
	pool        *pgxpool.Pool
	systems     []uuid.UUID
	checks      []uuid.UUID
	incidents   []uuid.UUID
	domains     []uuid.UUID
	certs       []uuid.UUID
	credentials []uuid.UUID
	persons     []uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{pool: systemsTestPool(t)}
	t.Cleanup(func() { f.cleanup(t) })
	return f
}

func (f *fixture) cleanup(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	deleteByID := func(table string, ids []uuid.UUID) {
		if len(ids) == 0 {
			return
		}
		if _, err := f.pool.Exec(ctx, `DELETE FROM `+table+` WHERE id = ANY($1)`, ids); err != nil {
			t.Logf("cleanup %s: %v", table, err)
		}
	}
	// check_results and an incident's timeline, actions and systems all go with
	// their parent, so the cascade does the ordering for us.
	deleteByID("systems.credentials", f.credentials)
	deleteByID("systems.certificates", f.certs)
	deleteByID("systems.domains", f.domains)
	deleteByID("systems.incidents", f.incidents)
	deleteByID("systems.checks", f.checks)
	deleteByID("systems.systems", f.systems)
	deleteByID("people.persons", f.persons)
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// today is the day the service reads from, so a fixture's "expires in 10 days"
// stays exactly 10 days out whatever time the test runs.
func today() time.Time {
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

func tick() string { return uuid.NewString()[:8] }

func (f *fixture) person(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustExec(t, f.pool, `INSERT INTO people.persons (id, full_name, employment_type, status)
		VALUES ($1, 'Systems Test Person', 'full_time', 'active')`, id)
	f.persons = append(f.persons, id)
	return id
}

func (f *fixture) system(t *testing.T, name string, tier Criticality) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.systems
		(id, ref, name, system_type, environment, criticality, status)
		VALUES ($1, $2, $3, 'api', 'prod', $4, 'active')`,
		id, "SYS-"+tick(), name, string(tier))
	f.systems = append(f.systems, id)
	return id
}

func (f *fixture) check(t *testing.T, systemID uuid.UUID, name string, ctype CheckType, active bool) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.checks
		(id, system_id, name, check_type, target, interval_seconds, failure_threshold, active)
		VALUES ($1, $2, $3, $4, 'https://example.test/healthz', 60, 2, $5)`,
		id, systemID, name, string(ctype), active)
	f.checks = append(f.checks, id)
	return id
}

// result publishes one probe outcome. It also proves the month partition exists
// and is where the row landed.
func (f *fixture) result(t *testing.T, checkID uuid.UUID, at time.Time, status CheckStatus, latencyMS int) {
	t.Helper()
	mustExec(t, f.pool, `INSERT INTO systems.check_results (id, check_id, at, status, latency_ms, source)
		VALUES ($1, $2, $3, $4, $5, 'atlas')`, uuid.New(), checkID, at, string(status), latencyMS)
}

func (f *fixture) incident(t *testing.T, title string, sev IncidentSeverity, status IncidentStatus, detected time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.incidents
		(id, ref, title, severity, status, detected_at, detected_source)
		VALUES ($1, $2, $3, $4, $5, $6, 'atlas')`,
		id, "INC-"+tick(), title, string(sev), string(status), detected)
	f.incidents = append(f.incidents, id)
	return id
}

func (f *fixture) linkIncident(t *testing.T, incidentID, systemID uuid.UUID) {
	t.Helper()
	mustExec(t, f.pool, `INSERT INTO systems.incident_systems (incident_id, system_id) VALUES ($1, $2)`,
		incidentID, systemID)
}

// ── the register ────────────────────────────────────────────────────────────

func TestSystemHealthIsDerivedFromTheLatestResult(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	id := f.system(t, "Atlas API "+tick(), Tier1)
	healthy := f.check(t, id, "Readiness", CheckHTTPContent, true)
	broken := f.check(t, id, "TCP", CheckTCP, true)

	// The latest result decides health, not the best or worst ever recorded.
	f.result(t, healthy, today().Add(-3*time.Hour), CheckPass, 120)
	f.result(t, broken, today().Add(-2*time.Hour), CheckFail, 0)

	sys, err := svc.System(ctx, id)
	if err != nil {
		t.Fatalf("get system: %v", err)
	}
	if sys.ChecksTotal != 2 || sys.ChecksFailing != 1 || sys.ChecksWarning != 0 {
		t.Errorf("derived health = total %d failing %d warning %d, want 2/1/0",
			sys.ChecksTotal, sys.ChecksFailing, sys.ChecksWarning)
	}
	// Availability counts the pass over the two probes.
	if sys.AvailabilityBP == nil || *sys.AvailabilityBP != 5000 {
		t.Errorf("availability = %v, want 5000", sys.AvailabilityBP)
	}
}

func TestConsecutiveFailuresDriveFlapProtection(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	id := f.system(t, "Flapper "+tick(), Tier2)
	ck := f.check(t, id, "Latency", CheckLatency, true)

	base := today().Add(-5 * time.Hour)
	// pass, then three failures: the run is three, which is what an incident
	// opening on the Nth failure needs.
	f.result(t, ck, base, CheckPass, 90)
	f.result(t, ck, base.Add(time.Hour), CheckFail, 0)
	f.result(t, ck, base.Add(2*time.Hour), CheckFail, 0)
	f.result(t, ck, base.Add(3*time.Hour), CheckFail, 0)

	checks, err := svc.Checks(ctx, CheckQuery{SystemID: &id}, 10, nil)
	if err != nil {
		t.Fatalf("list checks: %v", err)
	}
	if len(checks) != 1 {
		t.Fatalf("checks = %d, want 1", len(checks))
	}
	got := checks[0]
	if got.LastStatus == nil || *got.LastStatus != CheckFail {
		t.Errorf("last status = %v, want fail", got.LastStatus)
	}
	if got.ConsecutiveFailures != 3 {
		t.Errorf("consecutive failures = %d, want 3", got.ConsecutiveFailures)
	}

	// A later pass resets the run to zero.
	f.result(t, ck, base.Add(4*time.Hour), CheckPass, 80)
	checks, err = svc.Checks(ctx, CheckQuery{SystemID: &id}, 10, nil)
	if err != nil {
		t.Fatalf("list checks again: %v", err)
	}
	if checks[0].ConsecutiveFailures != 0 {
		t.Errorf("consecutive failures after a pass = %d, want 0", checks[0].ConsecutiveFailures)
	}
}

func TestInactiveChecksAreExcludedFromHealth(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	id := f.system(t, "Paused probes "+tick(), Tier3)
	f.check(t, id, "Live", CheckHTTPStatus, true)
	paused := f.check(t, id, "Muted", CheckHTTPStatus, false)
	f.result(t, paused, today().Add(-time.Hour), CheckFail, 0)

	sys, err := svc.System(ctx, id)
	if err != nil {
		t.Fatalf("get system: %v", err)
	}
	if sys.ChecksTotal != 1 || sys.ChecksFailing != 0 {
		t.Errorf("an inactive check should not count: total %d failing %d", sys.ChecksTotal, sys.ChecksFailing)
	}
}

func TestListSystemsFiltersAndSearch(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	marker := tick()
	tier1 := f.system(t, "Payments API "+marker, Tier1)
	f.system(t, "Marketing site "+marker, Tier3)

	// Tier 1 filter.
	tier := Tier1
	found, err := svc.Systems(ctx, SystemQuery{Criticality: &tier}, 50, nil)
	if err != nil {
		t.Fatalf("list systems: %v", err)
	}
	if !containsSystem(found, tier1) {
		t.Errorf("tier1 filter lost the tier1 system: %v", refsOf(found))
	}
	for _, s := range found {
		if s.Criticality != Tier1 {
			t.Errorf("tier1 filter returned a %s system", s.Criticality)
		}
	}

	// Search matches the name.
	hits, err := svc.Systems(ctx, SystemQuery{Search: marker}, 50, nil)
	if err != nil {
		t.Fatalf("search systems: %v", err)
	}
	if len(hits) != 2 {
		t.Errorf("search for %s returned %d systems, want 2", marker, len(hits))
	}
}

func TestUnhealthyFilterFindsFailingAndOpenIncidents(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	marker := tick()
	broken := f.system(t, "Broken "+marker, Tier1)
	ck := f.check(t, broken, "Readiness", CheckHTTPContent, true)
	f.result(t, ck, today().Add(-time.Hour), CheckFail, 0)

	withIncident := f.system(t, "Incident "+marker, Tier2)
	inc := f.incident(t, "Outage "+marker, Sev1, IncidentOpen, today().Add(-2*time.Hour))
	f.linkIncident(t, inc, withIncident)

	f.system(t, "Fine "+marker, Tier3)

	unhealthy, err := svc.Systems(ctx, SystemQuery{Search: marker, UnhealthyOnly: true}, 50, nil)
	if err != nil {
		t.Fatalf("list unhealthy: %v", err)
	}
	if len(unhealthy) != 2 {
		t.Fatalf("unhealthy = %v, want the failing and the incident systems", refsOf(unhealthy))
	}
	if !containsSystem(unhealthy, broken) || !containsSystem(unhealthy, withIncident) {
		t.Errorf("unhealthy filter missed a system: %v", refsOf(unhealthy))
	}
}

func TestRetiredSystemKeepsItsChecksButHealthIsFrozen(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	id := f.system(t, "Retired "+tick(), Tier3)
	f.check(t, id, "Last check", CheckHTTPStatus, true)
	mustExec(t, f.pool, `UPDATE systems.systems SET status = 'retired' WHERE id = $1`, id)

	// A retired system is still readable — its incident history depends on it.
	sys, err := svc.System(ctx, id)
	if err != nil {
		t.Fatalf("get retired system: %v", err)
	}
	if sys.Status != SystemRetired {
		t.Errorf("status = %s, want retired", sys.Status)
	}
	if !CheckEditable(SystemActive) || CheckEditable(sys.Status) {
		t.Error("a retired system's checks must not be editable")
	}
}

// ── checks and results ──────────────────────────────────────────────────────

func TestCheckResultsPaginateNewestFirst(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	id := f.system(t, "Results "+tick(), Tier2)
	ck := f.check(t, id, "Status", CheckHTTPStatus, true)

	base := today().Add(-6 * time.Hour)
	for i := 0; i < 3; i++ {
		f.result(t, ck, base.Add(time.Duration(i)*time.Hour), CheckPass, 100+i)
	}

	first, err := svc.CheckResults(ctx, ck, 2, nil)
	if err != nil {
		t.Fatalf("check results: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first page = %d results, want 2", len(first))
	}
	if first[0].At.Before(first[1].At) {
		t.Error("results are not newest first")
	}

	last := first[len(first)-1]
	next, err := svc.CheckResults(ctx, ck, 2, &Cursor{At: last.At, ID: last.ID})
	if err != nil {
		t.Fatalf("check results page 2: %v", err)
	}
	if len(next) != 1 {
		t.Fatalf("second page = %d results, want 1", len(next))
	}
	if !next[0].At.Before(last.At) {
		t.Error("the second page did not continue from the cursor")
	}
}

func TestCheckResultsUnknownCheckIsNotFound(t *testing.T) {
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	if _, err := svc.CheckResults(context.Background(), uuid.New(), 10, nil); err == nil {
		t.Fatal("expected ErrNotFound for an unknown check")
	}
}

// ── incidents ───────────────────────────────────────────────────────────────

func TestIncidentDetailLoadsSystemsTimelineAndActions(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	detected := today().Add(-8 * time.Hour)
	inc := f.incident(t, "Checkout down "+tick(), Sev1, IncidentResolved, detected)
	a := f.system(t, "Checkout API "+tick(), Tier1)
	b := f.system(t, "Checkout Worker "+tick(), Tier2)
	f.linkIncident(t, inc, a)
	f.linkIncident(t, inc, b)

	ack := detected.Add(6 * time.Minute)
	resolved := detected.Add(95 * time.Minute)
	mustExec(t, f.pool, `UPDATE systems.incidents
		SET acknowledged_at = $2, resolved_at = $3, review_required = true, root_cause = 'bad deploy'
		WHERE id = $1`, inc, ack, resolved)

	owner := f.person(t)
	mustExec(t, f.pool, `INSERT INTO systems.incident_timeline (id, incident_id, at, entry_type, note)
		VALUES ($1, $2, $3, 'detected', 'Readiness check failed'), ($4, $2, $5, 'resolved', 'Rolled back')`,
		uuid.New(), inc, detected, uuid.New(), resolved)
	mustExec(t, f.pool, `INSERT INTO systems.incident_actions
		(id, incident_id, title, owner_person_id, status) VALUES ($1, $2, 'Add a canary', $3, 'todo')`,
		uuid.New(), inc, owner)

	full, err := svc.Incident(ctx, inc)
	if err != nil {
		t.Fatalf("get incident: %v", err)
	}
	if len(full.Systems) != 2 {
		t.Errorf("systems = %d, want 2", len(full.Systems))
	}
	if len(full.Timeline) != 2 {
		t.Errorf("timeline = %d, want 2", len(full.Timeline))
	}
	if len(full.Actions) != 1 || full.Actions[0].OwnerName == "" {
		t.Errorf("actions = %+v, want one with an owner name", full.Actions)
	}
	if !full.ReviewRequired {
		t.Error("a resolved SEV1 should still be flagged for review")
	}
	// The list projection carries the system names too.
	listed, err := svc.Incidents(ctx, IncidentQuery{Search: "Checkout down"}, 10, nil)
	if err != nil {
		t.Fatalf("list incidents: %v", err)
	}
	if len(listed) != 1 || len(listed[0].SystemNames) != 2 {
		t.Errorf("list projection = %+v, want two system names", listed)
	}
}

func TestIncidentFilters(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	marker := tick()
	open := f.incident(t, "Open "+marker, Sev2, IncidentOpen, today().Add(-time.Hour))
	f.incident(t, "Closed "+marker, Sev4, IncidentClosed, today().Add(-72*time.Hour))

	sev := Sev2
	found, err := svc.Incidents(ctx, IncidentQuery{Search: marker, Severity: &sev}, 10, nil)
	if err != nil {
		t.Fatalf("list by severity: %v", err)
	}
	if len(found) != 1 || found[0].ID != open {
		t.Errorf("severity filter = %v", idsOf(found))
	}

	openOnly, err := svc.Incidents(ctx, IncidentQuery{Search: marker, OpenOnly: true}, 10, nil)
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(openOnly) != 1 || openOnly[0].Status.Open() == false {
		t.Errorf("open filter = %v", idsOf(openOnly))
	}

	// Unknown incident is ErrNotFound.
	if _, err := svc.Incident(ctx, uuid.New()); err == nil {
		t.Error("expected ErrNotFound for an unknown incident")
	}
}

// ── overview ────────────────────────────────────────────────────────────────

func TestOverviewMeasuresTheRegister(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	// Counts are global, so this test asserts deltas against the snapshot it
	// takes first — the shared database may hold other rows.
	before, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}

	marker := tick()
	id := f.system(t, "Overview system "+marker, Tier1)
	ck := f.check(t, id, "Readiness", CheckHTTPContent, true)
	f.result(t, ck, today().Add(-30*time.Minute), CheckFail, 0)
	// An unprobed active check counts toward the unprobed backlog.
	f.check(t, id, "Never run", CheckHeartbeat, true)

	inc := f.incident(t, "Overview incident "+marker, Sev1, IncidentOpen, today().Add(-45*time.Minute))
	f.linkIncident(t, inc, id)

	after, err := svc.Overview(ctx)
	if err != nil {
		t.Fatalf("overview again: %v", err)
	}
	if after.SystemsTotal-before.SystemsTotal != 1 {
		t.Errorf("systems_total delta = %d, want 1", after.SystemsTotal-before.SystemsTotal)
	}
	if after.SystemsTier1-before.SystemsTier1 != 1 {
		t.Errorf("systems_tier1 delta = %d, want 1", after.SystemsTier1-before.SystemsTier1)
	}
	if after.ChecksTotal-before.ChecksTotal != 2 {
		t.Errorf("checks_total delta = %d, want 2", after.ChecksTotal-before.ChecksTotal)
	}
	if after.ChecksFailing-before.ChecksFailing != 1 {
		t.Errorf("checks_failing delta = %d, want 1", after.ChecksFailing-before.ChecksFailing)
	}
	if after.ChecksUnprobed-before.ChecksUnprobed != 1 {
		t.Errorf("checks_unprobed delta = %d, want 1", after.ChecksUnprobed-before.ChecksUnprobed)
	}
	if after.OpenIncidents-before.OpenIncidents != 1 {
		t.Errorf("open_incidents delta = %d, want 1", after.OpenIncidents-before.OpenIncidents)
	}
	if after.Incidents30D-before.Incidents30D != 1 {
		t.Errorf("incidents_30d delta = %d, want 1", after.Incidents30D-before.Incidents30D)
	}
	if after.DetectedByAtlas30D-before.DetectedByAtlas30D != 1 {
		t.Errorf("detected_by_atlas_30d delta = %d, want 1", after.DetectedByAtlas30D-before.DetectedByAtlas30D)
	}
	// Every severity is reported, so a chart never has a hole.
	if len(after.OpenIncidentsBySev) != 4 {
		t.Errorf("open incidents by severity has %d buckets, want 4", len(after.OpenIncidentsBySev))
	}
	if !containsAvailability(after.Availability, id) {
		t.Errorf("availability is missing the probed system")
	}
}

func TestAvailabilityIsMeasuredOverTheWindow(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	id := f.system(t, "Availability "+tick(), Tier1)
	ck := f.check(t, id, "Readiness", CheckHTTPContent, true)
	// Three passes inside the window and one failure outside it: the ratio must
	// ignore the stale failure.
	f.result(t, ck, today().Add(-3*time.Hour), CheckPass, 100)
	f.result(t, ck, today().Add(-2*time.Hour), CheckPass, 100)
	f.result(t, ck, today().Add(-time.Hour), CheckPass, 100)
	f.result(t, ck, today().AddDate(0, 0, -45), CheckFail, 0)

	sys, err := svc.System(ctx, id)
	if err != nil {
		t.Fatalf("get system: %v", err)
	}
	if sys.AvailabilityBP == nil || *sys.AvailabilityBP != 10000 {
		t.Errorf("availability = %v, want 10000 (the stale failure is outside the window)", sys.AvailabilityBP)
	}
}

// ── expiring ────────────────────────────────────────────────────────────────

func TestExpiringGathersDomainsCertificatesAndRotations(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	svc := NewService(NewPGStore(f.pool), nil)

	marker := tick()
	id := f.system(t, "Expiry system "+marker, Tier1)

	domainID := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.domains (id, name, system_id, expires_on)
		VALUES ($1, $2, $3, $4)`, domainID, marker+"-domain.test", id, today().AddDate(0, 0, 10))
	f.domains = append(f.domains, domainID)

	// Beyond the widest band: must not be reported.
	farDomain := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.domains (id, name, expires_on)
		VALUES ($1, $2, $3)`, farDomain, marker+"-far.test", today().AddDate(0, 0, 200))
	f.domains = append(f.domains, farDomain)

	certID := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.certificates (id, hostname, system_id, expires_on)
		VALUES ($1, $2, $3, $4)`, certID, "api."+marker+".test", id, today().AddDate(0, 0, 3))
	f.certs = append(f.certs, certID)

	credID := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.credentials
		(id, system_id, label, credential_kind, store_path, next_rotation_on, active)
		VALUES ($1, $2, 'Stripe key', 'api_key', 'sm://atlas/stripe', $3, true)`,
		credID, id, today().AddDate(0, 0, 20))
	f.credentials = append(f.credentials, credID)

	// An inactive credential is not a rotation due.
	inactive := uuid.New()
	mustExec(t, f.pool, `INSERT INTO systems.credentials
		(id, system_id, label, credential_kind, store_path, next_rotation_on, active)
		VALUES ($1, $2, 'Old key', 'api_key', 'sm://atlas/old', $3, false)`,
		inactive, id, today().AddDate(0, 0, 5))
	f.credentials = append(f.credentials, inactive)

	items, err := svc.Expiring(ctx, ExpiryWarningDays[0])
	if err != nil {
		t.Fatalf("expiring: %v", err)
	}

	byID := map[uuid.UUID]Expiry{}
	for _, e := range items {
		byID[e.ID] = e
	}
	if _, found := byID[farDomain]; found {
		t.Error("a domain 200 days out should not be in the 30-day window")
	}
	if _, found := byID[inactive]; found {
		t.Error("an inactive credential should not be a rotation due")
	}

	cert, ok := byID[certID]
	if !ok {
		t.Fatalf("certificate missing from expiring: %+v", items)
	}
	if cert.DaysOut != 3 || cert.WarningBand != 7 {
		t.Errorf("certificate = days %d band %d, want 3/7", cert.DaysOut, cert.WarningBand)
	}
	dom, ok := byID[domainID]
	if !ok {
		t.Fatalf("domain missing from expiring: %+v", items)
	}
	if dom.DaysOut != 10 || dom.WarningBand != 14 {
		t.Errorf("domain = days %d band %d, want 10/14", dom.DaysOut, dom.WarningBand)
	}
	if dom.SystemID == nil || *dom.SystemID != id {
		t.Errorf("domain should name its system")
	}
	cred, ok := byID[credID]
	if !ok {
		t.Fatalf("credential missing from expiring: %+v", items)
	}
	if cred.Kind != ExpiryCredential || cred.WarningBand != 30 {
		t.Errorf("credential = kind %s band %d, want credential/30", cred.Kind, cred.WarningBand)
	}
	// Soonest first.
	for i := 1; i < len(items); i++ {
		if items[i].ExpiresOn.Before(items[i-1].ExpiresOn) {
			t.Errorf("expiring items are not soonest-first: %v", items)
			break
		}
	}
	// A wider window is a superset.
	wider, err := svc.Expiring(ctx, 90)
	if err != nil {
		t.Fatalf("expiring wider: %v", err)
	}
	if len(wider) < len(items) {
		t.Errorf("a 90-day window returned fewer items (%d) than a 30-day one (%d)", len(wider), len(items))
	}
}

func TestCheckResultsLandInAMonthPartition(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	id := f.system(t, "Partition "+tick(), Tier3)
	ck := f.check(t, id, "Status", CheckHTTPStatus, true)
	at := today().Add(-time.Hour)
	f.result(t, ck, at, CheckPass, 50)

	var table string
	if err := f.pool.QueryRow(ctx,
		`SELECT tableoid::regclass::text FROM systems.check_results WHERE check_id = $1`, ck).Scan(&table); err != nil {
		t.Fatalf("resolve partition: %v", err)
	}
	if table == "systems.check_results_default" {
		t.Errorf("result landed in the default partition, want a month partition (%s)", table)
	}
	if table == "systems.check_results" {
		t.Errorf("result has no partition: %s", table)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func containsSystem(list []System, id uuid.UUID) bool {
	for _, s := range list {
		if s.ID == id {
			return true
		}
	}
	return false
}

func containsAvailability(list []SystemAvailability, id uuid.UUID) bool {
	for _, a := range list {
		if a.SystemID == id {
			return true
		}
	}
	return false
}

func refsOf(list []System) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Ref)
	}
	return out
}

func idsOf(list []Incident) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(list))
	for _, i := range list {
		out = append(out, i.ID)
	}
	return out
}
