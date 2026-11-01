/**
 * Atlas web - the workspace map: sidebar sections, breadcrumbs and the
 * quick-jump list all read from here, so a page is named the same everywhere.
 *
 * Modules from blueprint §4 (capability map). Phase 2 built out Work and People,
 * Phase 3 added CRM, Contracts and Finance; the modules still on the roadmap are
 * listed under "Coming next" as muted labels rather than links to a 404, so the
 * shell never promises a page that does not exist.
 */

import type { IconName } from "@/components/icons";

export interface NavLink {
  label: string;
  href: string;
  icon: IconName;
  /** Section shown in the breadcrumb before the page name. */
  section?: string;
  /** Extra words the quick-jump matches on. */
  keywords?: string;
  /** Show the open-escalations count beside it. */
  badge?: "escalations";
  /** Role keys allowed to see this link. Absent = visible to all authenticated users. */
  roles?: string[];
}

/** Role groups used for navigation gating. */
export const ROLE_GROUPS = {
  ceo_internal: ["ceo_internal"],
  ceo_commercial: ["ceo_commercial"],
  finance_officer: ["finance_officer"],
  sales_rep: ["sales_rep"],
  project_lead: ["project_lead"],
  engineer: ["engineer"],
  staff: ["staff"],
  unit_manager: ["unit_manager"],
  auditor: ["auditor"],
  system_admin: ["system_admin"],
} as const;

/** Returns true when the user holds at least one of the required roles.
 * When `required` is empty or undefined, the link is visible to everyone. */
export function hasAnyRole(userRoles: string[], required?: string[]): boolean {
  if (!required || required.length === 0) return true;
  return required.some((r) => userRoles.includes(r));
}

/** Filters nav links to those the user is allowed to see. */
export function filterLinksByRoles(links: NavLink[], userRoles: string[]): NavLink[] {
  return links.filter((link) => hasAnyRole(userRoles, link.roles));
}

/** Returns only the sections whose links are visible to the user. */
export function filterSectionsByRoles(
  sections: NavSection[],
  userRoles: string[],
): NavSection[] {
  return sections
    .map((section) => ({
      ...section,
      links: filterLinksByRoles(section.links, userRoles),
    }))
    .filter((section) => section.links.length > 0);
}

export interface NavSection {
  title?: string;
  links: NavLink[];
}

export const NAV_SECTIONS: NavSection[] = [
  {
    links: [{ label: "Dashboard", href: "/dashboard", icon: "dashboard", keywords: "home overview" }],
  },
  {
    title: "Work",
    links: [
      { label: "Approvals", href: "/approvals", icon: "shieldCheck", section: "Work", keywords: "approve decline escalate", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Projects", href: "/work", icon: "folder", section: "Work", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Task board", href: "/work/tasks", icon: "board", section: "Work", keywords: "kanban tasks", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Workload", href: "/work/workload", icon: "bars", section: "Work", keywords: "capacity deadlines", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Escalations", href: "/work/escalations", icon: "alert", section: "Work", badge: "escalations", keywords: "overdue", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
    ],
  },
  {
    title: "People",
    links: [
      { label: "Availability & leave", href: "/people", icon: "calendar", section: "People", keywords: "roster holiday absence", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Activity", href: "/people/activity", icon: "pulse", section: "People", keywords: "sessions online sign-in", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Daily reports", href: "/daily-reports", icon: "clipboard", section: "People", keywords: "report standup", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Departments", href: "/people/departments", icon: "building", section: "People", keywords: "departments units teams", roles: ["ceo_internal", "ceo_commercial", "finance_officer", "sales_rep", "project_lead", "engineer", "staff"] },
      { label: "Onboard new person", href: "/people/onboard", icon: "plus", section: "People", keywords: "invite new hire register", roles: ["ceo_internal", "ceo_commercial", "system_admin", "finance_officer"] },
    ],
  },
  {
    title: "Revenue",
    links: [
      { label: "Commercial dashboard", href: "/dashboard/commercial", icon: "pie", section: "Revenue", keywords: "pipeline receivables cash leads", roles: ["ceo_internal", "ceo_commercial"] },
      { label: "Daily Brief", href: "/dashboard/daily", icon: "clipboard", section: "Revenue", keywords: "yesterday today attention decisions", roles: ["ceo_internal", "ceo_commercial"] },
      { label: "CRM", href: "/crm", icon: "people", section: "Revenue", keywords: "leads pipeline sales opportunities contacts clients", roles: ["ceo_commercial", "ceo_internal", "sales_rep"] },
      { label: "Contracts", href: "/contracts", icon: "document", section: "Revenue", keywords: "agreements signing milestones renewals obligations", roles: ["ceo_commercial", "ceo_internal"] },
      { label: "Finance", href: "/finance", icon: "card", section: "Revenue", keywords: "cash receivables expenses invoices budgets payments profitability recurring", roles: ["ceo_internal", "ceo_commercial", "finance_officer"] },
    ],
  },
  {
    title: "Platform",
    links: [
      { label: "Systems", href: "/systems", icon: "server", section: "Platform", keywords: "monitoring uptime incidents checks domains certificates credentials health", roles: ["ceo_internal", "system_admin"] },
    ],
  },
];

/** Modules on the roadmap, shown dimmed with a "Soon" tag. */
export interface ComingNextItem {
  label: string;
  icon: IconName;
  roles?: string[];
}

export const COMING_NEXT: ComingNextItem[] = [
  { label: "Units", icon: "building", roles: ["ceo_internal", "ceo_commercial", "unit_manager"] },
  { label: "Intel", icon: "pie", roles: ["ceo_internal", "ceo_commercial"] },
];

export const SETTINGS_LINK: NavLink = {
  label: "Settings",
  href: "/settings",
  icon: "sliders",
  keywords: "sessions devices account",
  roles: ["ceo_internal", "ceo_commercial", "system_admin", "finance_officer"],
};

export const ALL_LINKS: NavLink[] = [
  ...NAV_SECTIONS.flatMap((section) => section.links),
  SETTINGS_LINK,
];

/** The nav link a path belongs to (longest matching prefix wins). */
export function linkForPath(pathname: string): NavLink | undefined {
  return ALL_LINKS.filter(
    (link) => pathname === link.href || pathname.startsWith(`${link.href}/`),
  ).sort((a, b) => b.href.length - a.href.length)[0];
}

/** Breadcrumb trail for a path: section, page, and a detail leaf if deeper. */
export function breadcrumbFor(
  pathname: string,
): Array<{ label: string; href?: string }> {
  const link = linkForPath(pathname);
  if (!link) return [];
  const trail: Array<{ label: string; href?: string }> = [];
  if (link.section) trail.push({ label: link.section });
  if (pathname === link.href) {
    trail.push({ label: link.label });
    return trail;
  }
  trail.push({ label: link.label, href: link.href });
  if (pathname.startsWith("/work/projects/")) trail.push({ label: "Project" });
  else if (pathname.startsWith("/work/tasks/")) trail.push({ label: "Task" });
  return trail;
}
