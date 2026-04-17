# ComandCenter → WhatsApp Mobile UX Gap Report

**Auditor:** iris (UI/UX)  
**Date:** 2025-07-14  
**Scope:** All templates in `internal/comandcenter/web/` vs WhatsApp mobile (iOS/Android)  
**Mode:** Read-only investigation — no code changes

---

## Files Audited

| File | Role |
|------|------|
| `templates/layout.html` | Base layout, CSS, meta tags |
| `templates/login.html` | Password login screen |
| `templates/chat_list.html` | Session list (WA home screen equiv) |
| `templates/chat_view.html` | Chat conversation view |
| `templates/components/message_bubble.html` | Message bubble component |
| `templates/components/session_row.html` | Session list row component |
| `templates/partials/messages.html` | Messages partial (htmx swap) |
| `templates/partials/sessions.html` | Sessions partial (htmx swap) |
| `static/app.js` | WebSocket client for live messages |

---

## Screen 1: Login

### Current State
- Centered card w/ WA-green logo circle, title, password field, submit button
- Error banner for invalid password
- `bg-gray-100` background (not WA palette)

### WhatsApp Reference
- WA has no password login — uses phone verification + QR code
- Closest analog: WA Web QR code screen w/ large logo, explanation text, green accent

### Gaps

| ID | Pri | File | Element | Gap | WA Reference |
|----|-----|------|---------|-----|-------------|
| L1 | P1 | `login.html` | `<input type="password">` | `py-2` → ~32px height. Must be ≥44px for touch + 16px font to prevent iOS auto-zoom | WA inputs always ≥48px height, 16px+ font |
| L2 | P2 | `login.html` | `<button>` | `py-2` → ~32px height. Touch target too small | WA buttons ≥48px |
| L3 | P2 | `login.html` | background | `bg-gray-100` not WA palette — should be `#ECE5DD` or WA teal gradient | WA uses branded teal bg on onboarding |
| L4 | P2 | `layout.html` | `<meta viewport>` | `maximum-scale=1` blocks pinch-zoom — accessibility violation (WCAG 1.4.4) | Remove `maximum-scale=1` |
| L5 | P1 | `login.html` | form | No loading state on submit — button stays active during POST | WA shows spinner on auth actions |
| L6 | P2 | `layout.html` | safe areas | No `env(safe-area-inset-*)` padding anywhere | WA respects all safe-area insets (notch, home indicator) |

---

## Screen 2: Chat List (Home)

### Current State
- WA-green header w/ title + search/menu icons
- Search bar below header (rounded pill)
- Session rows: 48px avatar circle (letter), name+timestamp top row, last-message+status-dot bottom row
- Polling refresh via `hx-get` every 3s
- Empty state: "No active sessions" text

### WhatsApp Reference
- Header: "WhatsApp" title, camera icon, search icon, overflow menu
- Tab bar below header: Chats / Status / Calls (segmented or bottom tabs)
- Each row: 50px avatar (photo), name (bold) + timestamp (right-aligned), last msg preview + delivery ticks + unread badge (blue circle w/ count)
- FAB: green floating action button (new chat) bottom-right
- Pull-to-refresh

### Gaps

