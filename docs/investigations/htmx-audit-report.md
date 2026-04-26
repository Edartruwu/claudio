# HTMX/Templ Audit Report

Audited all `.templ` files, `static/app.js`, `static/sw.js`, `routes.go`, and `handlers.go` in `internal/comandcenter/web/`.

---

## Critical (causes visible jank / full reloads)

### C1. Back button swaps entire `<body>` innerHTML
- **File:** `chat_view.templ:20-23`
- **Pattern:** `hx-get="/sessions" hx-target="body" hx-swap="innerHTML" hx-push-url="/"`
- **Problem:** Replaces entire `<body>` when navigating back from chat to session list. Destroys all JS state, WS connections, and causes a visible full-page flash. Same effect as a hard reload but worse (no browser paint optimizations).
- **Fix:** Target `#main` instead of `body`. Handler already serves partial for HX-Request. Match the desktop session-click pattern in `layout.templ:812` which correctly uses `{target: '#main', swap: 'innerHTML'}`.

### C2. Login form swaps entire `<body>` outerHTML
- **File:** `login.templ:169-172`
- **Pattern:** `hx-post="/login" hx-target="body" hx-swap="outerHTML"`
- **Problem:** `outerHTML` on `<body>` is dangerous and explicitly warned against in `layout.templ:77` comment: _"hx-swap='outerHTML' on `<body>` must never be used"_. Can cause htmx to lose its root reference and break all subsequent interactions.
- **Fix:** Use `HX-Redirect` response header from the Go handler instead. Login handler should return `w.Header().Set("HX-Redirect", "/")` on success, re-render login form partial on failure with 422 status.

### C3. Message send via `fetch()` bypasses htmx entirely
- **File:** `chat_view.templ:501-504` (ccSend fn) and `chat_view.templ:744-748` (ccSendCommand)
- **Pattern:** `htmx.ajax('POST', ...)` with `swap: 'none'` for text messages, but `fetch()` with manual `Content-Type` header for slash commands.
- **Problem:** Two different mechanisms for the same action (sending messages). The `fetch()` path for `/compact`, `/clear` etc. gets no htmx lifecycle events, no `hx-indicator` support, no `hx-disabled-elt`. User gets no visual feedback that the command was sent (only a JS toast on error).
- **Fix:** Unify on `htmx.ajax()` or declarative `hx-post` for all message sends. Add `hx-indicator` on the send button and `hx-disabled-elt="this"` to prevent double-sends.

### C4. Session navigation uses `<a href>` without `hx-boost` — mobile gets full-page reload
- **File:** `session_row.templ:69-73`, `layout.templ:806-813`
- **Pattern:** Plain `<a href="/chat/{id}">` links. Desktop intercepts clicks via JS in `layout.templ:806` and calls `htmx.ajax()`. Mobile falls through to default browser navigation = full page reload.
- **Problem:** Mobile users experience hard navigation on every session click. Full page reload tears down WS, re-parses all CSS/JS, flashes white. This is the #1 source of perceived "clunkiness" on mobile.
- **Fix:** Add `hx-boost="true"` on the session list container or on the `<a>` tags directly. Or add `hx-get` + `hx-target="#main"` + `hx-push-url="true"` on the `<a>` and remove the JS interception. Both approaches give SPA-like nav on all viewports.

---

## Major (unnecessary round-trips / poor UX)

### M1. `fetch()` for agent/team settings instead of htmx forms
- **File:** `info_panel.templ:120-141`
- **Pattern:** `applyAgent()` and `applyTeam()` use `fetch('/api/sessions/{id}/set-agent', ...)` with manual JSON body, then call `_showFeedback()` JS function.
- **Problem:** 30+ lines of imperative JS for what could be a `<form hx-post>` with an `hx-swap="none"` + `HX-Trigger` response header. No loading state, no disabled-during-request, no error boundary.
- **Fix:** Convert to `<form hx-post="/api/sessions/{id}/set-agent" hx-swap="none" hx-indicator="#agent-feedback">`. Handler returns `HX-Trigger: {"showToast":{"msg":"Saved!"}}`.

### M2. File browser is 100+ lines of `fetch()` + manual DOM — should be htmx partials
- **File:** `chat_view.templ:510-630`
- **Pattern:** `ccBrowse()` fetches JSON from `/api/sessions/{id}/browse`, then manually constructs HTML via string concatenation (`innerHTML = html`).
- **Problem:** Duplicates server-rendered HTML philosophy. Manual `innerHTML` with `escHtml()` is fragile (XSS surface if any edge case missed). No htmx lifecycle events, no indicator. ~120 lines of JS that could be 0.
- **Fix:** Create a `FileBrowserPartial` templ component. Use `hx-get="/partials/sessions/{id}/browse?path=..."` + `hx-target="#fb-list"` + `hx-swap="innerHTML"`. Server returns HTML, htmx handles swap.

