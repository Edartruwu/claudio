# Claudio Design Feature Audit Report

**Scope:** Sprint 1-5 feature build: Design Agent + BundleMockup + RenderMockup + VerifyMockup + ExportHandoff tools + 3 bundled skills + /designs gallery

**Last Audit:** Read-only static analysis of all feature files

---

## File-by-File Assessment

### 1. `internal/agents/agents.go`

**Correctness:** ✅ Implementation correct. Capabilities field added (line 73), DesignAgent() defined (line 311), LoadCustomAgents() supports agent dirs and flat files (line 332).

**Edge Cases:**
- Custom agents override built-in by type (AllAgents, line 489-494). Risk: silent shadowing if custom dir agent has same Type as built-in. **Not validated at load time.**
- LoadCustomAgents scans both directory form and flat .md files. Candidates prioritized (AGENT.md > agent.md > <dirname>.md). **OK** — clear precedence.

**Go Idioms:** ✅ Standard patterns. Slice operations, filepath.Join, frontmatter parsing via yaml.v3.

**Security:** ✅ No input validation issues. filepath.Join safe. Custom agent files read from disk only; no untrusted input injection.

**Missing Pieces:**
- Line 73 comment says "Known values: design" but no enum validation. **Typo risk:** agent author writes `Capability: "Design"` (capital D) → silently ignored, tools never register.
- No backward-compat check: if older agents.json from previous session has no Capabilities field, will it gracefully default to `[]`? (Likely yes in Go, but not explicitly tested.)

**Flag:** ⚠️ **WARN** — Typo in agent frontmatter `capabilities:` value silently ignored; no validation feedback.

---

### 2. `internal/agents/design_prompt.go`

**Correctness:** ⚠️ **CRITICAL** — Scoring threshold mismatch (see below).

**Key sections:**
- Step 6 (line 54-58): Call RenderMockup, inspect `console_errors`. ✅ Correct.
- Step 7 (line 60-65): Call VerifyMockup, check `overall_score >= 7.0`. ❌ **WRONG SCALE.**
- Step 7 (line 227-228): Secondary check: `if pass is true AND overall_score >= 90`. Implies score is 0-100, contradicting 7.0 check.

**Root cause:** verify_prompt.go defines `overall_score: <int 0-100>` with pass threshold `>= 75` (line 65 verify_prompt.go). But design_prompt line 62 says `>= 7.0`. **Off by 10x.**

**Impact:**
- Agent will ALWAYS pass step 7 (7.0 is ultra-low on 0-100 scale).
- Verification becomes toothless. Any non-zero score passes.
- Mockups of quality 7-74 (poor) will slip through to BundleMockup.

**Flag:** 🔴 **CRITICAL** — Scoring scale mismatch. Replace `7.0` with `75` in design_prompt.go line 62, 63.

---

### 3. `internal/config/config.go`

**Correctness:** ✅ Designs path added to Paths struct (line 167). EnsureDirs() includes p.Designs in mkdir list (line 214).

**Edge Cases:**
- Designs dir created with mode 0700 (line 216). ✅ Correct — private to user.
- No check if Designs path is relative. GetPaths() always returns absolute (via filepath.Join(base, ...)), so safe.

**Go Idioms:** ✅ Standard config initialization.

**Security:** ✅ No path traversal risk at config level. Perms are sane.

**Missing Pieces:** None identified.

**Flag:** ✅ **INFO** — Clean, minimal. No issues.

---

### 4. `internal/tools/bundle.go` (BundleMockupTool)

**Correctness:** ✅ Main logic correct. Inlines local scripts, optionally fetches and embeds CDN deps, writes bundle to disk.

**Edge Cases:**
- **CDN fetch failure handling** (line ~176-185): If CDN is unreachable (`resp.StatusCode != 200` or read error), **warning added but script tag left as-is**. Result: OfflineCapable becomes false (line 227), design_prompt sees warning, must re-render. ✅ Graceful.
- **File not found in Files map** (line ~127-137): Falls back to filepath.Join(htmlDir, srcRef). If that also fails, warning + original tag unchanged. ✅ Correct fallback.
- **Empty output path** (line ~190): Defaults to `~/.claudio/designs/{timestamp}/bundle/mockup.html`. ✅ Sensible.

