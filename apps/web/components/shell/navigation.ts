/**
 * Atlas web - the workspace map the shell renders.
 *
 * Go is the authority (`GET /api/v1/me/navigation`): it decides which
 * destinations appear and which actions each destination offers, from the
 * caller's effective permissions. This module only translates the wire shape
 * into what the shell needs: a typed icon name, a flat list for search, and
 * breadcrumbs. It never decides access, and it never maps a role to a menu.
 *
 * `FALLBACK_NAVIGATION` is deliberately tiny. When the navigation endpoint
 * fails, the shell shows only destinations that every signed-in person may open
 * (the dashboard and their own settings). It never guesses at module access, so
 * a degraded response can hide a control but never offer an unauthorised one.
 */

import type { IconName } from "@/components/icons";
import {
  getNavigation,
  type Navigation,
  type NavigationItem,
  type NavigationSection,
} from "@/lib/api/atlas";

/** A destination with its icon resolved to the local icon set. */
export interface NavItem extends Omit<NavigationItem, "icon"> {
  icon: IconName;
}

export interface NavSection extends Omit<NavigationSection, "items"> {
  items: NavItem[];
}

export interface NavModel {
  sections: NavSection[];
  /** Every visible destination, in sidebar order. */
  items: NavItem[];
  /** The caller's effective permissions, as Go reported them. */
  permissions: string[];
}

/**
 * Go emits icon names as plain strings; this map is the only place the web app
 * binds them to the icon set. An unknown name falls back to the dashboard mark
 * rather than throwing, so a new Go destination never breaks the shell.
 */
const ICONS: Record<string, IconName> = {
  dashboard: "dashboard",
  folder: "folder",
  board: "board",
  bars: "bars",
  alert: "alert",
  calendar: "calendar",
  calendarPlus: "calendarPlus",
  pulse: "pulse",
  clipboard: "clipboard",
  people: "people",
  document: "document",
  card: "card",
  server: "server",
  building: "building",
  pie: "pie",
  sliders: "sliders",
  shieldCheck: "shieldCheck",
  github: "github",
  plus: "plus",
  clock: "clock",
};

export function iconFor(name: string): IconName {
  return ICONS[name] ?? "dashboard";
}

/** Translate the navigation response into the shell's model. */
export function toNavModel(navigation: Navigation): NavModel {
  const sections: NavSection[] = navigation.sections.map((section) => ({
    key: section.key,
    title: section.title,
    items: section.items.map((item) => ({
      ...item,
      icon: iconFor(item.icon),
    })),
  }));
  return {
    sections,
    items: sections.flatMap((section) => section.items),
    permissions: navigation.permissions,
  };
}

/** The model used only when the navigation endpoint cannot be read. */
export const FALLBACK_NAVIGATION: Navigation = {
  sections: [
    {
      key: "home",
      items: [
        {
          key: "dashboard",
          label: "Dashboard",
          href: "/dashboard",
          icon: "dashboard",
          keywords: "home overview",
          actions: {},
        },
      ],
    },
    {
      key: "account",
      items: [
        {
          key: "settings",
          label: "Settings",
          href: "/settings",
          icon: "sliders",
          keywords: "sessions devices account",
          actions: {},
        },
      ],
    },
  ],
  permissions: [],
};

/** Read a navigation response, or the safe fallback when it cannot be read. */
export async function loadNavigation(): Promise<NavModel> {
  try {
    return toNavModel(await getNavigation());
  } catch {
    return toNavModel(FALLBACK_NAVIGATION);
  }
}

/**
 * Every action boolean across the visible destinations, merged. When two
 * destinations declare the same permission they agree (Go derived both from the
 * same principal), so the merge is a union. Used by the global create menu and
 * quick jump, which are not tied to one page.
 */
export function allActions(model: NavModel): Record<string, boolean> {
  const merged: Record<string, boolean> = {};
  for (const item of model.items) {
    for (const [permission, allowed] of Object.entries(item.actions)) {
      merged[permission] = merged[permission] === true || allowed;
    }
  }
  return merged;
}

/** The nav item a path belongs to (longest matching href wins). */
export function linkForPath(model: NavModel, pathname: string): NavItem | undefined {
  return model.items
    .filter(
      (item) => pathname === item.href || pathname.startsWith(`${item.href}/`),
    )
    .sort((a, b) => b.href.length - a.href.length)[0];
}

/** Breadcrumb trail for a path: section, page, and a detail leaf if deeper. */
export function breadcrumbFor(
  model: NavModel,
  pathname: string,
): Array<{ label: string; href?: string }> {
  const item = linkForPath(model, pathname);
  if (!item) return [];
  const trail: Array<{ label: string; href?: string }> = [];
  if (item.section) trail.push({ label: item.section });
  if (pathname === item.href) {
    trail.push({ label: item.label });
    return trail;
  }
  trail.push({ label: item.label, href: item.href });
  if (pathname.startsWith("/work/projects/")) trail.push({ label: "Project" });
  else if (pathname.startsWith("/work/tasks/")) trail.push({ label: "Task" });
  else if (pathname.startsWith("/crm/leads/")) trail.push({ label: "Lead" });
  else if (pathname.startsWith("/crm/contacts/")) trail.push({ label: "Contact" });
  else if (pathname.startsWith("/crm/organizations/")) trail.push({ label: "Organization" });
  else if (pathname.startsWith("/crm/opportunities/")) trail.push({ label: "Opportunity" });
  else if (pathname.startsWith("/crm/interactions/")) trail.push({ label: "Interaction" });
  else if (pathname.startsWith("/crm/followups/")) trail.push({ label: "Follow-up" });
  else if (pathname.startsWith("/crm/proposals/")) trail.push({ label: "Proposal" });
  else if (pathname.startsWith("/contracts/")) trail.push({ label: "Contract" });
  else if (pathname.startsWith("/email/threads/")) trail.push({ label: "Message" });
  else if (pathname.startsWith("/email/addresses")) trail.push({ label: "Addresses" });
  return trail;
}
