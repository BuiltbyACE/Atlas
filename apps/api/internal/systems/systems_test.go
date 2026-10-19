package systems

import (
	"errors"
	"testing"
	"time"
)

// These tests cover the rules that need no database: the warning bands, the
// lifecycle predicates, the derived-phase arithmetic and the stable problem
// codes. Everything that needs real rows lives in the integration test.

func TestWarningBand(t *testing.T) {
	cases := []struct {
		daysOut int
		want    int
	}{
		{-3, 7}, // already lapsed is always the tightest band
		{0, 7},  // expires today
		{1, 7},
		{7, 7},
		{8, 14},
		{14, 14},
		{15, 30},
		{30, 30},
		{31, 0}, // beyond every band
		{400, 0},
	}
	for _, tc := range cases {
		if got := warningBand(tc.daysOut); got != tc.want {
			t.Errorf("warningBand(%d) = %d, want %d", tc.daysOut, got, tc.want)
		}
	}
}

func TestExpiryWarningDaysAreTightestFirst(t *testing.T) {
	// The bands are checked in order, so they must be sorted tightest first or
	// warningBand silently returns the wrong label.
	for i := 1; i < len(ExpiryWarningDays); i++ {
		if ExpiryWarningDays[i] >= ExpiryWarningDays[i-1] {
			t.Fatalf("ExpiryWarningDays must be descending, got %v", ExpiryWarningDays)
		}
	}
}

func TestDaysBetween(t *testing.T) {
	asOf := time.Date(2026, 10, 19, 23, 45, 0, 0, time.UTC)

	// The time of day is discarded: a warning band cannot shift because a read
	// happened after lunch.
	if got := daysBetween(asOf, time.Date(2026, 10, 26, 0, 5, 0, 0, time.UTC)); got != 7 {
		t.Errorf("daysBetween = %d, want 7", got)
	}
	// A local offset is normalised to UTC before the day difference.
	loc := time.FixedZone("EAT", 3*60*60)
	if got := daysBetween(asOf, time.Date(2026, 10, 26, 1, 0, 0, 0, loc)); got != 6 {
		t.Errorf("daysBetween with offset = %d, want 6", got)
	}
	// The past is negative, which is what makes a lapsed item read as overdue.
	if got := daysBetween(asOf, time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)); got != -7 {
		t.Errorf("daysBetween past = %d, want -7", got)
	}
}

func TestEnumValidation(t *testing.T) {
	if !validSystemType(TypeIntegration) || validSystemType("mainframe") {
		t.Error("system type validation is wrong")
	}
	if !validEnvironment(EnvProd) || validEnvironment("dev") {
		t.Error("environment validation is wrong")
	}
	if !validCriticality(Tier1) || validCriticality("tier0") {
		t.Error("criticality validation is wrong")
	}
	if !validSystemStatus(SystemActive) || validSystemStatus("archived") {
		t.Error("system status validation is wrong")
	}
	if !validCheckType(CheckHTTPContent) || validCheckType("ping") {
		t.Error("check type validation is wrong")
	}
	if !validCheckStatus(CheckWarn) || validCheckStatus("flapping") {
		t.Error("check status validation is wrong")
	}
	if !validCheckSource(SourceHeartbeat) || validCheckSource("rumour") {
		t.Error("check source validation is wrong")
	}
	if !validIncidentSeverity(Sev2) || validIncidentSeverity("sev5") {
		t.Error("severity validation is wrong")
	}
	if !validIncidentStatus(IncidentMitigated) || validIncidentStatus("paused") {
		t.Error("incident status validation is wrong")
	}
	if !validIncidentSource(DetectedByClient) || validIncidentSource("twitter") {
		t.Error("incident source validation is wrong")
	}
	if !validTimelineEntry(EntryAction) || validTimelineEntry("emoji") {
		t.Error("timeline entry validation is wrong")
	}
	if !validActionStatus(ActionDone) || validActionStatus("someday") {
		t.Error("action status validation is wrong")
	}
}

func TestSeverityRequiresReview(t *testing.T) {
	for _, s := range []IncidentSeverity{Sev1, Sev2} {
		if !s.RequiresReview() {
			t.Errorf("%s should require a post-incident review", s)
		}
	}
	for _, s := range []IncidentSeverity{Sev3, Sev4} {
		if s.RequiresReview() {
			t.Errorf("%s should not require a post-incident review", s)
		}
	}
}