**Go Idioms:** ✅ Standard regex, file I/O, JSON marshaling.

**Security:**
- ✅ No path traversal: srcRef is resolved via filepath.Join only; no ".." handling needed (browser will reject).
- ✅ No code injection: HTML is read, not eval'd.
- ⚠️ **HTTP client timeout is 30s** (line ~168). If CDN is very slow, bundle operation hangs for 30s. Acceptable but **not configurable**.

**Missing Pieces:**
- No validation that EntryHTML is actually HTML (could be .txt, binary, etc.). Likely OK since error will be caught downstream, but **could fail gracefully with better error message**.
- No size limit on CDN fetches. A malicious CDN returning 1GB could OOM. ⚠️ **Low risk** (design agent is trusted), but **missing defense**.

**Flag:** ⚠️ **WARN** — CDN fetch has no size limit; 30s timeout not user-configurable.

---

### 5. `internal/tools/render.go` + `render_script.go` (RenderMockupTool)

**Correctness:** ✅ Playwright Node.js script solid. Launches headless Chromium, captures full-page + artboard screenshots, waits for fonts, collects console errors.

**Edge Cases:**
- **Playwright prerequisite check** (line ~108-128): Runs `node --version` and `npx playwright --version`. If missing, returns helpful error message. ✅ Good UX.
- **HTMLPath not found** (execute, ~135): Returns error. ✅ Correct.
- **Viewport defaults:** width=1440, height=900, scale=2. Hard-coded but sensible.

**Node.js script robustness:**
- Line 56-60 (render_script.go): Launches browser, creates context with viewport + deviceScaleFactor. ✅
- Line 65-72: Attaches console/pageerror listeners, collects in arrays. ✅
- Line 75-79: Waits for networkidle + font readiness. ✅ Good.
- Line 84-97: Captures full page + artboards (data-artboard). ✅
- Line 105-112: Catches Playwright errors, returns JSON with success=false. ✅

**Go Idioms:** ✅ Embeds Node.js as const, writes to temp file, execs via exec.Command with timeout support (via context, though context not fully used in current impl — see below).

**Security:**
- ✅ HTML path passed as argument, not eval'd.
- ⚠️ **Output dir hard-coded** in Node.js: `flag('--out-dir', '')` defaults to empty, script rejects (line 44-51). ✅ Safe.

**Missing Pieces:**
- **Context timeout not fully used**: RenderMockupTool.Execute receives context but doesn't pass it to exec.CommandContext. Node.js could hang indefinitely if page.waitForTimeout(2000) loops. ⚠️ **Potential hang risk** if malicious HTML contains infinite loops.
- **Playwright path not configurable:** Script tries `require('playwright')` then fallback to `@playwright/test`. If user has playwright in a custom npm dir, this may fail. ⚠️ **Low risk** but **not flexible**.

**Flag:** ⚠️ **WARN** — Context timeout not passed to exec; Node.js process could hang if HTML loops. Should use `exec.CommandContext(ctx, "node", ...)`.

---

### 6. `internal/tools/verify.go` + `verify_prompt.go` (VerifyMockupTool)

**Correctness:** ✅ Overall solid. Loads screenshot, encodes to base64, builds prompt with HTML context, calls vision LLM, parses JSON result.

**Critic model selection** (line 97-105): Priority: env var CLAUDIO_DESIGN_CRITIC_MODEL > constructor param > hardcoded haiku. ✅ Good.

**Edge Cases:**
- **Screenshot not found** (line 128-131): Returns error. ✅
- **HTML context truncated** (line 139-141): If HTML > 8000 chars, truncates and adds "... (truncated)". ✅ Safe, prevents token explosion.
- **JSON parse failure** (line 207-213): If LLM response isn't valid JSON, returns raw text + parse error. Caller sees error but can still inspect raw critique. ✅ Defensive.
- **Markdown fence stripping** (line 193-204): Handles ```json and plain ``` wrapping. ✅ LLM hygiene.

**Go Idioms:** ✅ Standard base64, JSON, string manipulation.

**Security:**
- ✅ Screenshot path checked (after RemapPathForWorktree), file read is isolated.
- ✅ No code injection: screenshot is image data, HTML context is text only.
- ✅ HTML context truncation prevents DoS via huge HTML.

