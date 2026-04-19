package tools

// fidelityScript is the embedded Node.js script that uses Playwright to capture
// a live screenshot of a running web app. Written to a temp file and executed
// via exec.Command("node", ...) by ReviewDesignFidelityTool.
const fidelityScript = `
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

    const url        = flag('--url', '');
    const outDir     = flag('--out-dir', '');
    const viewportW  = parseInt(flag('--viewport-w', '1440'), 10);
    const viewportH  = parseInt(flag('--viewport-h', '900'), 10);
    const scale      = parseFloat(flag('--scale', '2'));

    if (!url || !outDir) {
        console.log(JSON.stringify({
            success: false,
            errors: ['--url and --out-dir are required'],
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

    await page.goto(url, { waitUntil: 'networkidle' });

    // Brief settle for any post-load animations.
    await page.waitForTimeout(1000);
    await page.evaluate(() => document.fonts.ready);

    const outFile = path.join(outDir, 'live.png');
    await page.screenshot({ path: outFile, fullPage: true });

    await browser.close();

    console.log(JSON.stringify({
        success: true,
        errors,
        warnings,
        screenshots: [{ name: 'live', path: outFile }],
    }));
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
