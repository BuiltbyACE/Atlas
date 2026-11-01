import { test, expect } from '@playwright/test';
import { signIn } from './helpers/auth';

test.describe('Finance Module', () => {
  test('view finance dashboard', async ({ page }) => {
    await signIn(page);
    await page.goto('/finance');
    await expect(page.locator('h1')).toContainText('Finance');
  });

  test('create an expense', async ({ page }) => {
    await signIn(page);

    // Navigate to finance
    await page.click('a:has-text("Finance")');
    await expect(page).toHaveURL('/finance');

    // Click "New expense"
    await page.click('button:has-text("New expense")');

    // Fill in expense form
    await page.fill('input[name="category_key"]', 'operational');
    await page.fill('input[name="amount"]', '10000');
    await page.fill('input[name="spent_on"]', '2026-10-01');
    await page.fill('input[name="vendor"]', 'E2E Test Vendor');

    // Submit
    await page.click('button[type="submit"]');

    // Verify expense was created
    await expect(page).toHaveURL('/finance');
  });
});