**Missing Pieces:**
- **InputSchema requires design_brief but makes html_path optional** (line 87-88). Good. But if HTML is missing and brief is vague, critic can't assess properly. ⚠️ **Design workflow issue, not code issue.**
- **Slice initialization** (line 216-224): Ensures non-nil slices in output for clean JSON. ✅ Good practice.

**Flag:** ✅ **INFO** — Solid implementation. No critical issues.

---

### 7. `internal/tools/handoff.go` (ExportHandoffTool)

**Correctness:** ✅ Parses HTML + CSS for components, assets, fonts, generates spec.md + tokens-used.json.

**Edge Cases:**
- **mockup_dir not found:** Returns error. ✅
- **HTML parsing via regex:** Uses `regexp.MustCompile` patterns (line 91-112). ✅ Correct for quick scraping (not full HTML5 parser, but adequate for design-generated HTML).
- **Screenshot path traversal in template** (designs.html line 42): Uses `/designs/static/{.ID}/screenshots/{screenshot}`. Server side (handleDesignStatic) validates path. ✅

**Component detection** (line 114-136): Maps Tailwind class keywords (btn, button, card, etc.) to component names. Heuristic-based. ✅ Reasonable for generated mockups.

**Go Idioms:** ✅ Standard regexp, file I/O, JSON.

**Security:**
- ✅ No path traversal: FilePath.Join used correctly, cleaned path checked (line 1708-1709).
- ✅ No code exec: HTML parsed as text.

**Missing Pieces:**
- **Regex patterns assume Tailwind:** If designer uses inline styles instead, component detection fails silently. ⚠️ **Expected limitation for design-generated HTML**, but **not documented**.
- **Design-system.json not validated:** If user provides invalid design-system.json path, error is swallowed (line 179). ⚠️ **Silent failure, should warn.**

**Flag:** ⚠️ **WARN** — design_tokens path error silently ignored. Should warn if path provided but unreadable.

---

### 8. `internal/services/skills/loader.go`

**Correctness:** ✅ Bundled skills registered (commit, review, simplify, ..., design-system, mockup, handoff at lines 316-331). Content is embedded const.

**Design-system skill** (line 1862): Instructs user to save to `~/.claudio/designs/design-system.json`. ✅ Matches handoff.go parsing logic.

**Mockup skill** (line 1881): Mentions optional design-system.json path. ✅ Consistent.

**Handoff skill** (line 1990): Mentions design-system.json parsing. ✅ Consistent.

**Go Idioms:** ✅ Standard skill registry, YAML frontmatter parsing.

**Security:** ✅ Const content, no input risk.

**Missing Pieces:**
- Bundled skills are locked in code. ⚠️ **Design decisions (token scale, scoring threshold) embedded in strings**, not configurable. **This is root of mismatch bug** — design_prompt and verify_prompt have conflicting thresholds.

**Flag:** ⚠️ **WARN** — Threshold values hardcoded in skill content; scoring mismatch should be fixed in code.

---

### 9. `internal/tui/root.go` (registerCapabilityTools)

**Correctness:** ✅ Function at line 1932-1943 checks if agent has "design" capability, registers all 4 tools.

```go
if slices.Contains(capabilities, "design") {
    registry.Register(tools.NewBundleMockupTool(paths.Designs))
    registry.Register(tools.NewRenderMockupTool(paths.Designs))
    registry.Register(tools.NewVerifyMockupTool(paths.Designs, client, ""))
    registry.Register(tools.NewExportHandoffTool(paths.Designs))
}
```

**Logic:** ✅ Correct gate. Tools only available to agents with "design" capability.

**Execution:** Called twice (line 1761, 1912) on agent selection and startup. ✅

**Go Idioms:** ✅ Standard registry pattern.

**Edge Cases:**
- ✅ If Designs dir doesn't exist yet, EnsureDirs() was called at startup, so it exists.
- ✅ Empty capabilities slice → tools not registered. Correct.

**Missing Pieces:** None.

**Flag:** ✅ **INFO** — Clean, correct capability gating.

---

### 10. `internal/tui/agentselector/selector.go` (AgentSelectedMsg.Capabilities)

