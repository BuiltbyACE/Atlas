import { test, expect } from '@playwright/test';
import { signIn } from './helpers/auth';

test.describe('Daily Reports', () => {
  test('submit a daily report', async ({ page }) => {
    await signIn(page);

    // Navigate to daily reports
    await page.click('a:has-text("Daily reports")');
    await expect(page).toHaveURL('/daily-reports');

    // Click to create a new report
    await page.click('button:has-text("File today\'s report")');

    // Fill in report
    await page.fill('textarea[name="summary"]', 'E2E test daily report');
    await page.click('button[type="submit"]');

    // Verify report was submitted
    await expect(page.locator('text=E2E test daily report')).toBeVisible();
  });
});
