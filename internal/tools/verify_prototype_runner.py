#!/usr/bin/env python3
"""
verify_prototype_runner.py — Thin Playwright wrapper for click-testing HTML prototypes.

Reads JSON on stdin, runs Playwright interactions, writes JSON result to stdout.

Input JSON:
{
    "html_path": "/path/to/file.html",
    "test_scenario": "optional natural language (currently unused, reserved for AI-driven testing)",
    "click_sequence": ["#btn1", ".submit"],
    "screenshot_on_each_step": false,
    "timeout_ms": 30000
}

Output JSON:
{
    "passed": true,
    "steps_completed": 2,
    "steps_total": 2,
    "console_errors": [],
    "screenshots": ["/path/to/screenshot.png"],
    "failure_reason": "",
    "duration_ms": 1200
}
"""

import json
import sys
import time
from pathlib import Path


def run(config):
    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        return {
            "passed": False,
            "steps_completed": 0,
            "steps_total": 0,
            "console_errors": [],
            "screenshots": [],
            "failure_reason": "Playwright not installed. Run: pip install playwright && playwright install chromium",
            "duration_ms": 0,
        }

    html_path = Path(config["html_path"]).resolve()
    if not html_path.exists():
        return {
            "passed": False,
            "steps_completed": 0,
            "steps_total": 0,
            "console_errors": [],
            "screenshots": [],
            "failure_reason": f"File not found: {html_path}",
            "duration_ms": 0,
        }

    click_sequence = config.get("click_sequence", [])
    screenshot_each = config.get("screenshot_on_each_step", False)
    timeout_ms = config.get("timeout_ms", 30000)

    steps_total = len(click_sequence)
    steps_completed = 0
    console_errors = []
    screenshots = []
    failure_reason = ""
    start = time.monotonic()

    screenshot_dir = html_path.parent / "screenshots"
    screenshot_dir.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        context = browser.new_context(viewport={"width": 1440, "height": 900}, device_scale_factor=2)
        page = context.new_page()

        # Collect console errors/warnings
        page.on("console", lambda msg: console_errors.append(f"[{msg.type}] {msg.text}") if msg.type in ("error", "warning") else None)
        page.on("pageerror", lambda err: console_errors.append(f"[pageerror] {err}"))

        try:
            file_url = html_path.as_uri()
            page.goto(file_url, wait_until="networkidle", timeout=timeout_ms)
            page.wait_for_timeout(1000)  # let animations settle

            # Take initial screenshot
            init_ss = screenshot_dir / f"{html_path.stem}-initial.png"
            page.screenshot(path=str(init_ss), full_page=False)
            screenshots.append(str(init_ss))

            # Execute click sequence
            for i, selector in enumerate(click_sequence):
                try:
                    page.wait_for_selector(selector, timeout=timeout_ms, state="visible")
                    page.click(selector, timeout=timeout_ms)
                    page.wait_for_timeout(500)  # settle after click
                    steps_completed += 1

                    if screenshot_each:
                        ss_path = screenshot_dir / f"{html_path.stem}-step-{i+1}.png"
                        page.screenshot(path=str(ss_path), full_page=False)
                        screenshots.append(str(ss_path))

                except Exception as e:
                    failure_reason = f"Step {i+1} failed on selector '{selector}': {e}"
                    # Screenshot on failure
                    fail_ss = screenshot_dir / f"{html_path.stem}-fail-step-{i+1}.png"
                    try:
                        page.screenshot(path=str(fail_ss), full_page=False)
                        screenshots.append(str(fail_ss))
                    except Exception:
                        pass
                    break

            # Final screenshot if we completed all steps
            if not failure_reason and steps_total > 0:
                final_ss = screenshot_dir / f"{html_path.stem}-final.png"
                page.screenshot(path=str(final_ss), full_page=False)
                screenshots.append(str(final_ss))

        except Exception as e:
            failure_reason = f"Page load failed: {e}"

        context.close()
        browser.close()

    elapsed_ms = int((time.monotonic() - start) * 1000)

    return {
        "passed": failure_reason == "",
        "steps_completed": steps_completed,
        "steps_total": steps_total,
        "console_errors": console_errors,
        "screenshots": screenshots,
        "failure_reason": failure_reason,
        "duration_ms": elapsed_ms,
    }


if __name__ == "__main__":
    config = json.loads(sys.stdin.read())
    result = run(config)
    json.dump(result, sys.stdout)
