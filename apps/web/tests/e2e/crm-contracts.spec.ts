import { test, expect, type Page } from '@playwright/test';
import { signIn } from './helpers/auth';
import { unique } from './helpers/data';

/** Assert the real write succeeded, not just that the submit button moved. */
async function expectCreated(page: Page, url: string, action: () => Promise<void>) {
  const response = page.waitForResponse(
    (candidate) =>
      candidate.url().includes(url) && candidate.request().method() === 'POST',
  );
  await action();
  expect((await response).status()).toBe(201);
}

/** Organizations are the parent record for both leads and contracts. */
async function createOrganization(page: Page, name: string) {
  await page.goto('/crm/organizations/new');
  await page.locator('#org-name').fill(name);
  await expectCreated(page, '/api/v1/crm/organizations', async () => {
    await page.getByRole('button', { name: 'Create organization' }).click();
  });
  await expect(page.getByRole('button', { name: 'Create organization' })).toBeHidden();

  await page.goto('/crm/organizations');
  await expect(page.getByRole('link', { name, exact: true })).toBeVisible();
}

test.describe('CRM → Contracts Lifecycle', () => {
  test('create a lead', async ({ page }) => {
    const orgName = unique('E2E lead org');
    const need = unique('E2E lead need');

    await signIn(page);
    await createOrganization(page, orgName);

    await page.goto('/crm');
    await expect(page.getByRole('heading', { name: 'CRM', exact: true })).toBeVisible();
    await page.getByRole('link', { name: 'New lead' }).click();
    await page.locator('#lead-source').selectOption('website_form');
    await page.locator('#lead-need').fill(need);
    await page.locator('#lead-contact').fill(unique('E2E contact'));
    await page.locator('#lead-org').selectOption({ label: orgName });
    await expectCreated(page, '/api/v1/crm/leads', async () => {
      await page.getByRole('button', { name: 'Create lead' }).click();
    });
    await expect(page.getByRole('button', { name: 'Create lead' })).toBeHidden();

    await page.goto('/crm');
    await expect(page.getByRole('row').filter({ hasText: need })).toBeVisible();
  });

  test('create a contract', async ({ page }) => {
    const orgName = unique('E2E contract org');

    await signIn(page);
    await createOrganization(page, orgName);

    await page.goto('/contracts');
    await expect(page.getByRole('heading', { name: 'Contracts', exact: true })).toBeVisible();
    await page.getByRole('link', { name: 'New contract' }).click();
    await page.locator('#contract-org').selectOption({ label: orgName });
    await page.locator('#contract-value').fill('100000');
    await expectCreated(page, '/api/v1/contracts', async () => {
      await page.getByRole('button', { name: 'Create contract' }).click();
    });
    await expect(page.getByRole('button', { name: 'Create contract' })).toBeHidden();

    await page.goto('/contracts');
    await expect(page.getByRole('row').filter({ hasText: orgName })).toBeVisible();
  });
});