| ID | Pri | File | Element | Gap | WA Reference |
|----|-----|------|---------|-----|-------------|
| C1 | P0 | `chat_list.html` | header icons | SVG touch targets are 20px (w-5 h-5) with no padding expansion → fail 44px minimum | WA icons are 24px with ≥48px touch area |
| C2 | P1 | `chat_list.html` | search input | `py-2 text-sm` → too small for comfortable mobile touch (~32px height, ~14px font) | WA search: ≥40px height, 16px font |
| C3 | P1 | `session_row.html` | entire row | Row height implicit via `py-3` (~72px total) — acceptable but status dot is 10px (w-2.5) w/ no touch expansion. Status info is secondary but dot is tiny | WA rows ~72px, no standalone status dot — uses delivery ticks + unread badge instead |
| C4 | P1 | `session_row.html` | unread badge | **Missing entirely** — no unread message count badge | WA shows blue circle w/ white count number, right-aligned |
| C5 | P1 | `session_row.html` | delivery ticks | **Missing** — no read/delivered indicators | WA shows ✓ ✓ (grey=delivered, blue=read) |
| C6 | P1 | `chat_list.html` | tabs | **No tab bar** — WA has Chats/Status/Calls tabs. ComandCenter could use Chats/Tasks/Agents tabs | WA has segmented top tabs (Android) or bottom tabs |
| C7 | P1 | `chat_list.html` | FAB | **No floating action button** for new session | WA has green FAB bottom-right for new chat |
| C8 | P2 | `chat_list.html` | timestamp | `text-xs text-gray-400` — low contrast on white bg (~2.5:1). Fails WCAG AA | WA timestamps: `text-xs text-gray-500` (~4.5:1) or green for unread |
| C9 | P2 | `chat_list.html` | empty state | Plain text only "No active sessions" — no icon, no action | WA shows illustration + "Start messaging" CTA |
| C10 | P2 | `chat_list.html` | pull-to-refresh | **Not implemented** — relies on 3s polling only | WA supports pull-to-refresh gesture |
| C11 | P0 | `layout.html` | safe areas | `body { max-width: 480px; margin: 0 auto }` — no `padding-top: env(safe-area-inset-top)` → content hides under notch/Dynamic Island | WA uses safe-area padding on all edges |
| C12 | P1 | `session_row.html` | avatar | Letter avatar on solid green bg — no visual variety between sessions | WA uses unique colors or actual photos |
| C13 | P2 | `session_row.html` | last message | `text-xs` (~12px) — at minimum readable size, could be 13-14px | WA preview: ~14px |

---

## Screen 3: Chat View

### Current State
- WA-green header: back arrow, 36px avatar circle, name+path text, status dot, menu icon
- Messages area: `#ECE5DD` bg, bubbles w/ different styles per role
- User bubbles: `#25D366` bg, white text, right-aligned, rounded 12px
- Assistant bubbles: `#fff` bg, grey text, left-aligned, rounded 12px
- Tool-use: centered grey italic small text w/ 🔧 emoji
- Input bar: text input + round green send button
- WebSocket for live message streaming

### WhatsApp Reference
- Header: back arrow, 40px avatar, name (bold) + "online"/"typing..."/"last seen" status, video call icon, phone icon, overflow menu
- Messages: wallpaper bg, sent bubbles `#DCF8C6` (light green) not bright green, received `#FFFFFF`
- Bubble details: tail/arrow on bubble corner, delivery ticks (✓✓), timestamp inside bubble (right-aligned, smaller)
- Input bar: emoji icon (left), attachment icon (left), text field, camera icon (when empty) / send icon (when text present)
- Typing indicator: "typing..." under contact name in header
- Reply-to: swipe right on message to quote-reply
- Date dividers: "Today", "Yesterday", "Jan 5" floating pills between message groups

### Gaps

