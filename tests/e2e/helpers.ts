import { Page } from '@playwright/test';

/**
 * Login helper — navigates to /login, submits password, waits for redirect to /.
 * Password order: CC_PASSWORD env var → explicit arg → 'test' fallback.
 */
export async function login(page: Page, password = process.env.CC_PASSWORD || 'test') {
  await page.goto('/login');
  await page.fill('input[type="password"]', password);
  await page.click('button[type="submit"]');
  await page.waitForURL('/');
  await page.waitForLoadState('networkidle');
}

// ---------------------------------------------------------------------------
// Color helpers
// ---------------------------------------------------------------------------

/** Parse "#RRGGBB" → { r, g, b } */
export function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const clean = hex.replace('#', '');
  return {
    r: parseInt(clean.slice(0, 2), 16),
    g: parseInt(clean.slice(2, 4), 16),
    b: parseInt(clean.slice(4, 6), 16),
  };
}

/**
 * Parse "rgb(r, g, b)" or "rgba(r, g, b, a)" returned by getComputedStyle
 * into { r, g, b }.
 */
export function parseRgb(computed: string): { r: number; g: number; b: number } {
  const m = computed.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/);
  if (!m) throw new Error(`Cannot parse color: ${computed}`);
  return { r: +m[1], g: +m[2], b: +m[3] };
}

/**
 * Assert computed color is within ±tolerance per channel of expected hex.
 * Returns true if match, throws descriptive error otherwise.
 */
export function assertColorApprox(
  computed: string,
  expectedHex: string,
  tolerance = 15,
  label = ''
) {
  const actual = parseRgb(computed);
  const expected = hexToRgb(expectedHex);
  const label_ = label ? `[${label}] ` : '';
  for (const ch of ['r', 'g', 'b'] as const) {
    const diff = Math.abs(actual[ch] - expected[ch]);
    if (diff > tolerance) {
      throw new Error(
        `${label_}Color mismatch on channel ${ch}: ` +
          `computed=${computed} (${actual[ch]}), expected=${expectedHex} (${expected[ch]}), diff=${diff} > ${tolerance}`
      );
    }
  }
}
