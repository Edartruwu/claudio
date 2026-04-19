package tools

// renderScript is the embedded Node.js script that uses Playwright to render
// an HTML file and capture screenshots. Written to a temp file and executed
// via exec.Command("node", ...) by RenderMockupTool.
const renderScript = `
const path = require('path');
const fs = require('fs');

// Locate playwright — support both 'playwright' and '@playwright/test'.
let chromium;
try {
    chromium = require('playwright').chromium;
} catch (_) {
    try {
        chromium = require('@playwright/test').chromium;
    } catch (e) {
        console.log(JSON.stringify({
            success: false,
            errors: ['Cannot find playwright or @playwright/test: ' + e.message],
            warnings: [],
            screenshots: [],
        }));
        process.exit(1);
    }
}

async function main() {
    const argv = process.argv.slice(2);

    function flag(name, def) {
        const i = argv.indexOf(name);
        if (i === -1 || i + 1 >= argv.length) return def;
        return argv[i + 1];
    }

    const htmlPath      = flag('--html', '');
    const outDir        = flag('--out-dir', '');
    const viewportW     = parseInt(flag('--viewport-w', '1440'), 10);
    const viewportH     = parseInt(flag('--viewport-h', '900'), 10);
    const scale         = parseFloat(flag('--scale', '2'));
    const capture       = flag('--capture-screens', 'true') === 'true';

    if (!htmlPath || !outDir) {
        console.log(JSON.stringify({
            success: false,
            errors: ['--html and --out-dir are required'],
            warnings: [],
            screenshots: [],
        }));
        process.exit(1);
    }

    fs.mkdirSync(outDir, { recursive: true });

    const browser = await chromium.launch({ headless: true });
    const context = await browser.newContext({
        viewport: { width: viewportW, height: viewportH },
        deviceScaleFactor: scale,
    });
    const page = await context.newPage();

    const errors = [];
    const warnings = [];
    page.on('console', msg => {
        const t = msg.type();
        if (t === 'error')   errors.push(msg.text());
        if (t === 'warning') warnings.push(msg.text());
    });
    page.on('pageerror', err => {
        errors.push(err.message);
    });

    const fileUrl = 'file://' + path.resolve(htmlPath);
    // Use domcontentloaded — networkidle fires instantly on file:// (no network)
    // and Babel Standalone hasn't parsed JSX yet at that point.
    await page.goto(fileUrl, { waitUntil: 'domcontentloaded' });

    // Wait for Babel/React to finish mounting. Strategy:
    //   1. Wait up to 15s for at least one [data-artboard] element.
    //   2. After first artboard appears, give React another 1s to render siblings.
    //   3. Wait for fonts.
    let artboardsFound = false;
    if (capture) {
        try {
            await page.waitForSelector('[data-artboard]', { timeout: 15000 });
            artboardsFound = true;
            // Brief settle — siblings may still be mounting.
            await page.waitForTimeout(1000);
        } catch (_) {
            warnings.push('No [data-artboard] elements found after 15s. ' +
                'Check that each screen root div has a data-artboard="<name>" attribute.');
        }
    } else {
        // No artboard capture needed — just let React settle.
        await page.waitForTimeout(3000);
    }
    await page.evaluate(() => document.fonts.ready);

    const screenshots = [];

    // Per-artboard screenshots — these are the primary deliverable.
    // Full-canvas is only taken as fallback when no artboards exist.
    if (capture && artboardsFound) {
        const artboards = await page.$$('[data-artboard]');
        const renderedDir = path.join(outDir, 'rendered');
        await fs.mkdir(renderedDir, { recursive: true });
        for (const el of artboards) {
            const name = await el.getAttribute('data-artboard');
            if (!name) continue;
            const p = path.join(outDir, name + '.png');
            await el.screenshot({ path: p });
            screenshots.push({ name, path: p });

            // Extract plain rendered HTML (framework-agnostic)
            const html = await el.evaluate(el => el.outerHTML);
            const renderedPath = path.join(renderedDir, name + '.html');
            await fs.writeFile(renderedPath, html, 'utf8');

            // Extract interactive elements
            const interactives = await el.$$eval(
                'button, a[href], input, select, textarea, [role="button"]',
                els => els.map(el => ({
                    tag: el.tagName.toLowerCase(),
                    text: el.textContent.trim().slice(0, 80),
                    id: el.id || '',
                    className: el.className || '',
                    type: el.getAttribute('type') || '',
                    href: el.getAttribute('href') || ''
                }))
            );
            const interactionsPath = path.join(renderedDir, name + '.interactions.json');
            await fs.writeFile(interactionsPath, JSON.stringify(interactives, null, 2), 'utf8');
        }
    } else {
        // Fallback: full-page screenshot clipped to max 4000px to avoid
        // vision API limits (Claude max: 8000px per dimension).
        const fullPath = path.join(outDir, 'full-canvas.png');
        const bodyHeight = await page.evaluate(() => document.body.scrollHeight);
        const clipH = Math.min(bodyHeight, 4000);
        await page.screenshot({
            path: fullPath,
            clip: { x: 0, y: 0, width: viewportW, height: clipH },
        });
        screenshots.push({ name: 'full-canvas', path: fullPath });
    }

    await browser.close();

    console.log(JSON.stringify({ success: true, errors, warnings, screenshots }));
}

main().catch(e => {
    console.log(JSON.stringify({
        success: false,
        errors: [e.message],
        warnings: [],
        screenshots: [],
    }));
    process.exit(1);
});
`