**Correctness:** ✅ AgentSelectedMsg struct includes Capabilities field (line 22). Populated in Update() (line 146, 164).

**Go Idioms:** ✅ Standard Bubble Tea message.

**Edge Cases:** None identified.

**Flag:** ✅ **INFO** — Clean pass-through.

---

### 11. `internal/comandcenter/web/server.go` (/designs routes)

**handleDesignGallery (line 1648-1692):**

**Correctness:** ✅ Lists all design sessions from ~/.claudio/designs/, checks for bundle/mockup.html and handoff/spec.md.

**Edge Cases:**
- **Designs dir doesn't exist:** `os.ReadDir()` returns error, check `!os.IsNotExist(err)` correctly handles (line 1652-1656). ✅ Returns empty session list if dir missing. OK since gallery shows "No designs yet".
- **Screenshots dir missing:** `os.ReadDir()` called, error ignored (line 1674). ✅ Allows missing screenshots.
- **Sorting:** Sessions sorted newest first by ID (assumed timestamp) (line 1686-1688). ✅ Sensible.

**Sorting assumption risk:** ⚠️ If session ID is NOT a timestamp, sort order is unpredictable. **Design agent should name dirs with timestamp** (e.g., 20240115-143025). **Not enforced in code.**

---

**handleDesignStatic (line 1694-1715):**

**Path traversal defense:**
1. **Early ".." check** (line 1702): Rejects if id or rest contains "..". ✅ First defense.
2. **filepath.Clean check** (line 1707-1712): Resolves fp, ensures it starts with designsDir + "/". ✅ Second defense (standard Go mitigation).

**Correctness:** ✅ Solid double-check. This is the recommended pattern for path validation.

**Edge Cases:**
- **Symlink traversal:** If designsDir contains a symlink to /etc, filepath.Clean doesn't follow symlinks. ⚠️ **Symlink escape risk** — attacker creates symlink in designs dir pointing outside. However, **Designs dir is created with 0700**, only user can write, so **low risk in practice.**
- **Case-sensitive filesystems:** path check assumes case-sensitive. On case-insensitive FS (macOS), `../EVIL` bypasses check. ⚠️ **Low risk** (would need uppercase ".." which is unusual).

**Flag:** ✅ **INFO** — Path traversal defense is solid. Symlink risk mitigated by tight perms on Designs dir.

---

### 12. `internal/comandcenter/web/templates/designs.html`

**Correctness:** ✅ Gallery template. Renders session cards with bundle/handoff links.

**Asset serving** (line 42, 101, 112):
- Screenshots: `/designs/static/{{.ID}}/screenshots/{{filename}}`
- Bundle: `/designs/static/{{.ID}}/bundle/mockup.html`
- Handoff spec: `/designs/static/{{.ID}}/handoff/spec.md`

**Go Idioms:** ✅ Standard html/template, condition blocks, loops.

**Security:**
- Screenshot filenames come from `os.ReadDir()` on server (server.go line 1674-1678). ✅ Not user-controlled.
- Bundle/handoff checked with `os.Stat()` before rendering link (server.go line 1668, 1671). ✅
- Server-side path validation on handleDesignStatic. ✅

**HTML quality:** Good UX. Empty state, loading fallback, responsive grid. ✅

**Flag:** ✅ **INFO** — Solid template. No issues.

---

## Summary Table

| File | Issue | Severity | Fix |
|------|-------|----------|-----|
| `agents/design_prompt.go` | Score threshold 7.0 vs 75 mismatch | 🔴 **CRITICAL** | Change line 62, 63 to compare against 75, not 7.0 |
| `agents/agents.go` | Capability typos silently ignored | ⚠️ **WARN** | Add validation in LoadCustomAgents/AllAgents to warn if unknown capability detected |
| `tools/bundle.go` | CDN fetch no size limit; timeout hardcoded | ⚠️ **WARN** | Add max size check (e.g., 50MB); consider making timeout configurable |
| `tools/render.go` | Context timeout not passed to exec.Command | ⚠️ **WARN** | Use `exec.CommandContext(ctx, ...)` to respect caller timeout |
| `tools/handoff.go` | design_tokens path error silently ignored | ⚠️ **WARN** | Warn if design_tokens provided but unreadable |
| `services/skills/loader.go` | Embedded skill content has hardcoded thresholds | ⚠️ **WARN** | Move threshold values to agent config or environment; update design_prompt skill content |

