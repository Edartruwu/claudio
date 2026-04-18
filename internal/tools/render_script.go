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
    await page.goto(fileUrl, { waitUntil: 'networkidle' });

    // Wait for render + fonts.
    await page.waitForTimeout(2000);
    await page.evaluate(() => document.fonts.ready);

    const screenshots = [];

    // Full-page screenshot.
    const fullPath = path.join(outDir, 'full-canvas.png');
    await page.screenshot({ path: fullPath, fullPage: true });
    screenshots.push({ name: 'full-canvas', path: fullPath });

    // Per-artboard screenshots.
    if (capture) {
        const artboards = await page.$$('[data-artboard]');
        for (const el of artboards) {
            const name = await el.getAttribute('data-artboard');
            if (!name) continue;
            const p = path.join(outDir, name + '.png');
            await el.screenshot({ path: p });
            screenshots.push({ name, path: p });
        }
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
