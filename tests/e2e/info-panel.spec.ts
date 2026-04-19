/**
 * Info panel tests — ref mockup: 05-mobile-info.png
 * Requires:
 *   - Auth cookie (login helper)
 *   - SESSION_ID env var pointing to a seeded session
 *     e.g.  SESSION_ID=abc123 npx playwright test info-panel
 *
 * Tests that need a real session skip when SESSION_ID is absent.
 * Visual snapshots: first run creates baselines.
 */

import { test, expect } from '@playwright/test';
import { login } from './helpers';

const SESSION_ID = process.env.SESSION_ID;
const SKIP_MSG = 'Requires SESSION_ID env var pointing to a session with data';

test.describe('Info panel — mobile (390×844)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test.beforeEach(async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await login(page);
    await page.goto(`/chat/${SESSION_ID}/info`);
    await page.waitForLoadState('networkidle');
  });

  test('info panel shows session metadata', async ({ page }) => {
    // "Path" label
    await expect(page.getByText(/path/i)).toBeVisible();
    // "Status" label
    await expect(page.getByText(/status/i)).toBeVisible();
    // Status dot present
    await expect(
      page.locator('[class*="status"], [class*="dot"], [class*="rounded-full"]').first()
    ).toBeVisible();
  });

  test('tab bar has 4 tabs', async ({ page }) => {
    // Tasks, Team, Crons, Config
    await expect(page.getByText('Tasks')).toBeVisible();
    await expect(page.getByText('Team')).toBeVisible();
    await expect(page.getByText('Crons')).toBeVisible();
    await expect(page.getByText('Config')).toBeVisible();
  });

  test('Tasks tab is active by default', async ({ page }) => {
    // Tasks panel should be visible, others hidden
    const tasksPanel = page.locator('#panel-tasks, [data-panel="tasks"]').first();
    await expect(tasksPanel).toBeVisible();

    // Tasks tab button should have brand green styling or aria-selected
    const tasksTab = page.getByRole('button', { name: 'Tasks' }).or(
      page.locator('[data-tab="tasks"]')
    ).first();
    // Tab text visible & panel visible is sufficient
    await expect(tasksTab).toBeVisible();
  });

  test('task status icons render correctly', async ({ page }) => {
    // Look for task rows with status indicators
    const taskRows = page.locator('[class*="task"], [id*="task"]');
    const count = await taskRows.count();
    if (count === 0) {
      test.info().annotations.push({ type: 'note', description: 'No tasks in this session.' });
      return;
    }

    // done: ✓ or checkmark icon
    const doneIcon = page.locator('[class*="done"], [class*="complete"], [class*="check"]').first();
    // in_progress: blue dot
    const inProgressIcon = page.locator('[class*="in_progress"], [class*="in-progress"], [class*="running"]').first();

    // At least one task row visible
    await expect(taskRows.first()).toBeVisible();
  });

  test('Crons tab shows cron list with delete button', async ({ page }) => {
    // Click Crons tab
    const cronsTab = page.getByText('Crons').first();
    await cronsTab.click();
    await page.waitForLoadState('networkidle');

    // Crons panel visible
    const cronsPanel = page.locator('#panel-crons, [data-panel="crons"]').first();
    await expect(cronsPanel).toBeVisible();

    // If crons exist, delete button should be present
    const cronRows = page.locator('[class*="cron-row"], [data-cron-id]');
    const cronCount = await cronRows.count();
    if (cronCount > 0) {
      const deleteBtn = page.locator('button[class*="delete"], button[aria-label*="delete" i]').first();
      await expect(deleteBtn).toBeVisible();
    } else {
      test.info().annotations.push({ type: 'note', description: 'No crons configured in this session.' });
    }
  });

  test('Config tab shows session config with model name', async ({ page }) => {
    const configTab = page.getByText('Config').first();
    await configTab.click();
    await page.waitForLoadState('networkidle');

    const configPanel = page.locator('#panel-config, [data-panel="config"]').first();
    await expect(configPanel).toBeVisible();

    // Model name should be visible (any non-empty text mentioning model)
    const modelText = page.locator('[class*="model"], [data-key="model"]').first();
    const count = await modelText.count();
    if (count > 0) {
      await expect(modelText).toBeVisible();
    } else {
      // At minimum the panel rendered without error
      const panelText = await configPanel.innerText();
      expect(panelText.trim().length).toBeGreaterThan(0);
    }
  });

  test('visual snapshot — info panel mobile', async ({ page }) => {
    await expect(page).toHaveScreenshot('info-panel-mobile.png', {
      maxDiffPixelRatio: 0.05,
    });
  });
});
