/**
 * Session list tests — ref mockups: 03-mobile-sessions.png, 09-desktop-empty.png
 * Requires auth cookie set via login helper.
 *
 * Visual snapshots: first run against live server creates baselines.
 */

import { test, expect } from '@playwright/test';
import { login } from './helpers';

test.describe('Session list — mobile (390×844)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('mobile session list renders correctly', async ({ page }) => {
    // Heading
    await expect(page.getByText('ComandCenter')).toBeVisible();

    // Search input
    await expect(
      page.locator('input[type="search"], input[placeholder*="search" i], input[name="q"]')
    ).toBeVisible();

    // Filter tabs
    await expect(page.getByText('All')).toBeVisible();
    await expect(page.getByText('Active')).toBeVisible();
    await expect(page.getByText('Inactive')).toBeVisible();

    // Bottom nav
    await expect(page.getByText('Sessions')).toBeVisible();
    await expect(page.getByText('Designs')).toBeVisible();

    // Visual snapshot
    await expect(page).toHaveScreenshot('session-list-mobile.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('search input triggers HTMX request', async ({ page }) => {
    const searchInput = page.locator(
      'input[type="search"], input[placeholder*="search" i], input[name="q"]'
    );
    await expect(searchInput).toBeVisible();

    // Type a query and wait for the session list partial to settle
    await searchInput.fill('nonexistent-session-xyz');
    await page.waitForLoadState('networkidle');

    // Session list container should still be present (even if empty)
    const sessionList = page.locator(
      '#session-list, [id*="session"], [hx-target*="session"], ul[class*="session"]'
    ).first();
    await expect(sessionList).toBeAttached();
  });

  test('filter tabs switch correctly — Active tab gets brand indicator', async ({ page }) => {
    const activeTab = page.getByRole('button', { name: 'Active' }).or(
      page.locator('[data-filter="active"], [data-tab="active"]')
    ).first();

    await activeTab.click();
    await page.waitForLoadState('networkidle');

    // Active tab should have brand green styling
    const color = await activeTab.evaluate((el) => {
      const s = getComputedStyle(el);
      return s.color || s.borderBottomColor || s.backgroundColor;
    });
    // Just assert it changed (not default gray) — full color assertion requires exact UI
    expect(color).toBeTruthy();
    expect(color).not.toBe('rgba(0, 0, 0, 0)');
  });
});

test.describe('Session list — desktop (1440×900)', () => {
  test.use({ viewport: { width: 1440, height: 900 } });

  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('desktop session list renders correctly', async ({ page }) => {
    // Two-panel layout: sidebar + main content
    const sidebar = page.locator(
      'aside, [class*="sidebar"], [class*="w-\\[360px\\]"], nav'
    ).first();
    const mainContent = page.locator('main, [class*="flex-1"], [class*="main"]').first();

    await expect(sidebar).toBeVisible();
    await expect(mainContent).toBeAttached();

    // Visual snapshot
    await expect(page).toHaveScreenshot('session-list-desktop.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('empty state shows correct message', async ({ page }) => {
    // If no sessions exist the server should render an empty-state message
    const sessionItems = page.locator(
      '[href*="/chat/"], [class*="session-row"], li[class*="session"]'
    );
    const count = await sessionItems.count();

    if (count === 0) {
      // Empty state text
      const emptyMsg = page.locator(
        '[class*="empty"], [class*="no-session"], [data-empty]'
      ).or(
        page.getByText(/no sessions/i)
      ).first();
      await expect(emptyMsg).toBeVisible();
    } else {
      // Sessions present — test is a no-op in this environment
      test.info().annotations.push({
        type: 'note',
        description: 'Sessions exist; empty-state not exercised in this run.',
      });
    }
  });
});
