package crm

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// contactTestOrg creates a uniquely-named organization and registers cleanup
// that removes its contacts first, so a failed assertion cannot leave an orphan
// contact that blocks the organization delete on the next run.
func contactTestOrg(t *testing.T, ctx context.Context, pool *pgxpool.Pool, svc *Service, actor uuid.UUID) Organization {
	t.Helper()
	org, err := svc.CreateOrganization(ctx, actor, OrganizationInput{Name: "Consent Test " + uuid.NewString()})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM crm.contacts WHERE organization_id = $1`, org.ID)
		_, _ = pool.Exec(ctx, `DELETE FROM crm.organizations WHERE id = $1`, org.ID)
	})
	return org
}

func assertConsentSource(t *testing.T, field string, got *string, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("%s = %q, want NULL", field, *got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s = NULL, want %q", field, want)
	}
	if *got != want {
		t.Fatalf("%s = %q, want %q", field, *got, want)
	}
}

// TestContactConsentSourcesAllNull proves a contact created without any consent
// detail round-trips through insert, get and list with SQL NULL sources.
func TestContactConsentSourcesAllNull(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := mustPerson(t, pool)
	org := contactTestOrg(t, ctx, pool, svc, actor)

	created, err := svc.CreateContact(ctx, actor, ContactInput{OrganizationID: &org.ID, Name: "No Consent"})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if created.ConsentMarketingAt != nil || created.ConsentWhatsAppAt != nil || created.ConsentRecordingAt != nil {
		t.Fatalf("consent timestamps = %+v, want all NULL", created)
	}
	assertConsentSource(t, "consent_marketing_source", created.ConsentMarketingSource, "")
	assertConsentSource(t, "consent_whatsapp_source", created.ConsentWhatsAppSource, "")
	assertConsentSource(t, "consent_call_recording_source", created.ConsentRecordingSource, "")

	got, err := svc.GetContact(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	assertConsentSource(t, "consent_marketing_source", got.ConsentMarketingSource, "")
	assertConsentSource(t, "consent_whatsapp_source", got.ConsentWhatsAppSource, "")
	assertConsentSource(t, "consent_call_recording_source", got.ConsentRecordingSource, "")

	listed, err := svc.ListContacts(ctx, &org.ID, 50, nil)
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed = %+v, want the new contact", listed)
	}
	assertConsentSource(t, "listed consent_marketing_source", listed[0].ConsentMarketingSource, "")
	assertConsentSource(t, "listed consent_whatsapp_source", listed[0].ConsentWhatsAppSource, "")
	assertConsentSource(t, "listed consent_call_recording_source", listed[0].ConsentRecordingSource, "")
}

// TestContactConsentSourcesAllSet proves consent granted for every channel
// stores and scans a source for each column.
func TestContactConsentSourcesAllSet(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := mustPerson(t, pool)
	org := contactTestOrg(t, ctx, pool, svc, actor)

	created, err := svc.CreateContact(ctx, actor, ContactInput{
		OrganizationID: &org.ID, Name: "Full Consent",
		ConsentMarketing: true, ConsentWhatsApp: true, ConsentRecording: true,
		ConsentSource: "web_form",
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	assertConsentSource(t, "consent_marketing_source", created.ConsentMarketingSource, "web_form")
	assertConsentSource(t, "consent_whatsapp_source", created.ConsentWhatsAppSource, "web_form")
	assertConsentSource(t, "consent_call_recording_source", created.ConsentRecordingSource, "web_form")
	if created.ConsentMarketingAt == nil || created.ConsentWhatsAppAt == nil || created.ConsentRecordingAt == nil {
		t.Fatalf("consent timestamps = %+v, want all set", created)
	}

	got, err := svc.GetContact(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	assertConsentSource(t, "consent_marketing_source", got.ConsentMarketingSource, "web_form")
	assertConsentSource(t, "consent_whatsapp_source", got.ConsentWhatsAppSource, "web_form")
	assertConsentSource(t, "consent_call_recording_source", got.ConsentRecordingSource, "web_form")
}

// TestContactConsentSourcesMixed proves NULL and non-NULL sources coexist on a
// single row without the scanner confusing the columns.
func TestContactConsentSourcesMixed(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := mustPerson(t, pool)
	org := contactTestOrg(t, ctx, pool, svc, actor)

	created, err := svc.CreateContact(ctx, actor, ContactInput{
		OrganizationID: &org.ID, Name: "Marketing Only",
		ConsentMarketing: true, ConsentSource: "phone_call",
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	assertConsentSource(t, "consent_marketing_source", created.ConsentMarketingSource, "phone_call")
	assertConsentSource(t, "consent_whatsapp_source", created.ConsentWhatsAppSource, "")
	assertConsentSource(t, "consent_call_recording_source", created.ConsentRecordingSource, "")
	if created.ConsentMarketingAt == nil || created.ConsentWhatsAppAt != nil || created.ConsentRecordingAt != nil {
		t.Fatalf("consent timestamps = %+v, want only marketing set", created)
	}

	got, err := svc.GetContact(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetContact: %v", err)
	}
	assertConsentSource(t, "consent_marketing_source", got.ConsentMarketingSource, "phone_call")
	assertConsentSource(t, "consent_whatsapp_source", got.ConsentWhatsAppSource, "")
	assertConsentSource(t, "consent_call_recording_source", got.ConsentRecordingSource, "")
}

// TestContactUpdatePreservesConsent proves a normal update keeps the existing
// consent evidence, and that revoking one channel clears only that channel.
func TestContactUpdatePreservesConsent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil)
	actor := mustPerson(t, pool)
	org := contactTestOrg(t, ctx, pool, svc, actor)

	created, err := svc.CreateContact(ctx, actor, ContactInput{
		OrganizationID: &org.ID, Name: "Consent Keeper",
		ConsentMarketing: true, ConsentWhatsApp: true, ConsentRecording: true,
		ConsentSource: "contract",
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	// Rename without touching consent: sources and timestamps must survive.
	renamed, err := svc.UpdateContact(ctx, actor, created.ID, created.Version, ContactInput{
		OrganizationID: &org.ID, Name: "Consent Keeper Renamed",
		ConsentMarketing: true, ConsentWhatsApp: true, ConsentRecording: true,
		ConsentSource: "ignored_because_already_granted",
	})
	if err != nil {
		t.Fatalf("UpdateContact: %v", err)
	}
	assertConsentSource(t, "consent_marketing_source", renamed.ConsentMarketingSource, "contract")
	assertConsentSource(t, "consent_whatsapp_source", renamed.ConsentWhatsAppSource, "contract")
	assertConsentSource(t, "consent_call_recording_source", renamed.ConsentRecordingSource, "contract")
	if renamed.ConsentMarketingAt == nil || !renamed.ConsentMarketingAt.Equal(*created.ConsentMarketingAt) {
		t.Fatalf("marketing consent timestamp changed on update: %v -> %v", created.ConsentMarketingAt, renamed.ConsentMarketingAt)
	}

	// Revoke WhatsApp only; the other channels keep their evidence.
	revoked, err := svc.UpdateContact(ctx, actor, renamed.ID, renamed.Version, ContactInput{
		OrganizationID: &org.ID, Name: renamed.Name,
		ConsentMarketing: true, ConsentWhatsApp: false, ConsentRecording: true,
	})
	if err != nil {
		t.Fatalf("UpdateContact(revoke): %v", err)
	}
	assertConsentSource(t, "consent_marketing_source", revoked.ConsentMarketingSource, "contract")
	assertConsentSource(t, "consent_whatsapp_source", revoked.ConsentWhatsAppSource, "")
	assertConsentSource(t, "consent_call_recording_source", revoked.ConsentRecordingSource, "contract")
	if revoked.ConsentWhatsAppAt != nil {
		t.Fatalf("whatsapp consent timestamp = %v, want NULL after revoke", revoked.ConsentWhatsAppAt)
	}
	if revoked.ConsentMarketingAt == nil || revoked.ConsentRecordingAt == nil {
		t.Fatalf("revoking whatsapp cleared another channel: %+v", revoked)
	}
}