func TestIncidentStatusIsOpenUntilResolved(t *testing.T) {
	open := []IncidentStatus{IncidentOpen, IncidentAcknowledged, IncidentMitigated}
	for _, s := range open {
		if !s.Open() {
			t.Errorf("%s should count as open", s)
		}
	}
	for _, s := range []IncidentStatus{IncidentResolved, IncidentClosed} {
		if s.Open() {
			t.Errorf("%s should not count as open", s)
		}
	}
}

func TestIncidentLifecyclePredicates(t *testing.T) {
	// The predicates are the single rule for each transition: a refusal names
	// the transition the caller attempted.
	if !IncidentAcknowledgeable(IncidentOpen) || IncidentAcknowledgeable(IncidentAcknowledged) {
		t.Error("only an open incident can be acknowledged")
	}
	if !IncidentMitigable(IncidentOpen) || !IncidentMitigable(IncidentAcknowledged) ||
		IncidentMitigable(IncidentResolved) {
		t.Error("an incident can be mitigated until it is resolved")
	}
	if !IncidentResolvable(IncidentOpen) || !IncidentResolvable(IncidentMitigated) ||
		IncidentResolvable(IncidentClosed) {
		t.Error("a running incident can be resolved; a closed one cannot")
	}
	// Closing straight from open would skip the review, so it is not allowed.
	if !IncidentClosable(IncidentResolved) || IncidentClosable(IncidentOpen) {
		t.Error("only a resolved incident can be closed")
	}
	if !CheckEditable(SystemActive) || CheckEditable(SystemRetired) {
		t.Error("a retired system's checks are frozen with it")
	}
}

func TestCheckUnhealthy(t *testing.T) {
	if (Check{}).Unhealthy() {
		t.Error("a check that has never run is not unhealthy, it is unprobed")
	}
	pass := CheckPass
	if (Check{LastStatus: &pass}).Unhealthy() {
		t.Error("a passing check is healthy")
	}
	for _, s := range []CheckStatus{CheckWarn, CheckFail} {
		status := s
		if !(Check{LastStatus: &status}).Unhealthy() {
			t.Errorf("a %s check is unhealthy", s)
		}
	}
}

func TestElapsedSeconds(t *testing.T) {
	from := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	if elapsedSeconds(from, nil) != nil {
		t.Error("an unfinished phase has no duration")
	}
	to := from.Add(90 * time.Second)
	got := elapsedSeconds(from, &to)
	if got == nil || *got != 90 {
		t.Errorf("elapsedSeconds = %v, want 90", got)
	}
}

func TestProblemCode(t *testing.T) {
	if problemCode(errors.New("plain")) != "" {
		t.Error("an uncoded error has no problem code")
	}
	err := withCode(ErrInvalid, CodeRootCauseRequired)
	if problemCode(err) != CodeRootCauseRequired {
		t.Errorf("problemCode = %q, want %q", problemCode(err), CodeRootCauseRequired)
	}
	// The code wraps the sentinel, so errors.Is still finds the category.
	if !errors.Is(err, ErrInvalid) {
		t.Error("a coded error must still be an ErrInvalid")
	}
	if withCode(nil, CodeReasonRequired) != nil {
		t.Error("withCode(nil, ...) must stay nil")
	}
}

func TestInvalidFieldIsAnErrInvalid(t *testing.T) {
	err := errInvalidField("within_days")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("errInvalidField must be an ErrInvalid, got %v", err)
	}
}

func TestValidAvailabilityBP(t *testing.T) {
	if !validAvailabilityBP(0) || !validAvailabilityBP(9999) || !validAvailabilityBP(10000) {
		t.Error("0..10000 basis points are valid")
	}
	if validAvailabilityBP(-1) || validAvailabilityBP(10001) {
		t.Error("out-of-range availability is not valid")
	}
}

func TestSeverityLabel(t *testing.T) {
	for sev, want := range map[IncidentSeverity]string{
		Sev1: "SEV1", Sev2: "SEV2", Sev3: "SEV3", Sev4: "SEV4",
	} {
		if got := severityLabel(sev); got != want {
			t.Errorf("severityLabel(%s) = %q, want %q", sev, got, want)
		}
	}
}
