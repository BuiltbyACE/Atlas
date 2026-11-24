import { AccessDenied } from "@/components/access-denied";
import { PageHeader } from "@/components/ui";

/**
 * The shared "not for you" destination.
 *
 * Go is the authority on every page; this route exists so a link can point at a
 * consistent explanation instead of a bare error, and so a test has one place to
 * assert the message. It is inside the authenticated layout, so the shell still
 * shows the destinations this person actually has.
 */
export default function AccessDeniedPage() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Access denied"
        description="This destination is outside the permissions your account holds."
      />
      <AccessDenied title="That page is not available to you" />
    </div>
  );
}
