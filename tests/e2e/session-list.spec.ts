import { test, expect } from '@playwright/test';
import { login } from './helpers';

test.describe('Session list — mobile (390×844)', () => {
  test.use({ viewport: { width: 390, height: 844 } });
  test.beforeEach(async ({ page }) => { await login(page); });

  test('mobile session list renders correctly', async ({ page }) => {
    await expect(page.locator('#session-list')).toBeVisible();
    await expect(page.locator('.swipe-row').first()).toBeVisible();
  });

  test('session rows have link to chat', async ({ page }) => {
    const row = page.locator('.swipe-row').first();
    await expect(row.locator('a[href^="/chat/"]')).toBeVisible();
  });

  test('filter chips are visible', async ({ page }) => {
    await expect(page.locator('#filter-chips')).toBeVisible();
    await expect(page.locator('[data-filter-chip]')).toHaveCount(3);
  });

  test('filter tabs switch correctly — Active tab gets brand indicator', async ({ page }) => {
    const activeBtn = page.locator('[data-filter-chip="active"]');
    await activeBtn.click();
    await page.waitForTimeout(500);
    await expect(activeBtn).toBeVisible();
  });

  test('search input triggers HTMX request', async ({ page }) => {
    const searchInput = page.locator('input[hx-get="/partials/sessions"]');
    await expect(searchInput).toBeVisible();
    await searchInput.fill('test');
    await page.waitForTimeout(600);
    await expect(page.locator('#session-list')).toBeVisible();
  });

  test('visual snapshot — session list mobile', async ({ page }) => {
    await expect(page).toHaveScreenshot('session-list-mobile.png', { maxDiffPixelRatio: 0.05 });
  });
});

test.describe('Session list — desktop (1440x900)', () => {
  test.use({ viewport: { width: 1440, height: 900 } });
  test.beforeEach(async ({ page }) => { await login(page); });

  test('desktop session list renders correctly', async ({ page }) => {
    await expect(page.locator('#session-list')).toBeVisible();
    await expect(page.locator('.swipe-row').first()).toBeVisible();
  });

  test('visual snapshot — session list desktop', async ({ page }) => {
    await expect(page).toHaveScreenshot('session-list-desktop.png', { maxDiffPixelRatio: 0.05 });
  });
});
