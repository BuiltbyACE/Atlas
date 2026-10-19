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
      { label: "Projects", href: "/work", icon: "folder", section: "Work" },
      { label: "Task board", href: "/work/tasks", icon: "board", section: "Work", keywords: "kanban tasks" },
      { label: "Workload", href: "/work/workload", icon: "bars", section: "Work", keywords: "capacity deadlines" },
      { label: "Escalations", href: "/work/escalations", icon: "alert", section: "Work", badge: "escalations", keywords: "overdue" },
    ],
  },
  {
    title: "People",
    links: [
      { label: "Availability & leave", href: "/people", icon: "calendar", section: "People", keywords: "roster holiday absence" },
      { label: "Activity", href: "/people/activity", icon: "pulse", section: "People", keywords: "sessions online sign-in" },
      { label: "Daily reports", href: "/daily-reports", icon: "clipboard", section: "People", keywords: "report standup" },
    ],
  },
  {
    title: "Revenue",
    links: [
      { label: "CRM", href: "/crm", icon: "people", section: "Revenue", keywords: "leads pipeline sales opportunities contacts clients" },
      { label: "Contracts", href: "/contracts", icon: "document", section: "Revenue", keywords: "agreements signing milestones renewals obligations" },
      { label: "Finance", href: "/finance", icon: "card", section: "Revenue", keywords: "cash receivables expenses invoices budgets payments profitability recurring" },
    ],
  },
];

/** Modules on the roadmap, shown dimmed with a "Soon" tag. */
export const COMING_NEXT: Array<{ label: string; icon: IconName }> = [
  { label: "Systems", icon: "server" },
  { label: "Units", icon: "building" },
  { label: "Intel", icon: "pie" },
];

export const SETTINGS_LINK: NavLink = {
  label: "Settings",
  href: "/settings",
  icon: "sliders",
  keywords: "sessions devices account",
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
