import { test, expect } from '@playwright/test';
import { DASHBOARD_GREETING, signIn, signOut } from './helpers/auth';

test.describe('Authentication', () => {
  test('sign in with valid credentials', async ({ page }) => {
    await signIn(page);
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(page.locator('h1')).toHaveText(DASHBOARD_GREETING);
  });

  test('sign in and sign out', async ({ page }) => {
    await signIn(page);
    await signOut(page);
    await expect(page).toHaveURL(/\/login/);
    await expect(page.getByRole('heading', { name: 'Welcome back' })).toBeVisible();
  });

  test('rejects invalid credentials', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Email or phone number').fill('nonexistent@example.com');
    await page.getByLabel('Password', { exact: true }).fill('definitely-wrong');
    await page.getByRole('button', { name: 'Sign in', exact: true }).click();

    await expect(page.getByRole('alert').first()).toBeVisible();
    await expect(page).toHaveURL(/\/login/);
  });

  test('requires authentication for dashboard', async ({ page }) => {
    await page.goto('/dashboard');
    await expect(page).toHaveURL(/\/login/);
  });
});
