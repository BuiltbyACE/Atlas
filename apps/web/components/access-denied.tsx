import { ButtonLink } from "@/components/form";
import { EmptyState, type ProblemLike } from "@/components/ui";

/**
 * The one place a page says "Go refused this for you".
 *
 * The sidebar hides destinations the caller cannot open, but hiding is never the
 * guard (blueprint §5.2): a bookmark, a shared link or a permission revoked while
 * a tab was open can still land on a page. This keeps the explanation and the way
 * back identical everywhere, and never suggests the page might work on a retry.
 *
 * `error` is optional: pass the `ApiError` when there is one so its stable code
 * and request id stay available for a support report.
 */
export function AccessDenied({
  title = "You do not have access to this page",
  message = "Your account does not include the permission this page needs. If you believe that is wrong, ask an administrator to review your role.",
  error,
  backHref = "/dashboard",
  backLabel = "Back to your dashboard",
}: {
  title?: string;
  message?: string;
  /** The refusal Go returned, so its code and request id are not lost. */
  error?: ProblemLike;
  backHref?: string;
  backLabel?: string;
}) {
  return (
    <div className="space-y-3">
      <EmptyState
        icon="shield"
        iconTone="warning"
        title={title}
        className="bg-white"
        action={
          <ButtonLink href={backHref} variant="secondary">
            {backLabel}
          </ButtonLink>
        }
      >
        {message}
      </EmptyState>
      {error && (error.code || error.requestId) && (
        <p className="break-all px-1 font-mono text-xs text-muted">
          {error.code}
          {error.code && error.requestId && " · "}
          {error.requestId && <>ref {error.requestId}</>}
        </p>
      )}
    </div>
  );
}