| ID | Pri | File | Element | Gap | WA Reference |
|----|-----|------|---------|-----|-------------|
| V1 | P0 | `chat_view.html` | header back arrow | `w-5 h-5` SVG = 20px touch target, no padding wrapper → fails 44px minimum | WA back button: ≥48px tap area |
| V2 | P0 | `chat_view.html` | input field | `py-2 text-sm` → ~32px height, ~14px font → **iOS will auto-zoom** on focus since font < 16px | WA input: ≥44px height, 16px font |
| V3 | P0 | `chat_view.html` | input bar | No `padding-bottom: env(safe-area-inset-bottom)` → input hides behind home indicator on iPhone X+ | WA input bar respects bottom safe area |
| V4 | P0 | `message_bubble.html` | user bubble color | `#25D366` (bright neon green) w/ white text — low contrast (~2.1:1), **fails WCAG AA** | WA sent: `#DCF8C6` (pastel green) w/ dark text (~7:1 contrast) |
| V5 | P1 | `chat_view.html` | send button | `w-10 h-10` = 40px — under 44px minimum | WA send button: 48px |
| V6 | P1 | `message_bubble.html` | bubble width | `max-w-xs` = 320px — on 375px screen that's 85% width, leaving almost no alignment gap. On wider phones it's fine | WA bubbles: max ~75-80% of screen width |
| V7 | P1 | `message_bubble.html` | bubble corners | User: `12px 0 12px 12px`, Assistant: `0 12px 12px 12px` — has tail-like flat corner ✓ but no actual tail/arrow shape | WA has SVG tail/notch on bubble corner |
| V8 | P1 | `message_bubble.html` | date dividers | **Missing** — no "Today"/"Yesterday" separators between message groups | WA shows floating date pills |
| V9 | P1 | `chat_view.html` | typing indicator | **Missing** — no "typing..." state shown | WA shows "typing..." in header subtitle + animated dots |
| V10 | P1 | `chat_view.html` | header actions | No call/video icons — acceptable for AI chat, but no action icons at all besides menu | WA has 2-3 action icons in header |
| V11 | P1 | `message_bubble.html` | delivery ticks | **Missing** — no sent/delivered/read indicators on user messages | WA shows ✓ (sent) ✓✓ (delivered) blue ✓✓ (read) |
| V12 | P1 | `message_bubble.html` | timestamp placement | Timestamps on separate line below text → wastes vertical space | WA: timestamp inline at bottom-right of text, same line |
| V13 | P1 | `chat_view.html` | input attachments | **Missing** — no emoji picker, no attachment icon, no camera | WA has emoji + attachment + camera icons |
| V14 | P2 | `message_bubble.html` | tool_use | Uses 🔧 emoji as icon — should be SVG per design standards | Use Lucide wrench SVG icon |
| V15 | P2 | `chat_view.html` | wallpaper | Plain `#ECE5DD` — WA has subtle doodle/pattern wallpaper | Optional: add subtle pattern bg |
| V16 | P2 | `chat_view.html` | message grouping | Each bubble has full margin — no grouping for consecutive same-role messages | WA reduces spacing between consecutive same-sender messages |
| V17 | P2 | `app.js` | WS reconnect | No reconnect logic — if WS drops, live updates stop silently | WA reconnects automatically w/ "Connecting..." banner |
| V18 | P1 | `chat_view.html` | input bar | No distinction between empty/filled state — send button always visible and green | WA: shows mic when empty, send arrow when text present |
| V19 | P2 | `message_bubble.html` | long content | `whitespace-pre-wrap` on full message — very long assistant responses will be huge bubbles. No collapse/expand | WA limits bubble size, but this is AI-specific need |
| V20 | P1 | `chat_view.html` | header status | Shows path as subtitle instead of "online"/"last seen" — path is dev info, not user-facing status | WA shows "online" / "last seen today at 3:42 PM" |

---

## Screen 4: Global / Cross-Cutting

| ID | Pri | File | Element | Gap | WA Reference |
|----|-----|------|---------|-----|-------------|
| G1 | P0 | `layout.html` | viewport | `maximum-scale=1` disables pinch-zoom — WCAG violation | Remove; use `touch-action: manipulation` for tap-delay instead |
| G2 | P0 | `layout.html` | safe areas | Zero safe-area handling anywhere — broken on all modern iPhones | Add `env(safe-area-inset-*)` to header, input bar, body |
| G3 | P0 | `layout.html` | font sizes | Base body font not set — Tailwind default is 16px ✓ but explicit input `text-sm` (14px) triggers iOS zoom | Set all input/textarea to `font-size: 16px` minimum |
| G4 | P1 | `layout.html` | PWA manifest | **Missing** — no `<link rel="manifest">`, no service worker, no icons | WA Web has full PWA manifest + offline support |
| G5 | P1 | `layout.html` | apple meta tags | Has `apple-mobile-web-app-capable` but missing `apple-mobile-web-app-status-bar-style` | Add `default` or `black-translucent` |
| G6 | P1 | all | loading states | **Zero loading indicators** — no skeleton, no spinner, no htmx indicator class | WA shows spinners on every async load |
| G7 | P1 | `layout.html` | color palette | User bubble `#25D366` ≠ WA sent bubble `#DCF8C6`. Current palette partially matches but key bubble color is wrong | Align: sent=`#DCF8C6` w/ dark text, received=`#FFFFFF`, bg=`#ECE5DD`, header=`#075E54` ✓ |
| G8 | P1 | `app.js` | error handling | WS `onerror`/`onclose` are empty no-ops — user gets no feedback if connection drops | WA shows "Connecting..." / "Phone not connected" banner |
| G9 | P2 | `layout.html` | htmx CDN | Loading htmx from unpkg CDN — no SRI hash, no fallback if CDN down | Vendor htmx.js locally like tailwind.min.css |
| G10 | P2 | `layout.html` | dark mode | **Not supported** — no `prefers-color-scheme` handling | WA supports dark mode (dark bg, adjusted bubble colors) |

