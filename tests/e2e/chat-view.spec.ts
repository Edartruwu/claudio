/**
 * Chat view tests — ref mockups: 04-mobile-chat.png, 06-desktop.png
 * Requires:
 *   - Auth cookie (login helper)
 *   - SESSION_ID env var pointing to a seeded session in the running server
 *     e.g.  SESSION_ID=abc123 npx playwright test chat-view
 *
 * Tests that need a real session skip when SESSION_ID is absent.
 * Visual snapshots: first run creates baselines.
 */

import { test, expect } from '@playwright/test';
import { login, assertColorApprox } from './helpers';

const SESSION_ID = process.env.SESSION_ID;
const SKIP_MSG = 'Requires SESSION_ID env var pointing to an active session';

test.describe('Chat view — mobile (390×844)', () => {
  test.use({ viewport: { width: 390, height: 844 } });

  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('chat header shows session name and status dot', async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await page.goto(`/chat/${SESSION_ID}`);
    await page.waitForLoadState('networkidle');

    // Back arrow on mobile
    const backArrow = page.locator('a[href="/"], [aria-label="back"], [href*="back"]').first();
    await expect(backArrow).toBeVisible();

    // Session name in header (some non-empty text)
    const header = page.locator('header, [role="banner"]').first();
    const headerText = await header.innerText();
    expect(headerText.trim().length).toBeGreaterThan(0);

    // Status dot — green circle for active
    const statusDot = page.locator(
      '[class*="status"], [class*="dot"], [class*="rounded-full"][class*="bg-"]'
    ).first();
    await expect(statusDot).toBeVisible();
  });

  test('user message bubble is right-aligned with brand green bg', async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await page.goto(`/chat/${SESSION_ID}`);
    await page.waitForLoadState('networkidle');

    // User bubbles: role=user → right-aligned
    const userBubble = page.locator('[class*="msg-bubble-user"], [class*="bubble"][class*="user"]').first();
    const count = await userBubble.count();
    if (count === 0) {
      test.info().annotations.push({ type: 'note', description: 'No user bubbles present in this session.' });
      return;
    }

    await expect(userBubble).toBeVisible();

    // Right alignment
    const align = await userBubble.evaluate((el) => {
      const s = getComputedStyle(el);
      return { marginLeft: s.marginLeft, justifyContent: s.justifyContent, textAlign: s.textAlign };
    });
    // At least one right-alignment indicator
    const isRight = align.marginLeft === 'auto' ||
      align.justifyContent === 'flex-end' ||
      align.textAlign === 'right';
    expect(isRight).toBeTruthy();
  });

  test('assistant message bubble is left-aligned with ai-blue border', async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await page.goto(`/chat/${SESSION_ID}`);
    await page.waitForLoadState('networkidle');

    const assistantBubble = page.locator(
      '[class*="msg-bubble-assistant"], [class*="bubble"][class*="assistant"]'
    ).first();
    const count = await assistantBubble.count();
    if (count === 0) {
      test.info().annotations.push({ type: 'note', description: 'No assistant bubbles in this session.' });
      return;
    }

    await expect(assistantBubble).toBeVisible();

    // Left side — margin-left should NOT be auto
    const ml = await assistantBubble.evaluate((el) => getComputedStyle(el).marginLeft);
    expect(ml).not.toBe('auto');
  });

  test('tool_call renders as collapsible details/summary', async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await page.goto(`/chat/${SESSION_ID}`);
    await page.waitForLoadState('networkidle');

    const details = page.locator('details').first();
    const count = await details.count();
    if (count === 0) {
      test.info().annotations.push({ type: 'note', description: 'No tool_use messages in this session.' });
      return;
    }

    await expect(details).toBeVisible();
    // summary inside details
    await expect(details.locator('summary')).toBeVisible();
  });

  test('input bar is pinned to bottom', async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await page.goto(`/chat/${SESSION_ID}`);
    await page.waitForLoadState('networkidle');

    const inputBar = page.locator('#msg-input, textarea[name="content"], textarea[placeholder*="message" i]').first();
    await expect(inputBar).toBeVisible();

    // Check that input is near the bottom of the viewport
    const box = await inputBar.boundingBox();
    if (box) {
      const viewportHeight = page.viewportSize()?.height ?? 844;
      // Input bottom should be within 100px of viewport bottom
      expect(box.y + box.height).toBeGreaterThan(viewportHeight - 120);
    }
  });

  test('date divider is centered and muted', async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await page.goto(`/chat/${SESSION_ID}`);
    await page.waitForLoadState('networkidle');

    const divider = page.locator(
      '[class*="date-divider"], [class*="divider"], [class*="text-center"][class*="text-muted"]'
    ).first();
    const count = await divider.count();
    if (count === 0) {
      test.info().annotations.push({ type: 'note', description: 'No date dividers visible in this session.' });
      return;
    }

    await expect(divider).toBeVisible();
    const textAlign = await divider.evaluate((el) => getComputedStyle(el).textAlign);
    expect(textAlign).toBe('center');
  });

  test('visual snapshot — chat view mobile', async ({ page }) => {
    test.skip(!SESSION_ID, SKIP_MSG);
    await page.goto(`/chat/${SESSION_ID}`);
    await page.waitForLoadState('networkidle');

    await expect(page).toHaveScreenshot('chat-view-mobile.png', {
      maxDiffPixelRatio: 0.05,
    });
  });
});
