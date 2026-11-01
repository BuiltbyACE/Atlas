import { test, expect } from '@playwright/test';
import { signIn, signOut } from './helpers/auth';

test.describe('Authentication', () => {
  test('sign in with valid credentials', async ({ page }) => {
    await signIn(page);
    await expect(page.locator('h1')).toContainText('Dashboard');
  });

  test('sign in and sign out', async ({ page }) => {
    await signIn(page);
    await signOut(page);
    await expect(page).toHaveURL('/login');
    await expect(page.locator('h1')).toContainText('Sign in');
  });

  test('rejects invalid credentials', async ({ page }) => {
    await page.goto('/login');
    await page.fill('input[id="atl-identifier"]', 'nonexistent@example.com');
    await page.fill('input[id="atl-password"]', 'wrongpassword');
    await page.click('button:has-text("Sign in")');

    // Should show an error, not redirect to dashboard
    await expect(page).not.toHaveURL('/dashboard');
    await expect(page.locator('[role="alert"]')).toBeVisible();
  });

  test('requires authentication for dashboard', async ({ page }) => {
    await page.goto('/dashboard');
    // Should redirect to login if not authenticated
    await expect(page).toHaveURL(/\/login/);
  });
});
