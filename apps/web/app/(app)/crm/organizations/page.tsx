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
import { personDirectory, personName } from "@/lib/directory";
import { humanise } from "@/lib/format";
import { SALES_TOPIC } from "@/lib/realtime/topics";

/**
 * Organizations index (`GET /api/v1/crm/organizations`, gated by
 * `crm.registry.read`; §8.1). The company register the commercial team sells
 * to, with each row linking to its contacts, leads and opportunities.
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

export default async function OrganizationsPage() {
  const [organizationsPage, contactsPage, names] = await Promise.all([
    load(() => listOrganizations({ limit: 200 })),
    load(() => listContacts({ limit: 500 })),
    personDirectory(),
  ]);

  if (organizationsPage.error) {
    return (
      <div className="space-y-6">
        <PageHeader
          title="Organizations"
          description="The company register."
        />
        <ProblemNotice
          error={organizationsPage.error}
          title={
            organizationsPage.error.status === 403
              ? "You do not have permission to view organizations"
              : "Organizations could not be loaded"
          }
        />
      </div>
    );
  }

  const organizations = organizationsPage.value?.items ?? [];
  const contacts = contactsPage.value?.items ?? [];

  const contactsByOrganization = new Map<string, CrmContact[]>();
  for (const contact of contacts) {
    if (!contact.organization_id) continue;
    const list = contactsByOrganization.get(contact.organization_id) ?? [];
    list.push(contact);
    contactsByOrganization.set(contact.organization_id, list);
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Organizations"
        description="Every company in the register, with its people and records."
        actions={
          <>
            <SignalRefresh topic={SALES_TOPIC} quiet />
            <ButtonLink href="/crm/organizations/new" variant="primary">
              <Icon name="plus" className="h-4 w-4" />
              New organization
            </ButtonLink>
          </>
        }
      />

      <Card padded={false}>
        {organizations.length === 0 ? (
          <div className="p-[22px]">
            <EmptyState
              icon="people"
              iconTone="info"
              className="min-h-[220px]"
              title="No organizations yet"
            >
              Add the first company the team sells to.
            </EmptyState>
          </div>
        ) : (
          <Table>
            <thead>
              <tr>
                <Th>Organization</Th>
                <Th>Industry</Th>
                <Th>Location</Th>
                <Th>Contacts</Th>
                <Th>Status</Th>
                <Th>Owner</Th>
              </tr>
            </thead>
            <tbody>
              {organizations.map((organization) => (
                <tr key={organization.id}>
                  <Td>
                    <div className="flex flex-col items-start gap-1">
                      <Link
                        href={`/crm/organizations/${organization.id}`}
                        className="font-semibold text-night hover:underline"
                      >
                        {organization.name}
                      </Link>
                      {organization.kra_pin && (
                        <span className="text-[13px] text-muted">
                          KRA {organization.kra_pin}
                        </span>
                      )}
                    </div>
                  </Td>
                  <Td className="text-muted">{organization.industry ?? "—"}</Td>
                  <Td className="text-muted">{organization.location ?? "—"}</Td>
                  <Td className="tabular text-muted">
                    {contactsByOrganization.get(organization.id)?.length ?? 0}
                  </Td>
                  <Td>
                    <Badge tone="neutral">
                      {humanise(organization.status)}
                    </Badge>
                  </Td>
                  <Td className="text-muted">
                    {organization.owner_person_id
                      ? (personName(names, organization.owner_person_id) ?? "—")
                      : "—"}
                  </Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Card>
    </div>
  );
}
