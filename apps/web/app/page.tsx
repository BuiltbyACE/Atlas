import { redirect } from "next/navigation";

/**
 * The app root sends unauthenticated visitors to the dashboard, which lives in
 * the authenticated `(app)` route group. If they have no session that layout
 * redirects them on to `/login`.
 */
export default function HomePage(): never {
  redirect("/dashboard");
}
