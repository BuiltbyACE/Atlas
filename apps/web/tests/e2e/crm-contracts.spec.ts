import { test, expect } from '@playwright/test';
import { signIn } from './helpers/auth';

test.describe('CRM → Contracts Lifecycle', () => {
  test('create a lead', async ({ page }) => {
    await signIn(page);

    // Navigate to CRM
    await page.click('a:has-text("CRM")');
    await expect(page).toHaveURL('/crm');

    // Click "New lead"
    await page.click('a:has-text("New lead")');

    // Fill in lead form
    await page.fill('input[name="source_key"]', 'website_form');
    await page.fill('input[name="contact_name"]', 'E2E Test Contact');
    await page.fill('input[name="need_summary"]', 'E2E test lead need');
    await page.fill('input[name="next_action"]', 'Follow up call');
    await page.fill('input[name="next_action_due_at"]', '2026-12-15');

    // Submit
    await page.click('button[type="submit"]');

    // Verify lead was created - should redirect back to CRM with the new lead visible
    await expect(page).toHaveURL('/crm');
  });

  test('create a contract', async ({ page }) => {
    await signIn(page);

    // Navigate to contracts
    await page.click('a:has-text("Contracts")');
    await expect(page).toHaveURL('/contracts');

    // Click "New contract"
    await page.click('a:has-text("New contract")');

    // Fill in contract form
    await page.fill('input[name="organization_id"]', '');
    await page.fill('input[name="contract_type"]', 'fixed_price_project');
    await page.fill('input[name="value_minor"]', '100000');
    await page.fill('input[name="currency"]', 'KES');

    // Submit
    await page.click('button[type="submit"]');

    // Verify contract was created
    await expect(page).toHaveURL('/contracts');
  });
});
