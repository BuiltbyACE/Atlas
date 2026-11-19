import { test, expect } from '@playwright/test';
import { signIn } from './helpers/auth';
import { navLink } from './helpers/nav';
import { unique } from './helpers/data';

test.describe('Daily Reports', () => {
  test('submit a daily report', async ({ page }) => {
    const work = unique('E2E report work');

    await signIn(page);
    await navLink(page, '/daily-reports').click();
    await expect(page).toHaveURL(/\/daily-reports$/);
    await expect(page.getByRole('heading', { name: 'Daily report', exact: true })).toBeVisible();

    // The form is reused for filing and editing. A prior run may have left a
    // report for today, so follow whichever control the real state offers.
    const initial = page.getByRole('button', { name: /^(File report|Edit report)$/ });
    await initial.first().waitFor({ state: 'visible' });
    if ((await initial.first().innerText()).trim() === 'Edit report') {
      await initial.first().click();
    }

    await page.locator('#report-work').fill(work);
    await page.getByRole('button', { name: /^(File report|Save changes)$/ }).click();

    await expect(page.getByText('Report filed', { exact: true })).toBeVisible();
    await expect(page.locator('dl')).toContainText(work);
  });
});