---

## Priority Summary

### P0 — Broken on Mobile (fix immediately)

| Count | Issues |
|-------|--------|
| 7 | V1 (back arrow touch), V2 (input zoom), V3 (input safe-area), V4 (bubble contrast), C1 (icon touch targets), C11 (header safe-area), G1 (zoom disabled), G2 (safe areas), G3 (input font zoom) |

**Root causes:** (a) No safe-area-inset handling → content under notch/home-bar. (b) `text-sm` on inputs → iOS auto-zoom. (c) Touch targets under 44px. (d) User bubble contrast fails WCAG.

### P1 — Looks Off vs WhatsApp (high impact polish)

| Count | Issues |
|-------|--------|
| 18 | L1, L5, C2-C8, C12, V5-V6, V8-V13, V18, V20, G4-G8 |

**Key themes:** Missing unread badges, delivery ticks, date dividers, typing indicator, loading states, PWA manifest, WS reconnect, bubble color mismatch.

### P2 — Nice to Have

| Count | Issues |
|-------|--------|
| 13 | L2-L4, L6, C9-C10, C13, V7, V14-V17, V19, G9-G10 |

**Key themes:** Empty state illustrations, pull-to-refresh, dark mode, bubble tails, tool-use emoji→SVG, wallpaper pattern, message collapsing.

---

## Top 5 Recommendations (Ordered by Impact)

1. **Safe-area insets everywhere** — Add `env(safe-area-inset-*)` to header, input bar, body padding. Without this, app is unusable on iPhone X+.

2. **Fix input font size to 16px** — Change all `text-sm` on inputs to `text-base` (16px). Prevents iOS auto-zoom which breaks layout.

3. **Fix user bubble color** — Change from `#25D366` (neon green + white text, 2.1:1 contrast) to `#DCF8C6` (pastel green + dark text, ~7:1 contrast). Current is unreadable + fails WCAG.

4. **Expand all touch targets to ≥44px** — Back arrow, header icons, send button, search icons. Use padding/min-width/min-height, not just SVG size.

5. **Add loading/connection states** — htmx indicator class for polling, WS reconnect logic w/ "Connecting..." banner, submit button spinner.

---

## Color Palette Comparison

| Element | Current | WhatsApp | Match? |
|---------|---------|----------|--------|
| Header bg | `#075E54` | `#075E54` | ✅ |
| Sent bubble | `#25D366` | `#DCF8C6` | ❌ |
| Sent text | `#FFFFFF` | `#303030` | ❌ |
| Received bubble | `#FFFFFF` | `#FFFFFF` | ✅ |
| Received text | `text-gray-800` | `#303030` | ✅ |
| Chat bg | `#ECE5DD` | `#ECE5DD` | ✅ |
| Body bg | `#ECE5DD` | `#ECE5DD` | ✅ |
| Input bar bg | `bg-gray-100` | `#F0F0F0` | ~✅ |
| Status active | `bg-green-400/500` | N/A (online dot) | ✅ |
| Theme-color | `#075E54` | `#075E54` | ✅ |
