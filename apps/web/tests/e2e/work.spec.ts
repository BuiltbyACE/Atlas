import { test, expect } from '@playwright/test';
import { signIn } from './helpers/auth';

test.describe('Work Module', () => {
  test('create a task and verify it appears in the task board', async ({ page }) => {
    await signIn(page);

    // Navigate to task board
    await page.click('a:has-text("Task board")');
    await expect(page).toHaveURL('/work/tasks');

    // Click "New task" button
    await page.click('a:has-text("New task")');
    await expect(page.url()).toContain('/work/tasks');

    // Fill in task form
    await page.fill('input[name="title"]', 'E2E test task');
    await page.fill('textarea[name="description"]', 'Created via E2E test');
    await page.fill('input[name="due_at"]', '2026-12-01');

    // Submit
    await page.click('button[type="submit"]');

    // Verify task appears in the list
    await expect(page.locator('text=E2E test task')).toBeVisible();
  });

  test('view task details', async ({ page }) => {
    await signIn(page);
    await page.goto('/work/tasks');

    // If there's a task, click on it to view details
    const taskLink = page.locator('a:has-text("E2E test task")');
    if (await taskLink.isVisible()) {
      await taskLink.click();
      await expect(page).toHaveURL(/\/work\/tasks\//);
      await expect(page.locator('h1')).toContainText('E2E test task');
    }
  });
});
