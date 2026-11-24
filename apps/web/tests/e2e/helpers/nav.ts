import type { Page } from '@playwright/test';

/**
 * The redesigned sidebar renders each destination more than once (a "Recent"
 * group and its full section). Some pages also repeat the same link in the
 * main content. Anchoring on the href keeps navigation deterministic without
 * depending on which of those intentional duplicates happens to come first.
 */
export function navLink(page: Page, href: string) {
  return page.locator(`a[href="${href}"]`).first();
}