---

## Risks & Observations

### High Priority

1. **🔴 Score threshold bug breaks verification gate** (agents/design_prompt.go line 62-63).
   - Design agent expects 0-10 scale; VerifyMockup uses 0-100 scale.
   - Mock-ups scoring 7-74 (poor) will pass verification.
   - **Fix:** Replace `7.0` with `75` in design_prompt; verify both scales match in all skill content + code.

### Medium Priority

2. **⚠️ Capability typos not caught** (agents/agents.go, agents/design_prompt.go).
   - If agent author writes `capabilities: ["Design"]` (capital D), silently ignored.
   - Tools never register; agent fails silently without explanation.
   - **Fix:** Add validation in `LoadCustomAgents()` to warn of unrecognized capabilities (known: "design").

3. **⚠️ Context timeout not honored in RenderMockup** (tools/render.go).
   - exec.Command doesn't use context timeout; Node.js process could hang indefinitely.
   - **Fix:** Change `exec.Command("node", ...)` to `exec.CommandContext(ctx, "node", ...)`.

4. **⚠️ Silent failure if design_tokens path is invalid** (tools/handoff.go).
   - If user provides design_tokens path that doesn't exist, error swallowed.
   - Spec is generated without token audit; user doesn't know.
   - **Fix:** Log warning if design_tokens provided but unreadable.

### Low Priority (Design)

5. ⚠️ **Session ID must be timestamp for consistent sort** (web/server.go line 1686).
   - Not enforced; if non-timestamp ID used, sort unpredictable.
   - **Design practice:** Bundle.go defaults to timestamp format (line 196). Safe if always used. Consider documenting or validating.

6. ⚠️ **CDN fetch has no size limit** (tools/bundle.go).
   - Malicious CDN could OOM. Low risk (design agent is trusted) but missing defense.
   - **Fix:** Add max fetch size (e.g., 50MB), reject if exceeded.

7. ⚠️ **CDN timeout hardcoded to 30s** (tools/bundle.go line 168).
   - Not configurable. Could cause long hangs. Acceptable but inflexible.
   - **Consider:** Make configurable via env var or input param.

### Code Quality

8. ✅ **Path traversal defenses are solid** (web/server.go, bundle.go, handoff.go).
   - Double-check pattern on handleDesignStatic. Symlink risk mitigated by 0700 perms.

9. ✅ **Go idioms are standard throughout.**
   - No unsafe patterns, proper error handling, clean JSON marshaling.

10. ✅ **Skill content is consistent across design-system, mockup, handoff.**
    - References same file paths, token formats. Good integration.

---

## Open Questions

1. **Has the design_prompt score mismatch been caught in testing?**
   - If VerifyMockup is returning scores 0-100 but agent compares against 7.0, all mockups silently pass step 7.
   - Should see widespread bug reports. May already be known.

2. **Is there a test suite for VerifyMockup prompt compliance?**
   - Vision LLM output format is JSON; parse failure is handled (line 207), but is output schema validated elsewhere?

3. **What happens if user manually creates .claudio/designs/ before first design run?**
   - Should be fine; EnsureDirs is idempotent. But if perms are wrong (not 0700), subsequent writes might fail.

4. **Are there integration tests verifying the full pipeline (design → render → verify → bundle)?**
   - Not visible in this audit; would want end-to-end tests.

5. **How is DesignAgent performance on large/complex mockups?**
   - Max 3 render+verify cycles (design_prompt line 64). If design is complex, might hit limit. Acceptable but worth documenting.

---

## Conclusion

**Overall Status:** 🟡 **Mostly Solid, One Critical Bug**

The Claudio Design feature is well-architected. Capabilities gating, tool registration, path security, and skill integration all work correctly. **However, the scoring threshold mismatch in design_prompt.go is a critical flaw** that breaks the verification gate. Combined with a few medium-risk issues (context timeout, silent error handling), **this feature should not be released without fixes.**

**Recommended action:** Fix critical bug first, then address WARN-level issues before rollout.