### M3. @mention autocomplete fetches JSON + builds DOM manually
- **File:** `chat_view.templ:632-706`
- **Pattern:** `fetch('/api/sessions/list')` returns JSON, JS builds dropdown HTML with inline styles.
- **Problem:** 75 lines of JS for autocomplete that could be a templ partial. JSON API endpoint duplicates data the server already has. Cache logic (10s TTL) adds complexity.
- **Fix:** `hx-get="/partials/mentions?q={prefix}"` with `hx-trigger="keyup changed delay:150ms"` on a hidden input. Server returns `<div>` with mention options. Cache at HTTP level with `Cache-Control: max-age=10`.

### M4. `messages.reload` and `messages.compacted` WS handlers use `fetch()` instead of htmx
- **File:** `app.js:347-366`
- **Pattern:** On WS events `messages.reload`/`messages.compacted`, JS calls `fetch('/partials/messages/' + sessionId)` then sets `innerHTML`.
- **Problem:** Bypasses htmx entirely. No `htmx:afterSwap` event fires, no extensions process, no `hx-swap-oob` possible. If any htmx-powered elements are in the messages container, they won't be initialized.
- **Fix:** Use `htmx.ajax('GET', '/partials/messages/' + sessionId, {target: '#messages', swap: 'innerHTML'})`. This ensures htmx processes the response and initializes any htmx attributes in the new content.

### M5. Project filter chips built via `fetch()` + manual DOM construction
- **File:** `chat_list.templ:106-149`
- **Pattern:** `fetch('/api/projects')` returns JSON → JS creates buttons via `document.createElement()`.
- **Problem:** 45 lines of DOM construction JS. Server already knows the projects — should render them in the initial templ output or as an htmx partial.
- **Fix:** Either include project chips in the initial `ChatList` templ render (data available server-side), or `hx-get="/partials/project-chips"` with `hx-trigger="load"` on a container div.

### M6. No `hx-indicator` on any slow operation except session list
- **File:** All templ files
- **Pattern:** Only `chat_list.templ:159` has `hx-indicator=".htmx-indicator"`. All other htmx requests (info panel tabs, task detail expand, cron delete, archive/delete session, back button) have zero loading feedback.
- **Problem:** User clicks, nothing happens for 200-500ms, then content appears. Feels broken/laggy.
- **Fix:** Add `hx-indicator` to info panel tab buttons (`info_panel.templ:176`), task detail rows (`info_panel.templ:225`), the back button (`chat_view.templ:20`), and archive/delete buttons (`session_row.templ:10,21`). The `LoadingSpinner` component already exists in `components.templ:118` — use it.

### M7. No `hx-disabled-elt` on destructive actions
- **File:** `session_row.templ:10,21,48,57`, `cron_list.templ:36`, `settings.templ:32`
- **Pattern:** Delete/archive buttons have no `hx-disabled-elt`. User can click multiple times before response arrives.
- **Problem:** Double-delete sends duplicate requests. Archive + Delete race possible on slow connections.
- **Fix:** Add `hx-disabled-elt="this"` on all destructive action buttons.

---

## Minor (best practice gaps)

### m1. Zero `hx-boost` usage across entire codebase
- **File:** All navigation links in `chat_list.templ:167,175,185` (bottom nav: Home, Designs, Settings)
- **Pattern:** Plain `<a href="/designs">`, `<a href="/settings">` — hard navigation.
- **Problem:** Every bottom-nav tap on mobile = full page reload. Easy to fix globally.
- **Fix:** Add `hx-boost="true"` on the `<nav>` container or on `<body>`. All `<a>` tags inside will automatically get SPA-like navigation.

### m2. No `hx-sync` on filter chip buttons
- **File:** `chat_list.templ:101-103`
- **Pattern:** Filter chip `onclick` calls `htmx.ajax('GET', '/partials/sessions?filter=...')`. Rapid clicks queue multiple requests.
- **Problem:** User clicking Active → Inactive → Active fast sends 3 requests; results may arrive out of order → shows wrong filter.
- **Fix:** Add `hx-sync="closest #session-list:replace"` or use `hx-get` + `hx-target` instead of `onclick` + `htmx.ajax()`, with `hx-sync` to cancel stale requests.

### m3. No View Transitions or swap transition CSS
- **File:** Global (all files)
- **Pattern:** No `htmx.config.globalViewTransitions = true`, no `.htmx-settling`/`.htmx-swapping` CSS classes used.
- **Problem:** Content swaps are instant cut — no visual continuity. View Transitions API (supported in Chrome/Edge/Safari) gives smooth cross-fade for free.
- **Fix:** Add `<meta name="htmx-config" content='{"globalViewTransitions":true}'>` in `layout.templ` `<head>`. Add CSS: `.htmx-swapping { opacity: 0; transition: opacity 0.15s; }`.

