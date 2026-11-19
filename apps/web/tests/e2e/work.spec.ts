import { test, expect, type Page } from '@playwright/test';
import { signIn } from './helpers/auth';
import { navLink } from './helpers/nav';
import { todayIso, unique } from './helpers/data';

/**
 * Work items always belong to a project in the current UI, so every case builds
 * its own project before adding a task. Titles are unique per run, which keeps
 * retries and repeat runs against the same disposable database unambiguous.
 */
async function createProjectAndTask(
  page: Page,
  projectName: string,
  taskTitle: string,
) {
  await navLink(page, '/work').click();
  await expect(page).toHaveURL(/\/work$/);

  // The page header and the empty-state card both offer "New project".
  await page.getByRole('button', { name: 'New project' }).first().click();
  await page.locator('#project-name').fill(projectName);
  await page.getByRole('button', { name: 'Create project' }).click();
  await expect(page).toHaveURL(/\/work\/projects\/[^/]+$/);
  await expect(page.locator('h1')).toHaveText(projectName);

  await page.getByRole('button', { name: 'New task' }).click();
  await page.locator('#task-title').fill(taskTitle);
  await page.locator('#task-description').fill('Created by the Playwright suite');
  await page.locator('#task-due').fill(todayIso());
  await page.getByRole('button', { name: 'Create task' }).click();

  const taskLink = page.getByRole('link', { name: taskTitle, exact: true });
  await expect(taskLink).toBeVisible();
  return taskLink;
}

test.describe('Work Module', () => {
  test('create a task and verify it appears in the task board', async ({ page }) => {
    const projectName = unique('E2E project');
    const taskTitle = unique('E2E task');

    await signIn(page);
    await createProjectAndTask(page, projectName, taskTitle);

    await page.goto('/work/tasks');
    await expect(page.locator('section[data-column="todo"]')).toContainText(taskTitle);
  });

  test('view task details', async ({ page }) => {
    const projectName = unique('E2E detail project');
    const taskTitle = unique('E2E detail task');

    await signIn(page);
    const taskLink = await createProjectAndTask(page, projectName, taskTitle);

    await taskLink.click();
    await expect(page).toHaveURL(/\/work\/tasks\/[^/]+$/);
    await expect(page.locator('h1')).toHaveText(taskTitle);
  });
});
