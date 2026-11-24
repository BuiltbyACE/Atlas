import { test, expect } from '@playwright/test';
import { signIn } from './helpers/auth';
import { navLink } from './helpers/nav';
import { todayIso, unique } from './helpers/data';

test.describe('Finance Module', () => {
  test('view finance dashboard', async ({ page }) => {
    await signIn(page);
    await navLink(page, '/finance').click();
    await expect(page).toHaveURL(/\/finance$/);
    await expect(page.getByRole('heading', { name: 'Finance', exact: true })).toBeVisible();
  });

  test('create an expense', async ({ page }) => {
    // The configured receipt threshold is 100000 minor units. Keep the fixture
    // below it so this case exercises the ordinary claim path, not receipt
    // upload; a unique amount makes the persisted row unambiguous.
    const amountMinor = 1000 + (Date.now() % 90000);
    const vendor = unique('E2E vendor');

    await signIn(page);
    await navLink(page, '/finance/my-expenses').click();
    await expect(page).toHaveURL(/\/finance\/my-expenses$/);

    await page.getByRole('button', { name: 'New expense' }).click();
    await page.locator('#expense-category').selectOption('operational');
    await page.locator('#expense-amount').fill(String(amountMinor));
    await page.locator('#expense-date').fill(todayIso());
    await page.locator('#expense-vendor').fill(vendor);

    const created = page.waitForResponse(
      (response) =>
        response.url().includes('/api/v1/finance/expenses') &&
        response.request().method() === 'POST',
    );
    await page.getByRole('button', { name: 'Create expense' }).click();
    const response = await created;
    expect(response.status()).toBe(201);
    const ref = (await response.json()).ref as string;
    expect(ref).toMatch(/^EXP-/);

    // The claim list is loaded in a client effect, so reload to observe the
    // persisted row rather than relying on optimistic local state. The test
    // identity has no finance.amount.sensitive.read, so the amount must be
    // masked in the UI; the reference is the stable readback handle.
    await page.reload();
    const row = page.getByRole('row').filter({ hasText: ref });
    await expect(row).toBeVisible();
    await expect(row).toContainText('Restricted');
  });
});
