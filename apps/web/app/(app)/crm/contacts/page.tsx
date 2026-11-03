import Link from "next/link";

import { ButtonLink } from "@/components/form";
import { Icon } from "@/components/icons";
import { SignalRefresh } from "@/components/signal-refresh";
import {
  Badge,
  Card,
  EmptyState,
  PageHeader,
  ProblemNotice,
  Table,
  Td,
  Th,
} from "@/components/ui";
import {
  listContacts,
  listOrganizations,
  type CrmContact,
  type CrmOrganization,
} from "@/lib/api/atlas";
import { ApiError } from "@/lib/api/client";

/**
 * Contacts index (`GET /api/v1/crm/contacts`, gated by `crm.registry.read`;
 * §8.1). People at organizations, with the consent state that governs how
 * they may be contacted (Atlas records consent, never assumes it).
 */
export const dynamic = "force-dynamic";

/** A settled load: either a value or the `ApiError` Go returned. */
async function load<T>(
  fn: () => Promise<T>,
): Promise<{ value: T | null; error: ApiError | null }> {
  try {
    return { value: await fn(), error: null };
  } catch (caught) {
    if (!(caught instanceof ApiError)) throw caught;
    return { value: null, error: caught };
  }
}

export default async function ContactsPage() {
  const [contactsPage, organizationsPage] = await Promise.all([
    load(() => listContacts({ limit: 200 })),
    load(() => listOrganizations({ limit: 200 })),
  ]);

  if (contactsPage.error) {
    return (
      <div className="space-y-6">
        <PageHeader title="Contacts" description="People at organizations." />
        <ProblemNotice
          error={contactsPage.error}
          title={
            contactsPage.error.status === 403
              ? "You do not have permission to view contacts"
              : "Contacts could not be loaded"
          }
        />
      </div>
    );
  }

  const contacts = contactsPage.value?.items ?? [];
  const organizationById = new Map<string, CrmOrganization>(
    (organizationsPage.value?.items ?? []).map((o) => [o.id, o]),
  );

  return (
    <div className="space-y-6">
      <PageHeader
        title="Contacts"
        description="People at organizations, and how they agreed to be contacted."
        actions={
          <>
            <SignalRefresh topic="crm" quiet />
            <ButtonLink href="/crm/contacts/new" variant="primary">
              <Icon name="plus" className="h-4 w-4" />
              New contact
            </ButtonLink>
          </>
        }
      />

      <Card padded={false}>
        {contacts.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="people"
              iconTone="info"
              className="min-h-[220px]"
              title="No contacts yet"
            >
              Add the first person at an organization.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Contact</Th>
                <Th>Organization</Th>
                <Th>Phone</Th>
                <Th>Email</Th>
                <Th>Consent</Th>
              </tr>
            </thead>
            <tbody>
              {contacts.map((contact) => {
                const organization = contact.organization_id
                  ? organizationById.get(contact.organization_id)
                  : undefined;
                return (
                  <tr key={contact.id}>
                    <Td>
                      <div className="flex flex-col items-start gap-1">
                        <Link
                          href={`/crm/contacts/${contact.id}`}
                          className="font-semibold text-night hover:underline"
                        >
                          {contact.name}
                        </Link>
                        {contact.role && (
                          <span className="text-[13px] text-muted">
                            {contact.role}
                          </span>
                        )}
                      </div>
                    </Td>
                    <Td>
                      {organization ? (
                        <Link
                          href={`/crm/organizations/${organization.id}`}
                          className="font-medium text-night hover:underline"
                        >
                          {organization.name}
                        </Link>
                      ) : (
                        <span className="text-muted">—</span>
                      )}
                    </Td>
                    <Td className="tabular text-muted">
                      {contact.phones[0] ?? contact.whatsapp_number ?? "—"}
                    </Td>
                    <Td className="tabular text-muted">
                      {contact.emails[0] ?? "—"}
                    </Td>
                    <Td>
                      <div className="flex flex-wrap items-center gap-1.5">
                        {contact.consent_marketing && (
                          <Badge tone="success">Marketing</Badge>
                        )}
                        {contact.consent_whatsapp && (
                          <Badge tone="success">WhatsApp</Badge>
                        )}
                        {contact.consent_call_recording && (
                          <Badge tone="success">Call recording</Badge>
                        )}
                        {!contact.consent_marketing &&
                          !contact.consent_whatsapp &&
                          !contact.consent_call_recording && (
                            <Badge tone="neutral">None recorded</Badge>
                          )}
                      </div>
                    </Td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}
