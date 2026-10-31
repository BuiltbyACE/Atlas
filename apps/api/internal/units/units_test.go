package units

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidUnitKind(t *testing.T) {
	if !validUnitKind(UnitSchool) || !validUnitKind(UnitDepartment) ||
		!validUnitKind(UnitProject) || !validUnitKind(UnitRegion) {
		t.Fatal("validUnitKind rejected a valid kind")
	}
	if validUnitKind("bogus") {
		t.Fatal("validUnitKind accepted an invalid kind")
	}
}

func TestValidIntegrationKind(t *testing.T) {
	if !validIntegrationKind(IntegrationAPI) || !validIntegrationKind(IntegrationWebhook) ||
		!validIntegrationKind(IntegrationFile) || !validIntegrationKind(IntegrationSFTP) {
		t.Fatal("validIntegrationKind rejected a valid kind")
	}
}

func TestValidIndicatorKind(t *testing.T) {
	if !validIndicatorKind(IndicatorCount) || !validIndicatorKind(IndicatorRatio) ||
		!validIndicatorKind(IndicatorAmount) || !validIndicatorKind(IndicatorBoolean) {
		t.Fatal("validIndicatorKind rejected a valid kind")
	}
}

func TestUnitEditable(t *testing.T) {
	if !UnitEditable(UnitActive) {
		t.Fatal("UnitEditable(active) should be true")
	}
	if UnitEditable(UnitRetired) {
		t.Fatal("UnitEditable(retired) should be false")
	}
}

func TestIntegrationEditable(t *testing.T) {
	if !IntegrationEditable(IntegrationOK) {
		t.Fatal("IntegrationEditable(ok) should be true")
	}
	if IntegrationEditable(IntegrationFailed) {
		t.Fatal("IntegrationEditable(failed) should be false")
	}
}

func TestValidUnitCode(t *testing.T) {
	if !validUnitCode("school") || !validUnitCode("dept-1") || !validUnitCode("a1") {
		t.Fatal("validUnitCode rejected a valid code")
	}
	if validUnitCode("") || validUnitCode("bad code") || validUnitCode("bad/code") {
		t.Fatal("validUnitCode accepted an invalid code")
	}
}

func TestSortSnapshots(t *testing.T) {
	u1 := uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001"))
	u2 := uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000002"))
	i1 := uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000010"))
	i2 := uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000020"))

	snaps := []UnitSnapshot{
		{UnitID: u2, IndicatorID: i2, OnDate: parseDate(t, "2026-01-02")},
		{UnitID: u1, IndicatorID: i2, OnDate: parseDate(t, "2026-01-01")},
		{UnitID: u1, IndicatorID: i1, OnDate: parseDate(t, "2026-01-01")},
		{UnitID: u1, IndicatorID: i1, OnDate: parseDate(t, "2026-01-02")},
	}
	sortSnapshots(snaps)

	if snaps[0].UnitID != u1 || snaps[0].IndicatorID != i1 {
		t.Fatal("sortSnapshots ordering wrong")
	}
	if snaps[1].IndicatorID != i1 || !snaps[1].OnDate.Equal(parseDate(t, "2026-01-02")) {
		t.Fatal("sortSnapshots ordering wrong")
	}
	if snaps[2].IndicatorID != i2 {
		t.Fatal("sortSnapshots ordering wrong")
	}
	if snaps[3].UnitID != u2 {
		t.Fatal("sortSnapshots ordering wrong")
	}
}

func parseDate(t *testing.T, s string) time.Time {
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