### m4. Team members polling every 30s when WS already pushes updates
- **File:** `info_panel.templ:264`
- **Pattern:** `hx-trigger="refresh from:body, every 30s"` on team members list.
- **Problem:** WS already sends `agent_status` events that trigger `htmx.trigger(document.body, 'refresh')` via `app.js:345`. The `every 30s` fallback still fires even when WS is connected, causing unnecessary requests.
- **Fix:** Either: (a) Remove `every 30s` and rely solely on WS `refresh` events (add a WS reconnect refresh as safety net), or (b) Gate polling: `hx-trigger="refresh from:body, every 30s[!window._wsConnected]"` — only polls when WS is disconnected.

### m5. Agent logs fetched via `fetch()` in WS handler
- **File:** `app.js:381-390`
- **Pattern:** On `agent.log` WS event, JS calls `fetch(...)` then sets `body.textContent`.
- **Problem:** Could use `htmx.ajax()` to benefit from htmx processing. Minor since it's plain text content.
- **Fix:** Low priority. Consider `htmx.ajax()` for consistency.

### m6. Session list refresh via `fetch()` in `reloadMessages()`
- **File:** `app.js:215-231`
- **Pattern:** `reloadMessages()` calls `fetch('/partials/messages/' + sessionId)` then sets `msgs.innerHTML`.
- **Problem:** Same as M4 — bypasses htmx. Any htmx-attributed elements in the response won't be initialized by htmx's internal processor.
- **Fix:** Use `htmx.ajax()` instead of `fetch()` for HTML fragment fetches.

### m7. Interrupt button has no disabled state during request
- **File:** `chat_view.templ:717-731`
- **Pattern:** `ccInterrupt()` is async `fetch()` with no UI disable. User can spam-click.
- **Problem:** Multiple interrupt requests sent. Minor since server is idempotent, but noisy.
- **Fix:** Add `_interrupting` guard (like `_sending` for messages) or convert to `hx-post` with `hx-disabled-elt`.

### m8. `hx-swap="none"` on delete-all-sessions without redirect
- **File:** `settings.templ:32-35`
- **Pattern:** `hx-delete="/api/sessions/all" hx-swap="none"` — deletes all sessions but page stays unchanged.
- **Problem:** After deleting all sessions, session list still shows old data until next poll/refresh. User thinks nothing happened.
- **Fix:** Handler should return `HX-Trigger: refresh` header to force session list reload + show a toast. Or use `HX-Redirect: /` to reload.

### m9. Desktop info panel auto-load via `htmx.ajax()` in `<script>` — could be declarative
- **File:** `chat_view.templ:311-316`
- **Pattern:** JS checks `window.innerWidth >= 1024` then calls `htmx.ajax('GET', '/chat/' + sid + '/info', ...)`.
- **Problem:** Mixes imperative and declarative styles. Could use `hx-get` with `hx-trigger="load"` and a CSS media query to hide on mobile.
- **Fix:** Low priority. The viewport check is hard to do purely declaratively. Current approach works.

---

## Summary

- **4 critical**, **7 major**, **9 minor** issues found
- **Total: 20 issues**

### Top 3 highest-impact fixes (ranked by UX improvement)

1. **C4 + m1: Add `hx-boost` to navigation links and session row `<a>` tags**
   - Impact: Eliminates full-page reloads for ALL navigation on mobile. Single biggest UX win. Turns a multi-page app into SPA-like experience with ~5 lines of code.
   - Effort: Low (add `hx-boost="true"` to nav containers + ensure handlers serve partials for `HX-Request`).

2. **C1 + C2: Fix body-targeting swaps**
   - Impact: Back button and login both currently destroy the entire DOM. Fixing these eliminates the two most jarring full-page flashes. Login fix also removes a dangerous `outerHTML` on `<body>`.
   - Effort: Low-medium (change targets to `#main`, add `HX-Redirect` to login handler).

3. **M6 + M7: Add `hx-indicator` and `hx-disabled-elt` globally**
   - Impact: Every interactive element gets loading feedback and double-click protection. Transforms perceived responsiveness.
   - Effort: Low (components already exist — just wire them up).

### Architecture note

The app has a **split personality**: half declarative htmx, half imperative `fetch()` + manual DOM. The templ templates are well-structured with good partial boundaries, but the `<script>` blocks in `chat_view.templ` (500+ lines) and `layout.templ` (200+ lines) duplicate htmx's responsibilities. A phased migration of `fetch()` calls to htmx partials would reduce JS by ~60% and make the UI consistently smooth.
