import { test, expect, type Page } from '@playwright/test';

/**
 * Test credentials. Set these via environment variables or use the defaults.
 * For local development, ensure a test user exists in the database
 * (e.g. via seeding or manual creation).
 */
export const TEST_USER = {
  email: process.env.E2E_TEST_EMAIL || 'test@example.com',
  password: process.env.E2E_TEST_PASSWORD || 'TestPassword123!',
};

/**
 * Signs in a user via the login flow. Handles both password-only and MFA
 * protected accounts. On success, asserts the dashboard is loaded.
 */
export async function signIn(page: Page, email?: string, password?: string) {
  const emailToUse = email ?? TEST_USER.email;
  const passwordToUse = password ?? TEST_USER.password;

  await page.goto('/login');

  // If already signed in, we should be redirected to dashboard
  const currentUrl = page.url();
  if (currentUrl.includes('/dashboard')) {
    return;
  }

  // Fill in the sign-in form (input ids: atl-identifier, atl-password)
  await page.fill('input[id="atl-identifier"]', emailToUse);
  await page.fill('input[id="atl-password"]', passwordToUse);
  await page.click('button:has-text("Sign in")');

  // Wait for either dashboard redirect or MFA challenge
  await page.waitForURL(/(\/dashboard|\/auth\/mfa)/, { timeout: 10000 });

  // If MFA is required, handle it
  if (page.url().includes('/auth/mfa')) {
    await page.fill('input[id="atl-code"]', process.env.E2E_TEST_MFA_CODE || '123456');
    await page.click('button:has-text("Verify")');
    await page.waitForURL('/dashboard', { timeout: 10000 });
  }

  // Verify we're on the dashboard
  await expect(page).toHaveURL('/dashboard');
  await expect(page.locator('h1')).toContainText('Dashboard');
}

/**
 * Signs out the current user via the sign-out button in the sidebar.
 */
export async function signOut(page: Page) {
  // Click the sign out button in the sidebar
  await page.click('button:has-text("Sign out")');
  await page.waitForURL('/login', { timeout: 10000 });
}
