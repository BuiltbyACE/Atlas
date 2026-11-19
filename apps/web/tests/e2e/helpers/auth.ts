import { expect, type Page } from '@playwright/test';

/**
 * The disposable CI identity created by `atlas-cli e2e provision`. It is a
 * non-privileged account (`mfa_required = false`), so sign-in is password-only
 * exactly like any other ordinary person's. Override the values locally when
 * pointing the suite at your own throwaway database.
 */
const fullName = process.env.E2E_TEST_NAME || 'Atlas E2E';

export const TEST_USER = {
  email: process.env.E2E_TEST_EMAIL || 'e2e@example.com',
  password: process.env.E2E_TEST_PASSWORD || 'E2eTestPassword123!',
  name: fullName,
  firstName: fullName.trim().split(/\s+/)[0],
};

/** The dashboard greets by first name; this is the h1 the suite anchors on. */
export const DASHBOARD_GREETING = new RegExp(
  `^(Good (morning|afternoon|evening)|Welcome back), ${TEST_USER.firstName}$`,
);

/**
 * Signs in through the real login form and waits for the dashboard. A
 * password-only account lands on /dashboard directly; if Go ever asks this
 * identity for a second factor, the run fails here rather than silently
 * skipping a step.
 */
export async function signIn(page: Page, email?: string, password?: string) {
  const emailToUse = email ?? TEST_USER.email;
  const passwordToUse = password ?? TEST_USER.password;

  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible();

  await page.getByLabel('Email or phone number').fill(emailToUse);
  await page.getByLabel('Password', { exact: true }).fill(passwordToUse);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();

  await page.waitForURL('**/dashboard', { timeout: 15000 });
  await expect(page.locator('h1')).toHaveText(DASHBOARD_GREETING);
}

/** Signs out with the sidebar's compact control and waits for /login. */
export async function signOut(page: Page) {
  await page.getByRole('button', { name: 'Sign out' }).click();
  await page.waitForURL('**/login', { timeout: 10000 });
  await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible();
}
