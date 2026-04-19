/**
 * Login page tests — ref mockup: 08-login-mobile.png
 * Viewport: 390×844 (iPhone 14 / mobile project)
 *
 * Visual snapshots are "golden baselines".
 * First run against a live server creates them; subsequent runs compare.
 */

import { test, expect } from '@playwright/test';
import { assertColorApprox } from './helpers';

// Run all login tests under the mobile viewport via project filter.
// These tests target the mobile Playwright project (390×844).

test.describe('Login page', () => {
  test('login page renders correctly', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    // Body background approx #0B0E0F
    const bgColor = await page.evaluate(() =>
      getComputedStyle(document.body).backgroundColor
    );
    assertColorApprox(bgColor, '#0B0E0F', 15, 'body bg');

    // Brand text
    await expect(page.getByText('ComandCenter')).toBeVisible();
    await expect(page.getByText('AI agent control panel')).toBeVisible();

    // Password label (case-insensitive)
    await expect(page.getByText(/access password/i)).toBeVisible();

    // Form controls
    await expect(page.locator('input[type="password"]')).toBeVisible();

    const connectBtn = page.locator('button[type="submit"]');
    await expect(connectBtn).toBeVisible();
    await expect(connectBtn).toHaveText(/connect/i);

    // Connect button bg approx brand green #00C48C
    const btnBg = await connectBtn.evaluate((el) =>
      getComputedStyle(el).backgroundColor
    );
    assertColorApprox(btnBg, '#00C48C', 20, 'connect-btn bg');

    // Footer path hint
    await expect(page.getByText('~/.claudio')).toBeVisible();

    // Visual snapshot — first run creates baseline, subsequent runs compare
    await expect(page).toHaveScreenshot('login-mobile.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('password show/hide toggle works', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    const input = page.locator('input[type="password"]');
    // Initially masked
    await expect(input).toHaveAttribute('type', 'password');

    // Click the eye/toggle button (look for a button near the password input)
    const toggleBtn = page.locator(
      'button[aria-label*="show" i], button[aria-label*="toggle" i], ' +
      'button[aria-label*="password" i], [data-testid="pw-toggle"], ' +
      'input[type="password"] ~ button, input[type="password"] + button, ' +
      '.relative button:not([type="submit"])'
    ).first();

    await toggleBtn.click();
    // Input should now be type=text
    await expect(page.locator('input[type="text"]')).toBeVisible();

    // Click again → back to password
    await toggleBtn.click();
    await expect(page.locator('input[type="password"]')).toBeVisible();
  });

  test('wrong password shows error', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    await page.fill('input[type="password"]', 'definitely-wrong-pw-xyz-9999');
    await page.click('button[type="submit"]');

    // Either stays on /login or shows error message
    await page.waitForLoadState('networkidle');
    const url = page.url();
    const staysOnLogin = url.includes('/login');
    const hasError = await page
      .locator('[role="alert"], .error, .text-error, [class*="error"]')
      .isVisible()
      .catch(() => false);

    expect(staysOnLogin || hasError).toBeTruthy();
  });

  test('correct password redirects to session list', async ({ page }) => {
    await page.goto('/login');
    await page.waitForLoadState('networkidle');

    const password = process.env.CC_PASSWORD || 'test';
    await page.fill('input[type="password"]', password);
    await page.click('button[type="submit"]');

    await page.waitForURL('/', { timeout: 5000 });
    expect(page.url()).toMatch(/\/$/);
  });
});
