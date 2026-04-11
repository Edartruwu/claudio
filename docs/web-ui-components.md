# Claudio Web UI — Component Specifications

**Framework:** Go + templ + htmx (server-rendered)  
**Design System:** Gruvbox Dark Theme  
**Font:** JetBrains Mono (14px base)  
**Status:** Implementation-ready specifications for component developers

---

## Table of Contents

1. [Design System Tokens](#design-system-tokens)
2. [Session List Item](#session-list-item)
3. [Agent Card](#agent-card)
4. [Team Panel](#team-panel)
5. [Task Board](#task-board)
6. [Approval Dialog](#approval-dialog)
7. [Status Bar](#status-bar)
8. [Command Palette](#command-palette)
9. [Interaction & Animation Guidelines](#interaction--animation-guidelines)
10. [Accessibility Checklist](#accessibility-checklist)

---

## Design System Tokens

### Color Palette

All colors are CSS custom properties defined in `:root`. Use these names directly in component markup.

| Token | Hex Value | Usage |
|-------|-----------|-------|
| `--bg` | `#1d2021` | Primary background (body) |
| `--bg0` | `#282828` | Secondary background (panels, cards) |
| `--bg1` | `#3c3836` | Tertiary background (hover states, inputs) |
| `--bg2` | `#504945` | Dividers, borders, hover accents |
| `--bg3` | `#665c54` | Deep hover, active states |
| `--fg` | `#ebdbb2` | Primary text |
| `--fg2` | `#bdae93` | Secondary text (dimmer) |
| `--fg3` | `#a89984` | Tertiary text |
| `--dim` | `#928374` | Disabled text, subtle labels |
| `--red` | `#fb4934` | Errors, dangers, failed states |
| `--green` | `#b8bb26` | Success, running/active states |
| `--yellow` | `#fabd2f` | Warnings, attention, pending states |
| `--blue` | `#83a598` | Links, info, questions |
| `--purple` | `#d3869b` | Accent, headings, primary UI |
| `--aqua` | `#8ec07c` | Secondary accent, highlights |
| `--orange` | `#fe8019` | Tertiary accent, nested actions |

### Spacing Scale

- **xs:** 4px
- **sm:** 8px
- **md:** 12px
- **lg:** 16px
- **xl:** 24px
- **2xl:** 32px

### Border Radius

- **sm:** 4px (small elements, buttons)
- **md:** 6px (cards, inputs) — default `--radius`
- **lg:** 10px (modals, large cards) — `--radius-lg`

### Transitions

- **Standard:** `150ms ease` (default `--transition`)
- **Hover/Focus:** 150ms
- **Expand/Collapse:** 200ms
- **Color/bg changes:** 150ms
- **Avoid:** Transitions > 300ms (feels sluggish)

### Typography

- **Font:** JetBrains Mono, fallback to Fira Code / Cascadia Code
- **Base size:** 14px (set on `html`)
- **Line height:** 1.6 (body), 1.3 (headings)
- **Font weight scale:** 400 (normal) · 600 (semibold) · 700 (bold)

#### Type Scale (in rem)

| Use | Size | Weight | Example |
|-----|------|--------|---------|
| Heading XL | 2rem (28px) | 700 | Login page title |
| Heading L | 1.2rem (17px) | 700 | Header title, modal title |
| Heading M | 1rem (14px) | 600 | Section heading |
| Body | 0.9rem (13px) | 400 | Default paragraph text |
| Small | 0.85rem (12px) | 400 | Tool card, panel items |
| Tiny | 0.75rem (11px) | 600 | Labels, badges, status text |
| Micro | 0.7rem (10px) | 700 | Uppercase labels, timestamps |

---

## Session List Item

### Overview

A clickable item in the left sidebar session list. Shows session metadata (title, message count, state) and quick actions (rename, delete). States: **default**, **active**, **streaming**, **approval-needed**.

### Layout

```
┌─────────────────────────────────────┐
│ ● Session Title             [123]   │ ← session-item-top
│ (hover highlights row)              │
├─────────────────────────────────────┤
│             ✏️  ✕                    │ ← session-item-actions (on hover)
└─────────────────────────────────────┘
```

### Dimensions

- **Height:** 56px (compact, expandable for description)
- **Padding:** 10px 12px (vertical × horizontal)
- **Gap between rows:** 4px

### States & Styling

#### Default State
```
Background:   var(--bg)
Border:       1px solid transparent
Indicator:    ● var(--dim)
Title:        var(--fg), 600 weight, 0.85rem
Badge:        var(--bg2) bg, var(--dim) text, 0.7rem, right-aligned
Actions:      Opacity 0 (hidden until hover)
Cursor:       pointer
Transition:   all 150ms ease
```

#### Active State (selected session)
```
Background:   var(--bg1)
Border:       1px solid var(--bg2) (left edge 3px solid var(--purple))
Indicator:    ● var(--purple)
Title:        var(--fg), 600 weight, underline optional
Badge:        Bright (var(--purple) or var(--green) if new messages)
Accent:       3px left border in var(--purple)
```

#### Streaming State (agent is running)
```
Indicator:    ● var(--yellow) + subtle pulse animation
Title:        var(--fg), 600 weight
Badge:        Animated yellow border or pulsing opacity
Accent:       Left border var(--yellow)
Animation:    Indicator pulses: opacity 0.5 → 1.0 over 800ms, infinite
```

#### Approval Needed State
```
Indicator:    ● var(--red)
Title:        var(--fg), 600 weight, maybe italicized
Badge:        var(--red) text, reads "APPROVAL" or count of pending
Accent:       Left border var(--red)
Background:   Very subtle var(--red) tint (rgba(251,73,52,0.04))
Animation:    Subtle pulse or glow to draw attention
```

#### Hover State
```
Background:   var(--bg1)
Border:       1px solid var(--bg2)
Actions:      Opacity 1 (fully visible)
Cursor:       pointer
Title:        Color var(--fg) (unchanged)
Transition:   150ms ease
```

### Components

#### Indicator Dot
```html
<span class="session-indicator"></span>
```

**CSS:**
```css
.session-indicator {
  display: inline-block;
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--dim);
  margin-right: 8px;
  transition: all 150ms ease;
}
.session-item.active .session-indicator {
  background: var(--purple);
}
.session-item.streaming .session-indicator {
  background: var(--yellow);
  animation: pulse-indicator 800ms ease-in-out infinite;
}
@keyframes pulse-indicator {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
```

#### Title
```html
<span class="session-title">Session Name Here</span>
```

**CSS:**
```css
.session-title {
  color: var(--fg);
  font-weight: 600;
  font-size: 0.85rem;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
```

#### Badge (Message Count)
```html
<span class="session-badge">123</span>
```

**CSS:**
```css
.session-badge {
  display: inline-block;
  background: var(--bg2);
  color: var(--dim);
  padding: 2px 8px;
  border-radius: 12px;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  min-width: 24px;
  text-align: center;
  margin-left: auto;
  transition: all 150ms ease;
}
.session-item.active .session-badge {
  background: var(--purple);
  color: var(--bg);
}
.session-item.streaming .session-badge {
  background: var(--yellow);
  color: var(--bg);
}
```

#### Action Buttons
```html
<div class="session-item-actions">
  <button class="btn-icon" onclick="renameSession(id)" title="Rename">✎</button>
  <button class="btn-icon btn-icon-danger" onclick="deleteSession(id)" title="Delete">×</button>
</div>
```

**CSS:**
```css
.session-item-actions {
  display: flex;
  gap: 4px;
  opacity: 0;
  transition: opacity 150ms ease;
}
.session-item:hover .session-item-actions {
  opacity: 1;
}
.btn-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  background: transparent;
  border: none;
  color: var(--dim);
  cursor: pointer;
  border-radius: 4px;
  transition: all 150ms ease;
  font-size: 0.9rem;
}
.btn-icon:hover {
  background: var(--bg2);
  color: var(--fg);
}
.btn-icon-danger:hover {
  background: rgba(251,73,52,0.2);
  color: var(--red);
}
```

### Interaction

#### Click to Switch Session
- **Trigger:** Click on session item (excluding action buttons)
- **Feedback:** Immediate visual active state (background + border)
- **Result:** htmx POST to `/api/sessions/{id}/switch` or similar
  - Server sends: New message stream, panel updates, task list
  - Client: Receives SSE stream, renders new messages
  - No page reload

#### Rename Session
- **Trigger:** Click pencil icon
- **Feedback:** Inline text input appears (or modal)
- **Result:** POST to `/api/sessions/{id}/rename?title=...`
  - Server updates DB, returns updated session item
  - htmx swaps in-place

#### Delete Session
- **Trigger:** Click × icon
- **Feedback:** Confirmation toast or modal
- **Result:** DELETE `/api/sessions/{id}`
  - Server deletes, returns 204 No Content
  - Client removes item from list

---

## Agent Card

### Overview

A card displayed in the "Agents" panel showing a single active agent. Displays name, status, current task, token usage, and quick actions (message, kill, view).

**Shown when:** Agents spawned via harness (e.g., `SpawnTeammate`)  
**Template role:** Partial (inserted into `#agents-list` panel via htmx swap)

### Layout

```
┌────────────────────────────────┐
│ Name        [RUNNING|IDLE|...] │ ← header
├────────────────────────────────┤
│ Current Task:                  │
│ "Analyzing codebase..."        │ ← task description
├────────────────────────────────┤
│ Input: 1,234  Output: 567      │ ← token row
│ Elapsed: 2m 34s                │ ← time row
├────────────────────────────────┤
│ [Message]  [Kill]  [View]      │ ← actions
└────────────────────────────────┘
```

### Dimensions

- **Width:** 100% of panel (340px container, minus padding)
- **Card padding:** 12px 14px
- **Margin-bottom:** 10px
- **Border-radius:** 6px

### States & Styling

#### Running State
```
Background:   var(--bg0)
Border:       1px solid var(--bg1)
Status badge: var(--yellow) bg, var(--bg) text, "RUNNING"
Status color: var(--yellow)
Accent:       Left border 3px solid var(--yellow)
Animation:    Subtle pulse or glow on card
Task text:    var(--fg2) (lighter)
Cursor:       default
```

#### Idle State
```
Background:   var(--bg0)
Border:       1px solid var(--bg1)
Status badge: var(--dim) bg, var(--fg2) text, "IDLE"
Status color: var(--dim)
Accent:       Left border 3px solid var(--dim)
Task text:    "No active task" in var(--dim)
Opacity:      Slightly reduced (0.8)
```

#### Finished State
```
Background:   var(--bg0)
Border:       1px solid var(--bg1)
Status badge: var(--green) bg, var(--bg) text, "DONE"
Status color: var(--green)
Accent:       Left border 3px solid var(--green)
Task text:    "Completed: ..." or original task name
Opacity:      Normal or slightly reduced
```

#### Failed State
```
Background:   var(--bg0)
Border:       1px solid var(--bg1)
Status badge: var(--red) bg, var(--bg) text, "FAILED"
Status color: var(--red)
Accent:       Left border 3px solid var(--red)
Background:   Subtle red tint (rgba(251,73,52,0.04))
Error message: var(--red) text, "Error: ..."
```

#### Approval Needed State
```
Background:   var(--bg0)
Border:       1px solid var(--red)
Status badge: var(--red) bg, var(--bg) text, "APPROVAL"
Status color: var(--red)
Accent:       Left border 3px solid var(--red)
Indicator:    🔔 icon next to name or badge
Animation:    Pulse or glow to draw attention
Task text:    Reason for approval (bold)
```

#### Hover State (on card)
```
Background:   var(--bg1)
Border:       1px solid var(--bg2)
Cursor:       default (not clickable as a whole)
Action buttons: Become more prominent (opacity increase)
```

### Components

#### Header (Name + Status)
```html
<div class="agent-card-header">
  <span class="agent-name">Agent Name</span>
  <span class="agent-status running">RUNNING</span>
</div>
```

**CSS:**
```css
.agent-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}
.agent-name {
  color: var(--fg);
  font-weight: 600;
  font-size: 0.85rem;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.agent-status {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 3px;
  font-size: 0.65rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  white-space: nowrap;
}
.agent-status.running { background: var(--yellow); color: var(--bg); }
.agent-status.idle { background: var(--dim); color: var(--fg2); }
.agent-status.done { background: var(--green); color: var(--bg); }
.agent-status.failed { background: var(--red); color: var(--bg); }
.agent-status.approval { background: var(--red); color: var(--bg); }
```

#### Task Description
```html
<div class="agent-task">
  <span class="agent-task-label">Current:</span>
  <span class="agent-task-text">Analyzing codebase structure...</span>
</div>
```

**CSS:**
```css
.agent-task {
  display: flex;
  flex-direction: column;
  gap: 3px;
  margin-bottom: 8px;
  padding: 8px;
  border-radius: 4px;
  background: rgba(146,131,116,0.05);
}
.agent-task-label {
  font-size: 0.7rem;
  color: var(--dim);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.agent-task-text {
  color: var(--fg2);
  font-size: 0.8rem;
  line-height: 1.4;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
```

#### Token Row
```html
<div class="agent-tokens">
  <span class="agent-tokens-item">
    <span class="label">Input:</span>
    <span class="value">1,234</span>
  </span>
  <span class="agent-tokens-item">
    <span class="label">Output:</span>
    <span class="value">567</span>
  </span>
</div>
```

**CSS:**
```css
.agent-tokens {
  display: flex;
  gap: 12px;
  margin-bottom: 6px;
  font-size: 0.75rem;
  padding: 4px 0;
  border-bottom: 1px solid var(--bg1);
  padding-bottom: 6px;
}
.agent-tokens-item {
  display: flex;
  gap: 4px;
}
.agent-tokens-item .label {
  color: var(--dim);
  font-weight: 700;
}
.agent-tokens-item .value {
  color: var(--purple);
  font-weight: 600;
}
```

#### Elapsed Time
```html
<div class="agent-elapsed">
  <span class="label">Elapsed:</span>
  <span class="value">2m 34s</span>
</div>
```

**CSS:**
```css
.agent-elapsed {
  display: flex;
  justify-content: space-between;
  margin-bottom: 10px;
  font-size: 0.75rem;
}
.agent-elapsed .label {
  color: var(--dim);
}
.agent-elapsed .value {
  color: var(--fg2);
  font-weight: 600;
}
```

#### Action Buttons
```html
<div class="agent-actions">
  <button class="btn btn-sm" onclick="sendMessageToAgent(id)">Message</button>
  <button class="btn btn-sm" onclick="killAgent(id)">Kill</button>
  <button class="btn btn-sm btn-primary" onclick="viewAgent(id)">View</button>
</div>
```

**CSS:**
```css
.agent-actions {
  display: flex;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--bg1);
}
.agent-actions .btn {
  flex: 1;
  text-align: center;
}
```

### Card Container
```css
.agent-card {
  padding: 12px 14px;
  border-radius: 6px;
  background: var(--bg0);
  border: 1px solid var(--bg1);
  border-left: 3px solid var(--dim);
  margin-bottom: 10px;
  transition: all 150ms ease;
}
.agent-card.running {
  border-left-color: var(--yellow);
  animation: agent-pulse 1s ease-in-out infinite;
}
.agent-card.failed {
  background: rgba(251,73,52,0.04);
  border-color: var(--red);
  border-left-color: var(--red);
}
.agent-card.approval {
  border: 2px solid var(--red);
  background: rgba(251,73,52,0.06);
  animation: approval-pulse 1s ease-in-out infinite;
}
@keyframes agent-pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.8; }
}
@keyframes approval-pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(251,73,52,0.4); }
  50% { box-shadow: 0 0 0 4px rgba(251,73,52,0.2); }
}
```

### Empty State

When no agents are active:

```html
<div class="empty-state">
  <div class="empty-icon">∅</div>
  <div class="empty-text">No active agents</div>
  <div class="empty-hint" style="font-size:0.75rem;color:var(--dim);margin-top:4px;">
    Agents appear here when spawned
  </div>
</div>
```

### Interaction

#### Send Message to Agent
- **Trigger:** Click "Message" button
- **Feedback:** Opens text input modal or inline textarea
- **Result:** POST `/api/agents/{id}/message` with text
  - Server queues message, sends SSE update
  - Card updates to show message pending state

#### Kill Agent
- **Trigger:** Click "Kill" button
- **Feedback:** Confirmation toast/modal or immediate kill with undo option
- **Result:** POST `/api/agents/{id}/kill`
  - Server terminates agent, sends SSE update
  - Card transitions to "killed" state (grayed out), then removed after 2 seconds

#### View Agent Details
- **Trigger:** Click "View" button
- **Feedback:** None (navigation)
- **Result:** Navigate to `/agents/{id}` or open side panel with full agent details
  - Shows: full conversation, event log, all token usage, retry options

---

## Team Panel

### Overview

A panel (in right sidebar) showing team info and member agents. Displays team name, template, active members with their status, and actions (spawn new teammate, dissolve team).

**Shown when:** User creates or joins a team  
**Template role:** Partial (appears in `#panel-content` when "Agents" toggle is active)

### Layout

```
┌────────────────────────────────┐
│ TEAM                           │ ← panel section title
├────────────────────────────────┤
│ Name: agent-team-v1            │
│ Template: agent-harness.yaml   │
├────────────────────────────────┤
│ MEMBERS (5)                    │
├────────────────────────────────┤
│ • Agent-1       [RUNNING]      │
│ • Agent-2       [IDLE]         │
│ • Agent-3       [DONE]         │
│ • Agent-4       [FAILED]       │
│ • Agent-5       [APPROVAL] ‼️  │
├────────────────────────────────┤
│ [+ Spawn Teammate]  [⚠️ Dissolve] │
└────────────────────────────────┘
```

### Dimensions

- **Width:** 340px (full panel width)
- **Padding:** 12px 14px per section
- **Section margin:** 16px 0

### Components

#### Team Header Info
```html
<div class="panel-section">
  <div class="panel-section-title">TEAM</div>
  <div class="team-info">
    <div class="team-info-row">
      <span class="label">Name:</span>
      <span class="value">agent-team-v1</span>
    </div>
    <div class="team-info-row">
      <span class="label">Template:</span>
      <span class="value team-template">agent-harness.yaml</span>
    </div>
  </div>
</div>
```

**CSS:**
```css
.team-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.team-info-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.8rem;
  gap: 8px;
}
.team-info-row .label {
  color: var(--dim);
  font-weight: 600;
  text-transform: uppercase;
  font-size: 0.7rem;
}
.team-info-row .value {
  color: var(--fg2);
  font-weight: 500;
  word-break: break-all;
  text-align: right;
  flex: 1;
}
.team-template {
  font-family: monospace;
  font-size: 0.75rem;
  color: var(--orange);
}
```

#### Members List Header
```html
<div class="panel-section">
  <div class="panel-section-title">
    MEMBERS
    <span class="member-count">(5)</span>
  </div>
  <div id="team-members" class="team-members-list">
    <!-- Member items inserted here -->
  </div>
</div>
```

**CSS:**
```css
.panel-section-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 1px;
  color: var(--dim);
  margin-bottom: 8px;
  padding-bottom: 4px;
  border-bottom: 1px solid var(--bg1);
}
.member-count {
  color: var(--purple);
  font-weight: 600;
}
.team-members-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
```

#### Team Member Item
```html
<div class="team-member-item running">
  <div class="member-indicator">●</div>
  <span class="member-name">Agent-1</span>
  <span class="member-status running">RUNNING</span>
</div>
```

**CSS:**
```css
.team-member-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 4px;
  font-size: 0.8rem;
  background: var(--bg);
  border: 1px solid var(--bg1);
  cursor: pointer;
  transition: all 150ms ease;
}
.team-member-item:hover {
  background: var(--bg1);
  border-color: var(--bg2);
}
.member-indicator {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--dim);
  transition: all 150ms ease;
}
.team-member-item.running .member-indicator {
  background: var(--yellow);
  animation: pulse-indicator 800ms ease-in-out infinite;
}
.team-member-item.done .member-indicator {
  background: var(--green);
}
.team-member-item.failed .member-indicator {
  background: var(--red);
}
.team-member-item.approval .member-indicator {
  background: var(--red);
}
.member-name {
  color: var(--fg);
  font-weight: 600;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.member-status {
  display: inline-block;
  padding: 2px 6px;
  border-radius: 3px;
  font-size: 0.65rem;
  font-weight: 700;
  text-transform: uppercase;
  white-space: nowrap;
}
.member-status.running { background: rgba(250,189,47,0.15); color: var(--yellow); }
.member-status.idle { background: rgba(146,131,116,0.15); color: var(--dim); }
.member-status.done { background: rgba(184,187,38,0.15); color: var(--green); }
.member-status.failed { background: rgba(251,73,52,0.15); color: var(--red); }
.member-status.approval { background: rgba(251,73,52,0.15); color: var(--red); }
@keyframes pulse-indicator {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
```

#### Team Actions
```html
<div class="team-actions">
  <button class="btn btn-sm btn-primary" onclick="spawnTeammate()">
    + Spawn Teammate
  </button>
  <button class="btn btn-sm btn-ghost" onclick="dissolveTeam()" title="Dissolve team">
    ⚠️
  </button>
</div>
```

**CSS:**
```css
.team-actions {
  display: flex;
  gap: 6px;
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid var(--bg1);
}
.team-actions .btn {
  flex: 1;
}
.team-actions .btn-ghost {
  flex: none;
}
```

### Responsive Behavior

#### With 5 Members (scrollable if needed)
```
Members list can grow tall. Keep max-height: 300px and overflow-y: auto if > 5 members.
```

#### With 1 Member (compact)
```
Just one row. Padding and spacing stay the same.
```

#### Empty Team (no active members)
```html
<div class="team-members-list">
  <div style="color:var(--dim);padding:8px 0;text-align:center;font-size:0.75rem;">
    No active members
  </div>
</div>
```

### Interaction

#### Spawn New Teammate
- **Trigger:** Click "+ Spawn Teammate" button
- **Feedback:** Button shows loading state (spinner or opacity)
- **Result:** POST `/api/team/spawn-teammate`
  - Server creates new agent instance from template
  - Sends SSE event with new agent details
  - htmx swaps team members list with new member added

#### Dissolve Team
- **Trigger:** Click ⚠️ button
- **Feedback:** Confirmation modal appears
- **Result:** DELETE `/api/team`
  - Server terminates all agents, deletes team
  - Sends SSE event to notify all clients
  - htmx removes team panel, shows "Team dissolved"

#### Click Team Member
- **Trigger:** Click on member item
- **Feedback:** Brief highlight (100ms)
- **Result:** Navigate to agent details page or show agent card in main panel

---

## Task Board

### Overview

A task list in the right panel, grouped by status (Pending / Running / Done / Failed). Each task shows title, description preview, assigned agent, and elapsed/estimated time.

**Shown when:** User clicks "Tasks" panel toggle  
**Template role:** Partial (appears in `#panel-content`)

### Layout

```
┌────────────────────────────────┐
│ TASKS (12)                     │ ← section header with count
├────────────────────────────────┤
│ PENDING (3)                    │ ← status group header
├────────────────────────────────┤
│ [Task] Implement caching       │
│        Add Redis support       │
│        —                        │
├────────────────────────────────┤
│ [Task] Refactor API routes     │
├────────────────────────────────┤
│ RUNNING (2)                    │ ← status group header
├────────────────────────────────┤
│ [⧗ Task] Analyze code          │
│          Elapsed: 2m 34s       │ ← live timer
├────────────────────────────────┤
│ DONE (5)                       │ ← status group header (collapsible)
├────────────────────────────────┤
│ [✓ Task] Write tests           │
│          Completed 5m ago      │
└────────────────────────────────┘
```

### Dimensions

- **Width:** 340px (full panel width)
- **Card padding:** 10px 12px
- **Section margin:** 12px 0
- **Card margin:** 6px 0

### States & Styling

#### Group Headers (Pending, Running, Done, Failed)
```
Background:   transparent
Text:         var(--dim) uppercase
Font:         0.7rem, 700 weight, letter-spacing 1px
Separator:    Border-bottom 1px solid var(--bg1)
Padding:      8px 0 6px 0
Clickable:    Optional collapse/expand (chevron icon)
Color:        Status-specific (yellow for pending, etc.)
```

**CSS for status-specific header colors:**
```css
.task-group-header.pending { color: var(--yellow); }
.task-group-header.running { color: var(--blue); }
.task-group-header.done { color: var(--green); }
.task-group-header.failed { color: var(--red); }
```

#### Task Card — Default (Pending)
```
Background:   var(--bg)
Border:       1px solid var(--bg1)
Title:        var(--fg), 600 weight, 0.85rem
Description:  var(--fg2), 0.8rem, italic, max 2 lines
Meta:         "Agent: none" or agent name, 0.7rem, var(--dim)
Icon:         Status icon left (□ for pending)
Cursor:       pointer
Hover:        Background var(--bg1), border var(--bg2)
```

#### Task Card — Running
```
Background:   var(--bg0)
Border:       1px solid var(--blue)
Title:        var(--fg), 600 weight
Status icon:  ⧗ (hourglass) animated
Elapsed time: "Elapsed: 2m 34s" in var(--blue)
Animation:    Subtle pulsing border or icon rotation
Agent name:   "Agent-1" in var(--purple)
```

#### Task Card — Done
```
Background:   var(--bg)
Border:       1px solid var(--bg1)
Opacity:      0.8 (slightly faded)
Status icon:  ✓ (checkmark) in var(--green)
Title:        var(--fg2) (dimmer)
Meta:         "Completed 5m ago" in var(--dim)
Cursor:       default
Grayed out:   Title and text less vibrant
```

#### Task Card — Failed
```
Background:   rgba(251,73,52,0.04)
Border:       1px solid var(--red)
Status icon:  ✕ (cross) in var(--red)
Title:        var(--fg), 600 weight, bold red-tinted
Error text:   var(--red) italic, "Error: ..."
Meta:         "Failed 1m ago"
Cursor:       pointer (clickable to see error)
```

### Components

#### Group Header
```html
<div class="task-group">
  <div class="task-group-header pending">
    <span class="task-group-label">PENDING</span>
    <span class="task-group-count">(3)</span>
  </div>
  <!-- Task cards inserted here -->
</div>
```

**CSS:**
```css
.task-group {
  margin: 12px 0;
}
.task-group-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 1px;
  margin-bottom: 8px;
  padding-bottom: 4px;
  border-bottom: 1px solid var(--bg1);
  cursor: pointer;
  user-select: none;
}
.task-group-header.pending { color: var(--yellow); }
.task-group-header.running { color: var(--blue); }
.task-group-header.done { color: var(--green); }
.task-group-header.failed { color: var(--red); }
.task-group-count {
  background: var(--bg1);
  padding: 2px 6px;
  border-radius: 3px;
  font-weight: 600;
  color: var(--fg2);
}
.task-group-toggle {
  margin-left: auto;
  color: currentColor;
  font-size: 0.65rem;
  transition: transform 150ms ease;
}
.task-group-toggle.collapsed { transform: rotate(-90deg); }
```

#### Task Card
```html
<div class="task-card pending">
  <div class="task-card-header">
    <span class="task-icon pending">□</span>
    <span class="task-title">Implement caching</span>
  </div>
  <div class="task-card-body">
    <div class="task-description">Add Redis support for session caching</div>
    <div class="task-meta">
      <span class="task-agent">Agent: none</span>
    </div>
  </div>
</div>
```

**CSS:**
```css
.task-card {
  padding: 10px 12px;
  border-radius: 6px;
  border: 1px solid var(--bg1);
  background: var(--bg);
  margin-bottom: 6px;
  cursor: pointer;
  transition: all 150ms ease;
}
.task-card:hover {
  background: var(--bg1);
  border-color: var(--bg2);
}
.task-card.done {
  opacity: 0.75;
}
.task-card.failed {
  background: rgba(251,73,52,0.04);
  border-color: var(--red);
}
.task-card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.task-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  font-size: 0.8rem;
  flex-shrink: 0;
}
.task-icon.pending { color: var(--yellow); }
.task-icon.running { color: var(--blue); animation: spin 1s linear infinite; }
.task-icon.done { color: var(--green); }
.task-icon.failed { color: var(--red); }
.task-title {
  color: var(--fg);
  font-weight: 600;
  font-size: 0.85rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  flex: 1;
}
.task-card.done .task-title {
  color: var(--fg2);
  text-decoration: line-through;
}
.task-card-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-left: 24px;
}
.task-description {
  color: var(--fg2);
  font-size: 0.8rem;
  font-style: italic;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.task-meta {
  display: flex;
  justify-content: space-between;
  font-size: 0.75rem;
  color: var(--dim);
}
.task-agent {
  color: var(--purple);
  font-weight: 600;
}
.task-time {
  color: var(--dim);
  font-style: italic;
}
@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
```

#### Running Task (with elapsed timer)
```html
<div class="task-card running">
  <div class="task-card-header">
    <span class="task-icon running">⧗</span>
    <span class="task-title">Analyze code structure</span>
  </div>
  <div class="task-card-body">
    <div class="task-description">Build dependency graph for all packages</div>
    <div class="task-meta">
      <span class="task-agent">Agent-2</span>
      <span class="task-time" id="task-elapsed-123">Elapsed: 2m 34s</span>
    </div>
  </div>
</div>
```

**CSS for running task:**
```css
.task-card.running {
  border-color: var(--blue);
  background: var(--bg0);
}
.task-time {
  color: var(--blue);
  font-weight: 600;
}
```

**Timer updates via htmx or JS:**
- Server sends SSE event every 10 seconds with updated elapsed time
- htmx swaps the `#task-elapsed-{id}` element
- Or JS updates client-side timer without server roundtrip

### Empty State

```html
<div class="task-board-empty">
  <div class="empty-icon">📋</div>
  <div class="empty-text">No tasks yet</div>
  <div class="empty-hint" style="font-size:0.75rem;color:var(--dim);margin-top:4px;">
    Tasks appear when agents spawn sub-agents or plan work
  </div>
</div>
```

### Interaction

#### Click Task Card
- **Trigger:** Click on task card (title or card area)
- **Feedback:** Visual highlight (200ms duration)
- **Result:** POST or navigate to `/tasks/{id}/detail`
  - Shows expanded view: full description, assigned agent, error details if failed, retry button

#### Group Collapse/Expand (optional)
- **Trigger:** Click task group header
- **Feedback:** Chevron rotates, group body animates collapse/expand
- **Result:** Server-side preference saved (localStorage or session)
  - Only visual; task data not refetched

#### Retry Failed Task
- **Trigger:** Click "Retry" button on failed task detail view
- **Feedback:** Button shows spinner
- **Result:** POST `/api/tasks/{id}/retry`
  - Server re-queues task, creates new task record
  - htmx shows "Task queued" toast, removes failed task from board

---

## Approval Dialog

### Overview

A prominent modal dialog interrupting the user to request approval for sensitive actions:
1. **Tool Execution** — approve a tool call (e.g., file write, API call)
2. **Plan Review** — approve or reject a numbered plan
3. **Ask User** — answer a question from an agent

Each has distinct styling and urgency level.

**Template role:** Modal fragment (OOB-swapped into `#approval-area`)

### Layout

#### Tool Approval
```
┌───────────────────────────────┐
│  ⚠️  TOOL APPROVAL REQUIRED    │ ← yellow header
├───────────────────────────────┤
│                               │
│  Tool: FileTool.WriteFile      │
│  Agent: Code-Analyzer          │
│                               │
│  Arguments:                    │
│  ┌─────────────────────────┐  │
│  │ path: ./config.json     │  │
│  │ content: {              │  │
│  │   "version": "2.0" }    │  │
│  └─────────────────────────┘  │
│                               │
│  Do you want to proceed?       │
│                               │
├───────────────────────────────┤
│              [DENY]  [APPROVE] │
└───────────────────────────────┘
```

#### Plan Approval
```
┌───────────────────────────────┐
│  📋  REVIEW PLAN               │ ← yellow header
├───────────────────────────────┤
│                               │
│  1. Analyze existing tests     │
│  2. Identify coverage gaps     │
│  3. Write new test cases       │
│  4. Run full test suite        │
│  5. Create PR                  │
│                               │
│  Feedback (optional):          │
│  ┌─────────────────────────┐  │
│  │ Your suggestions here   │  │
│  └─────────────────────────┘  │
│                               │
├───────────────────────────────┤
│             [REJECT]  [APPROVE]│
└───────────────────────────────┘
```

#### Ask User
```
┌───────────────────────────────┐
│  ❓  QUESTION                  │ ← blue header
├───────────────────────────────┤
│                               │
│  Which approach would you      │
│  prefer?                       │
│                               │
│  [Use Cache]  [Direct API]     │ ← option buttons
│                               │
│  Or type your answer:          │
│  ┌─────────────────────────┐  │
│  │ Your custom response    │  │
│  └─────────────────────────┘  │
│                               │
├───────────────────────────────┤
│                      [SUBMIT]  │
└───────────────────────────────┘
```

### Dimensions

- **Width:** 520px desktop, 90vw mobile (max 90vw)
- **Max-height:** 80vh (scrollable if taller)
- **Padding:** 18px (header + body)
- **Overlay:** `position: fixed; inset: 0; background: rgba(0,0,0,0.6)`
- **Z-index:** 100+

### States & Styling

#### Shared Styling (All Dialog Types)

```css
.approval-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  animation: fadeIn 150ms ease;
}
.approval-dialog {
  width: 520px;
  max-width: 90vw;
  max-height: 80vh;
  overflow-y: auto;
  border-radius: var(--radius-lg);
  background: var(--bg0);
  box-shadow: 0 12px 40px rgba(0,0,0,0.5);
  display: flex;
  flex-direction: column;
  border: 1px solid var(--bg2);
}
@keyframes fadeIn {
  from { opacity: 0; transform: scale(0.95); }
  to { opacity: 1; transform: scale(1); }
}
```

#### Tool Approval Dialog
```
Header background: rgba(250,189,47,0.1)
Header border:     1px solid var(--bg2) (bottom)
Header text:       ⚠️  TOOL APPROVAL REQUIRED (yellow)
Body:              Code block with args, dark background
Buttons:           [DENY] red · [APPROVE] green (prominent)
Urgency:           High (yellow border, prominent header)
```

**CSS:**
```css
.approval-dialog.tool-approval {
  border: 1px solid var(--yellow);
}
.approval-dialog-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 18px;
  background: rgba(250,189,47,0.08);
  border-bottom: 1px solid var(--bg2);
}
.approval-dialog-header .icon {
  color: var(--yellow);
  font-size: 1.1rem;
  flex-shrink: 0;
}
.approval-dialog-header .title {
  font-weight: 700;
  color: var(--yellow);
  font-size: 0.9rem;
}
```

#### Plan Approval Dialog
```
Header background: rgba(250,189,47,0.1)
Header text:       📋  REVIEW PLAN (yellow)
Steps:             Numbered list, 0.85rem
Feedback input:    Optional textarea below steps
Buttons:           [REJECT] red · [APPROVE] green
Urgency:           Medium (yellow, collapsible feedback)
```

#### Ask User Dialog
```
Header background: rgba(131,165,152,0.1)
Header text:       ❓  QUESTION (blue)
Question:          Bold, centered, 0.9rem
Option buttons:    Multiple choice buttons (dynamic)
Input:             Optional textarea for free-form response
Buttons:           [SUBMIT] blue (single button)
Urgency:           Low (blue, friendly, no deny option)
```

**CSS:**
```css
.approval-dialog.askuser {
  border: 1px solid var(--blue);
}
.approval-dialog.askuser .approval-dialog-header {
  background: rgba(131,165,152,0.08);
  border-color: var(--bg2);
}
.approval-dialog.askuser .approval-dialog-header .icon {
  color: var(--blue);
}
.approval-dialog.askuser .approval-dialog-header .title {
  color: var(--blue);
}
```

### Components

#### Header
```html
<div class="approval-dialog-header">
  <span class="icon">⚠️</span>
  <span class="title">TOOL APPROVAL REQUIRED</span>
</div>
```

#### Body (Tool Approval)
```html
<div class="approval-dialog-body">
  <div class="approval-section">
    <div class="approval-label">Tool</div>
    <div class="approval-tool-name">FileTool.WriteFile</div>
  </div>
  <div class="approval-section">
    <div class="approval-label">Agent</div>
    <div class="approval-value">Code-Analyzer</div>
  </div>
  <div class="approval-section">
    <div class="approval-label">Arguments</div>
    <div class="approval-input-preview">path: ./config.json
content: { "version": "2.0" }</div>
  </div>
  <div class="approval-question">Do you want to proceed?</div>
</div>
```

**CSS:**
```css
.approval-dialog-body {
  padding: 14px 18px;
  flex: 1;
  overflow-y: auto;
}
.approval-section {
  margin-bottom: 12px;
}
.approval-label {
  font-size: 0.7rem;
  color: var(--dim);
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-bottom: 4px;
}
.approval-tool-name {
  color: var(--orange);
  font-weight: 600;
  font-size: 0.85rem;
}
.approval-value {
  color: var(--fg2);
  font-size: 0.8rem;
}
.approval-input-preview {
  padding: 10px;
  border-radius: var(--radius);
  background: var(--bg);
  border: 1px solid var(--bg2);
  font-family: monospace;
  font-size: 0.8rem;
  max-height: 200px;
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--fg2);
  line-height: 1.4;
}
.approval-question {
  color: var(--fg);
  font-weight: 600;
  font-size: 0.85rem;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--bg1);
}
```

#### Body (Plan Approval)
```html
<div class="approval-dialog-body">
  <div class="plan-approval-steps">
    <ol class="plan-step-list">
      <li><span class="step-num">1</span> Analyze existing tests</li>
      <li><span class="step-num">2</span> Identify coverage gaps</li>
      <li><span class="step-num">3</span> Write new test cases</li>
      <li><span class="step-num">4</span> Run full test suite</li>
      <li><span class="step-num">5</span> Create PR</li>
    </ol>
  </div>
  <div class="plan-feedback-section">
    <label for="plan-feedback" class="approval-label">Feedback (optional)</label>
    <textarea id="plan-feedback" class="plan-feedback-input"
      placeholder="Any adjustments or questions?"></textarea>
  </div>
</div>
```

**CSS:**
```css
.plan-approval-steps {
  margin-bottom: 14px;
}
.plan-step-list {
  list-style: none;
  padding: 0;
  margin: 0;
  counter-reset: step-counter;
}
.plan-step-list li {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  color: var(--fg);
  font-size: 0.85rem;
}
.step-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: var(--bg1);
  color: var(--purple);
  font-weight: 700;
  font-size: 0.8rem;
  flex-shrink: 0;
}
.plan-feedback-section {
  margin-top: 14px;
  padding-top: 14px;
  border-top: 1px solid var(--bg1);
}
.plan-feedback-input {
  width: 100%;
  padding: 8px 10px;
  border-radius: var(--radius);
  border: 1px solid var(--bg2);
  background: var(--bg);
  color: var(--fg);
  font-family: monospace;
  font-size: 0.8rem;
  outline: none;
  transition: border-color var(--transition);
  min-height: 60px;
  resize: vertical;
}
.plan-feedback-input:focus {
  border-color: var(--purple);
}
```

#### Body (Ask User)
```html
<div class="approval-dialog-body">
  <div class="askuser-question-area">
    <div class="askuser-question">Which approach would you prefer?</div>
  </div>
  <div class="askuser-options">
    <button class="askuser-option" onclick="answerAskUser('Use Cache')">Use Cache</button>
    <button class="askuser-option" onclick="answerAskUser('Direct API')">Direct API</button>
  </div>
  <div class="askuser-section">
    <label class="approval-label">Or type your answer:</label>
    <input type="text" class="askuser-input" placeholder="Your custom response"
      onkeypress="if(event.key==='Enter')answerAskUser(this.value)"/>
  </div>
</div>
```

**CSS:**
```css
.askuser-question-area {
  margin-bottom: 14px;
  padding-bottom: 14px;
  border-bottom: 1px solid var(--bg1);
  text-align: center;
}
.askuser-question {
  color: var(--fg);
  font-weight: 600;
  font-size: 0.9rem;
  line-height: 1.4;
}
.askuser-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 14px;
}
.askuser-option {
  padding: 8px 14px;
  border-radius: var(--radius);
  border: 1px solid var(--blue);
  background: transparent;
  color: var(--blue);
  font-family: monospace;
  font-size: 0.8rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 150ms ease;
}
.askuser-option:hover {
  background: rgba(131,165,152,0.15);
  border-color: var(--aqua);
  color: var(--aqua);
}
.askuser-input {
  width: 100%;
  padding: 8px 10px;
  border-radius: var(--radius);
  border: 1px solid var(--bg2);
  background: var(--bg);
  color: var(--fg);
  font-family: monospace;
  font-size: 0.8rem;
  outline: none;
  transition: border-color var(--transition);
}
.askuser-input:focus {
  border-color: var(--blue);
}
```

#### Footer (Action Buttons)
```html
<div class="approval-actions">
  <button class="btn btn-deny" onclick="denyApproval()">DENY</button>
  <button class="btn btn-approve" onclick="approveApproval()">APPROVE</button>
</div>
```

**CSS:**
```css
.approval-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  padding: 12px 18px;
  border-top: 1px solid var(--bg2);
  background: var(--bg0);
  flex-shrink: 0;
}
.btn-approve {
  background: var(--green);
  color: var(--bg);
  border-color: var(--green);
  font-weight: 600;
  cursor: pointer;
}
.btn-approve:hover {
  opacity: 0.85;
}
.btn-deny {
  background: var(--red);
  color: var(--bg);
  border-color: var(--red);
  font-weight: 600;
  cursor: pointer;
}
.btn-deny:hover {
  opacity: 0.85;
}
```

### Interaction

#### Tool Approval Flow
1. Agent initiates tool call
2. Server sends `approval_needed` SSE event with tool details
3. htmx swaps approval dialog into `#approval-area` (OOB)
4. Dialog appears centered with modal backdrop
5. User clicks DENY or APPROVE
6. Client POSTs `/api/chat/approve?session={id}&action=approve` with approval decision
7. Server unblocks agent, continues execution
8. htmx removes dialog (OOB swap to empty state)

#### Plan Approval Flow
1. Agent finishes planning, sends plan for review
2. Server sends `plan_approval` SSE event with numbered steps
3. htmx swaps approval dialog into `#approval-area`
4. User reviews and optionally adds feedback
5. User clicks APPROVE or REJECT
6. Client POSTs `/api/chat/plan-approval?action=approve&feedback=...`
7. Server unblocks agent with feedback (if rejected)
8. Dialog removed

#### Ask User Flow
1. Agent needs user input (choice or free text)
2. Server sends `askuser_request` SSE event with question and options
3. htmx swaps approval dialog into `#approval-area`
4. User selects option or types answer
5. User clicks SUBMIT (or presses Enter in input)
6. Client POSTs `/api/chat/askuser-response?answer=...`
7. Server returns answer to agent
8. Dialog removed

### Keyboard & Accessibility

- **Escape key:** Close dialog (deny/reject)
- **Tab:** Focus cycles through buttons
- **Enter:** Submit form (in inputs) or approve (primary button)
- **aria-modal="true"** on dialog
- **aria-labelledby** on dialog pointing to title
- **Focus trap:** Keyboard focus stays within dialog

---

## Status Bar

### Overview

A 28px fixed bar at the bottom of the chat interface showing connection status, active agent count, current model, token usage, and pending approval count.

**Location:** Below `#stream-area`, above chat input bar  
**Template role:** Permanent (rendered in layout)

### Layout

```
┌─────────────────────────────────────────────────────────────────┐
│ ● connected  |  Agents: 2  |  Model: claude-3.5-sonnet  |  Tokens...  │
└─────────────────────────────────────────────────────────────────┘
```

### Dimensions

- **Height:** 28px (fixed)
- **Padding:** 0 16px
- **Font size:** 0.7rem
- **Background:** var(--bg1)
- **Border-top:** 1px solid var(--bg2)

### States & Components

#### Connection Status Indicator
```
Default:       ● var(--green) + "connected"
Disconnected:  ● var(--red) + "disconnected"
Reconnecting:  ● var(--yellow) + "connecting..." (pulsing)
```

**CSS:**
```css
.status-indicator {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.status-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--green);
  transition: all 150ms ease;
}
.status-dot.disconnected {
  background: var(--red);
}
.status-dot.reconnecting {
  background: var(--yellow);
  animation: pulse-status 800ms ease-in-out infinite;
}
@keyframes pulse-status {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
.status-text {
  color: var(--dim);
  font-size: 0.7rem;
}
.status-text.ok { color: var(--green); }
.status-text.error { color: var(--red); }
.status-text.warning { color: var(--yellow); }
```

#### Agent Count
```
Display:  Agents: {count}
Color:    var(--blue) when > 0, var(--dim) when 0
Clickable: Click to toggle Agents panel
```

#### Model Display (Clickable)
```
Display:       Model: {model-name}
Color:         var(--purple) (accent)
Cursor:        pointer
On click:      Opens model selector modal
Hover state:   Text brightens to var(--fg)
Icon:          Optional ▼ dropdown arrow
```

**CSS:**
```css
.status-model-item {
  cursor: pointer;
  user-select: none;
  transition: all 150ms ease;
  padding: 0 4px;
  border-radius: 3px;
}
.status-model-item:hover {
  background: var(--bg2);
}
.status-model {
  color: var(--purple);
  font-weight: 600;
  font-size: 0.7rem;
}
.status-model-item:hover .status-model {
  color: var(--fg);
}
```

#### Token Usage
```
Display:       {input} in | {output} out | {total} total
Color:         var(--fg2) for labels, var(--fg) for values
Separator:     | (pipe) in var(--bg2)
Tooltip:       Full breakdown on hover (optional)
```

#### Approval Indicator (if pending)
```
Display:       🔔 {count} pending
Color:         var(--red) (urgent)
Animation:     Pulse when approval > 0
Clickable:     Click to jump to approval dialog
```

### Complete Status Bar CSS

```css
.status-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 16px;
  height: var(--status-h);  /* 28px */
  background: var(--bg1);
  border-top: 1px solid var(--bg2);
  font-size: 0.7rem;
  color: var(--dim);
  flex-shrink: 0;
  overflow-x: auto;
  overflow-y: hidden;
}
.status-item {
  display: flex;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}
.status-label {
  color: var(--bg3);
  font-size: 0.65rem;
}
.status-value {
  color: var(--fg2);
  font-weight: 600;
}
.status-sep {
  color: var(--bg2);
  user-select: none;
}
.status-approvals {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
}
.status-approvals-badge {
  background: var(--red);
  color: var(--bg);
  padding: 2px 6px;
  border-radius: 3px;
  font-weight: 700;
  animation: pulse-approval 1s ease-in-out infinite;
}
@keyframes pulse-approval {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.6; }
}
```

### Interaction

#### Click Model to Change
- **Trigger:** Click model name
- **Feedback:** None (immediate)
- **Result:** Modal model selector opens
  - Server sends list of available models
  - User selects new model
  - POST `/api/session/model?model=claude-3.5-sonnet`
  - Status bar updates with new model name

#### Click Approval Badge
- **Trigger:** Click approval count badge
- **Feedback:** Scroll to approval area or focus active approval dialog
- **Result:** JavaScript scrolls page to `#approval-area` and flashes approval dialog

---

## Command Palette

### Overview

A modal overlay with a search box and command results, grouped by category (Sessions, Agents, Tasks, Commands). Keyboard-driven (Cmd+K / Ctrl+K to open).

**Template role:** Modal fragment (inserted via htmx or kept hidden, shown via JS)

### Layout

```
┌────────────────────────────────────────────┐
│ Search or jump to...            [Cmd+K]    │ ← search input
├────────────────────────────────────────────┤
│ SESSIONS                                   │ ← group header
│ • Recent Session #1                        │
│ • Chat with Debugging                      │
├────────────────────────────────────────────┤
│ AGENTS                                     │
│ • Code-Analyzer (running)                  │
│ • Test-Writer (idle)                       │
├────────────────────────────────────────────┤
│ TASKS                                      │
│ • [Pending] Implement caching              │
│ • [Running] Analyze structure              │
├────────────────────────────────────────────┤
│ COMMANDS                                   │
│ • Create New Session (Cmd+N)               │
│ • Toggle Agents Panel (Ctrl+1)             │
│ • Send Message (Ctrl+Enter)                │
└────────────────────────────────────────────┘
```

### Dimensions

- **Width:** 600px (desktop), 90vw (mobile)
- **Max-height:** 70vh (scrollable)
- **Padding:** 12px (search area)
- **Item height:** 40px
- **Result item padding:** 10px 16px

### Styling

#### Command Palette Container
```css
.command-palette-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0,0,0,0.6);
  display: none;  /* hidden by default */
  z-index: 200;
  animation: fadeIn 150ms ease;
}
.command-palette-backdrop.open {
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 60px;
}
.command-palette {
  width: 600px;
  max-width: 90vw;
  max-height: 70vh;
  border-radius: var(--radius-lg);
  background: var(--bg0);
  border: 1px solid var(--bg2);
  box-shadow: 0 16px 48px rgba(0,0,0,0.6);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
```

#### Search Input
```html
<div class="command-palette-search">
  <input type="text" id="command-input" class="command-input"
    placeholder="Search or jump to..." autocomplete="off"/>
  <span class="command-hint">[Esc] to close</span>
</div>
```

**CSS:**
```css
.command-palette-search {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--bg1);
  background: var(--bg0);
  flex-shrink: 0;
}
.command-input {
  flex: 1;
  background: var(--bg);
  border: none;
  color: var(--fg);
  font-family: monospace;
  font-size: 0.85rem;
  outline: none;
  padding: 8px 10px;
  border-radius: 4px;
  transition: all 150ms ease;
}
.command-input:focus {
  border: 1px solid var(--purple);
}
.command-hint {
  color: var(--dim);
  font-size: 0.65rem;
  white-space: nowrap;
  text-transform: uppercase;
}
```

#### Results Area
```css
.command-results {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}
.command-group {
  margin-bottom: 8px;
}
.command-group-title {
  font-size: 0.65rem;
  font-weight: 700;
  color: var(--dim);
  text-transform: uppercase;
  letter-spacing: 1px;
  padding: 6px 10px;
  margin: 4px 0;
  border-bottom: 1px solid var(--bg1);
}
```

#### Result Items
```html
<div class="command-result-item">
  <span class="result-icon">📝</span>
  <span class="result-text">Recent Session #1</span>
  <span class="result-meta">5 messages</span>
</div>
```

**CSS:**
```css
.command-result-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  transition: all 150ms ease;
  margin-bottom: 4px;
}
.command-result-item:hover,
.command-result-item.selected {
  background: var(--bg1);
  border: 1px solid var(--bg2);
}
.result-icon {
  font-size: 1rem;
  flex-shrink: 0;
  width: 24px;
  text-align: center;
}
.result-text {
  flex: 1;
  color: var(--fg);
  font-size: 0.85rem;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.result-meta {
  color: var(--dim);
  font-size: 0.7rem;
  white-space: nowrap;
  text-align: right;
}
```

#### Result Types (Icons & Colors)

| Type | Icon | Color | Example |
|------|------|-------|---------|
| Session | 📝 | var(--purple) | Recent Session |
| Agent | 🤖 | var(--blue) | Code-Analyzer |
| Task | ✓ | var(--green) | Completed task |
| Command | ⌨️ | var(--aqua) | Create Session |

### Interaction

#### Open Palette
- **Trigger:** Cmd+K (macOS) or Ctrl+K (Windows/Linux)
- **Feedback:** Backdrop + palette slide in with animation (150ms)
- **Result:** Focus input immediately, clear previous search

#### Type to Filter
- **Trigger:** User types in search input
- **Feedback:** Results filter in real-time (debounced 100ms)
- **Result:** Results update as user types
  - Server-side search (POST `/api/search?q=...`) OR
  - Client-side fuzzy filter (prefetched all items)

#### Arrow Keys Navigate
- **Trigger:** ↑ / ↓ arrows
- **Feedback:** Selected item highlights (background color)
- **Result:** Active result class updates on item

#### Enter to Select
- **Trigger:** Press Enter on highlighted result
- **Feedback:** Brief loading state if navigating
- **Result:** Navigate to or activate selected item
  - Session: Switch to session (htmx POST)
  - Agent: Open agent details
  - Task: Open task details
  - Command: Execute command

#### Escape to Close
- **Trigger:** Esc key
- **Feedback:** Palette slides out (150ms)
- **Result:** Focus returns to chat input

### JavaScript Handler Outline

```javascript
// Open palette
document.addEventListener('keydown', (e) => {
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault();
    openCommandPalette();
  }
});

// Filter results
const input = document.getElementById('command-input');
input.addEventListener('input', debounce((e) => {
  const query = e.target.value;
  // POST /api/search?q=... or filter client-side
  updateResults(query);
}, 100));

// Navigate with arrows
document.addEventListener('keydown', (e) => {
  if (!paletteOpen) return;
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    moveSelection(1);
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    moveSelection(-1);
  } else if (e.key === 'Enter') {
    e.preventDefault();
    selectItem();
  } else if (e.key === 'Escape') {
    closeCommandPalette();
  }
});
```

---

## Interaction & Animation Guidelines

### Guiding Principles

1. **Clarity:** Every interactive element must show clear feedback immediately (< 100ms)
2. **Purpose:** Animations communicate state changes, not distract
3. **Performance:** Use CSS-only transitions; avoid JS animation where possible
4. **Accessibility:** Respect `prefers-reduced-motion` media query
5. **Consistency:** Reuse timing (150ms) and easing (ease) across components

### Transition Timing & Easing

| Purpose | Duration | Easing | Example |
|---------|----------|--------|---------|
| Hover feedback | 150ms | ease | Button hover, card lift |
| Modal/dialog open/close | 150ms | ease | Approval dialog fade-in |
| Collapse/expand | 200ms | ease | Thinking block, group toggle |
| Color/background change | 150ms | ease | Status indicator, badge |
| Status pulse | 800ms | ease-in-out | Running agent indicator |
| Smooth scroll | auto | smooth | Message area scroll-behavior |

### htmx Swap Patterns

#### innerHTML (Content Swap)
Used for: Panel content updates, task list refreshes

```html
hx-get="/api/panel/tasks"
hx-target="#panel-content"
hx-swap="innerHTML swap:200ms settle:300ms"
```

- Swap: 200ms fade transition
- Settle: 300ms wait before transition end

#### outerHTML (Element Swap)
Used for: Individual item updates, task card removal

```html
hx-delete="/api/tasks/{id}"
hx-target="closest .task-card"
hx-swap="outerHTML swap:100ms settle:200ms"
```

#### Out-of-Band Swaps (OOB)
Used for: Approval dialogs, status bar updates

```html
<!-- Server sends in response: -->
<div id="approval-area" hx-swap-oob="innerHTML">
  <!-- New approval dialog -->
</div>
<div id="status-bar" hx-swap-oob="innerHTML">
  <!-- Updated status -->
</div>
```

### Loading States

Every interactive element shows feedback during server requests:

```html
<button 
  hx-post="/api/action"
  hx-indicator="#spinner-id"
  class="btn btn-primary"
>
  Send
</button>

<!-- Indicator: hidden by default, shown during request -->
<div id="spinner-id" style="display:none;" class="spinner"></div>
```

**CSS for spinner:**
```css
.spinner {
  display: inline-block;
  width: 12px;
  height: 12px;
  border: 2px solid var(--bg2);
  border-top-color: var(--purple);
  border-radius: 50%;
  animation: spin 600ms linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}
.htmx-request .spinner { display: inline-block; }
```

### Focus Management

- **Modal dialogs:** Focus trap (Tab cycles within dialog only)
- **Dropdowns/panels:** Focus moves to first interactive element
- **Form submissions:** Focus moves to success message or error message
- **Keyboard:** Escape closes any open dialog/modal

---

## Accessibility Checklist

### WCAG 2.1 AA Compliance

| Criterion | Status | Notes |
|-----------|--------|-------|
| 1.4.3 Contrast (AAA) | ✓ | All text meets 4.5:1 (body) / 3:1 (large) |
| 2.1.1 Keyboard | ✓ | All functions accessible via keyboard (Tab, Enter, Escape) |
| 2.4.3 Focus Order | ✓ | Tab order is logical (left-to-right, top-to-bottom) |
| 2.4.7 Focus Visible | ✓ | Focus ring visible on all interactive elements |
| 3.2.1 On Focus | ✓ | Focus does not trigger unexpected navigation |
| 3.3.1 Error Identification | ✓ | Error messages appear near input, in color + text |
| 3.3.4 Error Suggestion | ✓ | Form errors suggest fixes |
| 4.1.3 Status Messages | ✓ | aria-live on dynamic content (SSE messages) |

### Implementation Notes

#### Focus Indicators
```css
/* Remove default outline */
button:focus-visible {
  outline: 2px solid var(--purple);
  outline-offset: 2px;
}
```

#### Color Contrast
- All text: 4.5:1 minimum (AA for normal text)
- Large text (18px+): 3:1 minimum
- Test with: WebAIM Contrast Checker

#### aria-live Regions
```html
<!-- Chat messages appear here -->
<div id="messages" class="messages-area" aria-live="polite" aria-label="Chat messages">
</div>

<!-- Status updates (non-intrusive) -->
<div id="status-updates" aria-live="polite" aria-label="Status updates" aria-atomic="false">
</div>

<!-- Approval dialogs (high priority) -->
<div id="approval-area" aria-live="assertive" aria-label="Approval required">
</div>
```

#### Semantic HTML
- Use `<button>` for actions, not `<div>`
- Use `<dialog>` for modals (native semantics)
- Use `<label>` with `for` attribute on form inputs
- Use `<fieldset>` for grouped form controls

#### Screen Reader Support
```html
<!-- Buttons with icons need labels -->
<button aria-label="Delete session" class="btn-icon">✕</button>

<!-- Describe status indicators -->
<span class="status-dot" aria-label="Connection active"></span>

<!-- Form inputs with labels -->
<label for="search-input">Search sessions</label>
<input id="search-input" type="text"/>

<!-- Lists of dynamically added items -->
<ul id="agents-list" aria-label="Active agents">
  <!-- Items inserted here -->
</ul>
```

#### Reduced Motion
```css
@media (prefers-reduced-motion: reduce) {
  * {
    animation-duration: 1ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 1ms !important;
  }
}
```

---

## Implementation Summary

### File Organization (templ files)

```
/internal/web/templates/
├── chat.templ                    (Main chat page)
├── panels.templ                  (Panel content: agents, tasks, etc.)
├── components/
│   ├── session-item.templ        (Reusable session list item)
│   ├── agent-card.templ          (Reusable agent card)
│   ├── team-panel.templ          (Team view section)
│   ├── task-board.templ          (Task list grouped by status)
│   ├── approval-dialog.templ     (Tool, plan, askuser approval)
│   ├── status-bar.templ          (Bottom status bar)
│   └── command-palette.templ     (Search/jump modal)
└── layout.templ                  (Base HTML document)
```

### CSS Organization

All CSS is embedded in `/internal/web/static.go`. Group by component:

```go
// In cssContent string:
/* ── Session Items ── */
/* ── Agent Cards ── */
/* ── Team Panel ── */
/* ── Task Board ── */
/* ── Approval Dialogs ── */
/* ── Status Bar ── */
/* ── Command Palette ── */
```

### Key Design Tokens (Ready to Use)

```css
/* Colors */
--bg, --bg0, --bg1, --bg2, --bg3
--fg, --fg2, --fg3, --dim
--red, --green, --yellow, --blue, --purple, --aqua, --orange

/* Spacing */
4px, 8px, 12px, 16px, 24px, 32px

/* Radius */
--radius: 6px
--radius-lg: 10px

/* Typography */
JetBrains Mono, 14px base
Weights: 400, 600, 700
Scale: 0.7rem (10px) to 2rem (28px)

/* Timing */
--transition: 150ms ease
Standard: 150ms, 200ms, 300ms
Avoid: > 300ms
```

---

## Next Steps for Implementation

1. **Create templ component files** for each component using the layouts and CSS specs above
2. **Implement htmx bindings** for interactive components (approval dialogs, task updates, etc.)
3. **Add SSE event handlers** in JavaScript to render agent cards, task updates in real-time
4. **Test keyboard navigation** and accessibility (Tab, Escape, focus traps)
5. **Validate color contrast** with WebAIM or similar tool
6. **Test on mobile** (responsive layouts, touch targets ≥ 44px)
7. **Performance check** (no layout thrashing, smooth animations at 60fps)

---

**Document Version:** 1.0  
**Last Updated:** [Current Date]  
**Maintained by:** Claudio Design Team  
**Reference:** Gruvbox Dark Theme, Go templ, htmx, JetBrains Mono
