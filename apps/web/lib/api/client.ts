/**
 * Atlas web - thin typed fetch wrapper around the GENERATED OpenAPI client.
 *
 * IMPORTANT: the real, fully typed client is **generated** from
 * `contracts/openapi/public.yaml` (the Go API's OpenAPI 3.1 spec) into
 * `lib/api/generated/`. Those files are generated output and must never be
 * hand-edited. The `generate:api` script (see package.json / README.md) will run
 * `openapi-typescript` against that spec once `public.yaml` exists.
 *
 * This module is the small, stable seam in front of it: it resolves the base
 * URL, forwards the session cookie on the server, serialises JSON bodies and
 * turns failures into RFC 9457 Problem Details errors.
 *
 * Contract references: blueprint sections 5.2 (Go owns all writes and
 * permissions), 5.3 (same-origin routing, server components forward the session
 * cookie) and 11 (RFC 9457 error shape, stable `code` values).
 */

/** RFC 9457 Problem Details, extended with Atlas' stable error `code`. */
export interface ProblemDetails {
  /** A URI reference identifying the problem type. */
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  /** A URI reference identifying this specific occurrence. */
  instance?: string;
  /** Stable Atlas error code, e.g. `TASK_EVIDENCE_REQUIRED`. */
  code?: string;
  /** Field-level validation errors, keyed by field path. */
  errors?: Record<string, string[]>;
  [key: string]: unknown;
}

/** Error thrown for any non-2xx response from the Go API. */
export class ApiError extends Error {
  readonly status: number;
  readonly problem: ProblemDetails;

  constructor(status: number, problem: ProblemDetails) {
    super(problem.detail ?? problem.title ?? `API request failed (${status})`);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }

  /** Stable machine-readable code from the API, when present. */
  get code(): string | undefined {
    return this.problem.code;
  }
}

type QueryValue = string | number | boolean | null | undefined;

export interface ApiFetchOptions
  extends Omit<RequestInit, "body" | "headers"> {
  /** Serialised to JSON unless it is already a `FormData` body. */
  body?: unknown;
  /** Appended to the URL as a query string, skipping null/undefined values. */
  query?: Record<string, QueryValue>;
  /** Explicit cookie header to send (overrides server cookie forwarding). */
  cookie?: string;
  headers?: HeadersInit;
}

/**
 * Resolve the API base URL.
 *
 * - Browser: `NEXT_PUBLIC_API_BASE_URL` (empty means same-origin, as Caddy
 *   routes `/api/*` to Go in production).
 * - Server: `ATLAS_API_INTERNAL_URL` (the private network address of Go).
 */
function resolveBaseUrl(): string {
  if (typeof window !== "undefined") {
    return process.env.NEXT_PUBLIC_API_BASE_URL ?? "";
  }
  return (
    process.env.ATLAS_API_INTERNAL_URL ??
    process.env.NEXT_PUBLIC_API_BASE_URL ??
    "http://localhost:8080"
  );
}

function buildUrl(path: string, query?: Record<string, QueryValue>): string {
  const base = resolveBaseUrl().replace(/\/+$/, "");
  const normalized = path.startsWith("/") ? path : `/${path}`;
  const search = new URLSearchParams();
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== null) {
        search.set(key, String(value));
      }
    }
  }
  const qs = search.toString();
  return `${base}${normalized}${qs ? `?${qs}` : ""}`;
}

/**
 * On the server, forward the incoming request's session cookie so Go authorises
 * the call as the current user (blueprint section 5.7). Imported lazily so this
 * module stays safe to reference from client code.
 */
async function readServerCookieHeader(): Promise<string | undefined> {
  if (typeof window !== "undefined") return undefined;
  try {
    const { cookies } = await import("next/headers");
    const store = await cookies();
    const cookieHeader = store.toString();
    return cookieHeader.length > 0 ? cookieHeader : undefined;
  } catch {
    // No request scope (e.g. build time) - send no cookie.
    return undefined;
  }
}

async function toApiError(response: Response): Promise<ApiError> {
  let problem: ProblemDetails = {
    status: response.status,
    title: response.statusText,
  };
  try {
    const data: unknown = await response.json();
    if (data && typeof data === "object") {
      const parsed = data as ProblemDetails;
      problem = { ...parsed, status: parsed.status ?? response.status };
    }
  } catch {
    // Non-JSON body (or empty): keep the status-based problem.
  }
  return new ApiError(response.status, problem);
}

/**
 * Typed fetch against the Go API.
 *
 * @example
 * const me = await apiFetch<CurrentUser>("/api/v1/me", { cache: "no-store" });
 */
export async function apiFetch<T = unknown>(
  path: string,
  options: ApiFetchOptions = {},
): Promise<T> {
  const { body, query, cookie, headers, ...rest } = options;

  const requestHeaders = new Headers(headers);
  if (!requestHeaders.has("Accept")) {
    requestHeaders.set("Accept", "application/json");
  }

  let requestBody: BodyInit | undefined;
  if (body instanceof FormData) {
    requestBody = body;
  } else if (body !== undefined) {
    if (!requestHeaders.has("Content-Type")) {
      requestHeaders.set("Content-Type", "application/json");
    }
    requestBody = JSON.stringify(body);
  }

  const cookieHeader = cookie ?? (await readServerCookieHeader());
  if (cookieHeader) {
    requestHeaders.set("cookie", cookieHeader);
  }

  const response = await fetch(buildUrl(path, query), {
    ...rest,
    headers: requestHeaders,
    body: requestBody,
  });

  if (!response.ok) {
    throw await toApiError(response);
  }

  if (response.status === 204 || response.status === 205) {
    return undefined as T;
  }

  const text = await response.text();
  if (text.length === 0) {
    return undefined as T;
  }

  return JSON.parse(text) as T;
}
