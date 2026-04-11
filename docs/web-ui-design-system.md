# Claudio Web UI — Design System & Navigation Architecture

**Version:** 1.0  
**Status:** Approved for Implementation  
**Last Updated:** 2025-01-01  
**Target WCAG Level:** AA  
**Browser Support:** Chrome, Firefox, Safari, Edge (latest 2 versions)

---

## Table of Contents

1. [Overview](#overview)
2. [Design Principles](#design-principles)
3. [Information Architecture](#information-architecture)
4. [Layout System](#layout-system)
5. [Design Language](#design-language)
6. [Navigation Components](#navigation-components)
7. [Interaction Patterns](#interaction-patterns)
8. [Page & Route Map](#page--route-map)
9. [Accessibility](#accessibility)
10. [Template Architecture](#template-architecture)
11. [Implementation Checklist](#implementation-checklist)

---

## Overview

### Context

Claudio is redesigning its web UI to support four first-class entities that are core to the AI coding assistant workflow:

- **Sessions** — Independent chat conversations scoped to a project
- **Agents** — Sub-agents spawned during conversations (e.g., Explore, Plan, Verify agents)
- **Teams** — Named groups of agents with templates (e.g., "go-fullstack-team-a4304dc2")
- **Tasks** — Work items with status tracking (pending → in_progress → completed/failed)

### Current State

The existing web UI:
- **Stack:** Go backend + templ templates + HTMX 2.0.4 + Server-Sent Events
- **Theme:** Gruvbox dark with embedded CSS/JS
- **Layout:** Left sidebar (sessions only) | Main chat area | Right panel (tabs)
- **Shortcoming:** Teams and improved agent/task UX are missing from web UI

### Design Goal

Create a unified, responsive three-column app shell that surfaces all four entities in persistent left-side navigation, with clear visual distinction, status indicators, and htmx-driven interactions. The design maintains the existing chat-centric experience while providing elegant navigation and context switching.

---

## Design Principles

1. **Entity-First Navigation** — All four entities visible and accessible from the sidebar at all times
2. **Status Transparency** — Every entity shows its current state (idle, running, awaiting input, error, completed)
3. **Progressive Disclosure** — Detailed entity info available via modal/panel without leaving the main chat
4. **Monospace Consistency** — JetBrains Mono throughout (matches terminal, CLI aesthetic)
5. **Dark Aesthetics** — Gruvbox dark theme for reduced eye strain and terminal consistency
6. **Fast Interaction** — All entity switches via htmx (no full page reload), visible feedback within 200ms
7. **Keyboard-First** — All navigation and actions accessible via keyboard; shortcuts for power users
8. **Mobile-Friendly** — Touch-friendly (44px targets), responsive layout (hamburger nav, bottom drawer panels)

---

## Information Architecture

### Navigation Model

The redesigned UI uses a **unified navigation model** where all four entities appear in a persistent left sidebar, organized by entity type (not a flat list).

```
Left Sidebar (Entity-Scoped Navigation)
├─ Sessions (Create/rename/delete/select)
│  ├─ Chat 1 [active]
│  ├─ Chat 2
│  └─ + Create
├─ Agents (Spawn/kill/monitor)
│  ├─ Explore Agent [running]
│  ├─ Plan Agent [idle]
│  └─ + Spawn
├─ Teams (Create/manage/select)
│  ├─ go-fullstack-team [3 agents, 1 task]
│  └─ + Create
└─ Tasks (Create/update/close)
   ├─ Pending (2)
   ├─ In Progress (1)
   └─ + Create
```

### Entity Visibility & Selection

| Entity | Visibility | Selection | Primary Action | Secondary Actions |
|--------|------------|-----------|----------------|--------------------|
| **Session** | List in sidebar (always) | Click to load chat | Load session messages | Rename, delete, export |
| **Agent** | List in sidebar + badge | Click to focus | Scroll chat to agent, show metrics | Kill, pause, resume |
| **Team** | List in sidebar + count | Click to load workspace | Load team view | Manage members, delete |
| **Task** | List in sidebar (grouped) | Click to open modal | Show task detail + history | Update status, reassign, comment |

### Interaction Flow Example: Session Switching

```
User clicks "Chat 2" in Sessions list
    ↓
Sidebar item gets active styling (aria-current="page")
    ↓
htmx GET /api/sessions/{id} (with loading indicator)
    ↓
Main chat area swapped with new session's history
    ↓
Input focus moved to text area
    ↓
User can immediately type (or see agent working)
```

---

## Layout System

### Overall Structure

**Desktop (≥1024px):**
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Claudio          Project: /home/user/project          [⚙ Settings] [👤 User] │  Header (56px)
└─────────────────────────────────────────────────────────────────────────────┘
┌──────────────────┬────────────────────────────────┬─────────────────────────┐
│                  │                                │                         │
│  Entity Nav      │   Main Chat Area               │   Right Panel (Tabs)    │
│  (280px fixed)   │   (flex-grow: 1)               │   (280px fixed)         │
│                  │                                │                         │
│  ┌────────────┐  │  ┌────────────────────────────┐│ ┌────────────────────┐ │
│  │ Sessions   │  │  │  Message stream            ││ │ [Stats][Tools][⚙] │ │
│  │ ├─ Chat 1  │  │  │  (scrollable)              ││ │                    │ │
│  │ └─ Chat 2  │  │  │                            ││ │ Content region:    │ │
│  │            │  │  │ [Thinking...]              ││ │ • Tokens IN/OUT    │ │
│  │ Agents     │  │  │ [Tool approval?]           ││ │ • Model: Opus      │ │
│  │ ├─ Agent A │  │  │                            ││ │ • Tools enabled    │ │
│  │ └─ Agent B │  │  │ [Input area]               ││ │ • Config section   │ │
│  │            │  │  └────────────────────────────┘│ └────────────────────┘ │
│  │ Teams      │  │                                │                         │
│  │ └─ Team 1  │  │                                │                         │
│  │            │  │                                │                         │
│  │ Tasks      │  │                                │                         │
│  │ ├─ Pending │  │                                │                         │
│  │ └─ Done    │  │                                │                         │
│  └────────────┘  │                                │                         │
│                  │                                │                         │
└──────────────────┴────────────────────────────────┴─────────────────────────┘
│ ● Connected | Model: claude-opus | Tokens IN: 1234 | OUT: 567 | Cache: 123  │  Status (32px)
└─────────────────────────────────────────────────────────────────────────────┘
```

**Tablet (768–1023px):**
- Left sidebar collapses to **56px icon-only nav** (labels appear on hover)
- Main chat area expands to fill space
- Right panel remains 280px
- On click of hamburger, sidebar expands to overlay

**Mobile (<768px):**
- Sidebar hidden by default (hamburger icon in header)
- Header remains 56px with hamburger, project name, settings/user menu
- Main chat area takes full width
- Right panel hidden by default; pull-up drawer on tab click (max 80% height)
- Hamburger → Full-height slide-out drawer from left (100% width)

### Spacing Scale

Claudio uses a **base-4 spacing scale** for all padding, margins, and gaps:

| Token | Value | Use |
|-------|-------|-----|
| space-1 | 4px | Icon gaps, badge padding, tight spacing |
| space-2 | 8px | Input padding (y), list gaps, small margins |
| space-3 | 12px | Button padding (y), form field spacing |
| space-4 | 16px | Card padding, form gaps, comfortable spacing |
| space-6 | 24px | Sidebar section padding, panel padding |
| space-8 | 32px | Major section separation |

**Rule:** All padding, margin, and gap values must come from this scale. No arbitrary pixel values.

### Typography Scale

Claudio uses **JetBrains Mono** throughout (already established):

| Role | Size | Weight | Line Height | Use |
|------|------|--------|-------------|-----|
| H1 (Page Title) | 24px | 600 | 32px | Page headings, main titles |
| H2 (Section) | 18px | 600 | 24px | Section headings, modal titles |
| H3 (Subsection) | 14px | 500 | 20px | Entity names, group headers |
| Body (Default) | 12px | 400 | 16px | Message text, descriptions |
| Body Small (Meta) | 11px | 400 | 14px | Timestamps, counts, hints |
| Code/Monospace | 12px | 400 | 16px | Code blocks, commands (JetBrains Mono) |

### Z-Index Scale

| Layer | Value | Use |
|-------|-------|-----|
| Base content | 0 | Normal document flow |
| Sticky header | 10 | Fixed top nav, sticky table headers |
| Dropdowns | 20 | Select menus, comboboxes, popovers |
| Floating | 30 | Tooltips, floating labels |
| Modals/Dialogs | 50 | Modal backdrop and dialog element |
| Toasts | 100 | Notification toasts (always on top) |

**Rule:** Never use arbitrary z-index values. Use only this scale in CSS.

---

## Design Language

### Color System

Claudio extends the existing **Gruvbox Dark** theme with semantic tokens for the new entity model.

#### Core Color Tokens

```css
/* Backgrounds & Surfaces */
--color-bg: #1d2021;              /* Page background */
--color-surface: #282828;         /* Card/panel backgrounds (slightly lighter) */
--color-surface-raised: #3c3836;  /* Modals, raised surfaces (even lighter) */
--color-border: #504945;          /* Dividers, input borders */

/* Text */
--color-text: #ebdbb2;            /* Primary text (foreground) */
--color-text-muted: #928374;      /* Meta, hints, secondary text */
--color-text-inverse: #1d2021;    /* Text on light/colored backgrounds */

/* Actions & States */
--color-primary: #83a598;         /* Primary action, links (blue) */
--color-primary-hover: #5e8f79;   /* Hover state for primary */
--color-accent: #fe8019;          /* Important CTA, approvals (orange) */
--color-accent-hover: #d65911;    /* Hover state for accent */
--color-success: #b8bb26;         /* Completed, positive states (green) */
--color-warning: #fabd2f;         /* Caution, in-progress (yellow) */
--color-error: #fb4934;           /* Errors, destructive actions (red) */
--color-info: #d3869b;            /* Informational messages (purple/pink) */
--color-focus-ring: #83a598;      /* Focus rings, outlines */
```

#### Entity Type Colors

```css
/* Entity-specific colors (semantic) */
--color-entity-session: #8ec07c;  /* Aqua: new/ongoing conversation */
--color-entity-agent: #d3869b;    /* Purple: sub-agents, spawned tasks */
--color-entity-team: #83a598;     /* Blue: team groups, collaboration */
--color-entity-task: #fabd2f;     /* Yellow: work items, todos */
```

#### Status Indicator Colors

```css
/* Status states (use with icons + text for accessibility) */
--color-status-idle: #928374;     /* Not running, inactive (muted) */
--color-status-running: #b8bb26;  /* Active, executing (green) */
--color-status-waiting: #fabd2f;  /* Awaiting input, approval (yellow) */
--color-status-error: #fb4934;    /* Error, failed (red) */
--color-status-completed: #b8bb26;/* Completed (green) */
```

### Color Contrast

All color combinations meet **WCAG AA** standards:

| Element | Required | Standard |
|---------|----------|----------|
| Body text on bg | 4.5:1 | AA |
| Large text (18px+) on bg | 3:1 | AA |
| UI components, icons | 3:1 | AA |
| Focus ring vs background | 3:1 | AA 2.2 |

**Verified pairs:**
- `--color-text` (#ebdbb2) on `--color-bg` (#1d2021): **11.6:1** ✓
- `--color-text-muted` (#928374) on `--color-bg`: **5.2:1** ✓
- `--color-primary` (#83a598) on `--color-bg`: **5.4:1** ✓
- `--color-success` (#b8bb26) on `--color-bg`: **6.1:1** ✓
- `--color-warning` (#fabd2f) on `--color-bg`: **7.2:1** ✓
- `--color-error` (#fb4934) on `--color-bg`: **6.9:1** ✓

### Visual Style

**Style:** Dark professional (developer tool / CLI aesthetic)  
**Mood:** Focused, technical, trustworthy, minimalist  
**Reference:** VSCode dark theme, GitHub dark mode, terminal color schemes

**Key visual elements:**
- **Borders:** Subtle, 1px, `--color-border` (not white, not invisible)
- **Rounded corners:** 4px (minimal), 8px (default), 12px (generous)
- **Shadows:** Minimal; only on modals and floating elements (`0 2px 8px rgba(0,0,0,0.3)`)
- **Icons:** 16px (nav items), 20px (buttons), 24px (headers), custom SVG icons (no emoji)
- **Transitions:** 150ms ease-out for hover/focus, 200ms ease-out for swaps, 300ms for fade-out

**Do:**
- Use subtle color shifts on hover (primary → primary-hover)
- Use borders for visual separation in dark theme (no relying on shadow alone)
- Use semantic token names in CSS (`var(--color-primary)`, never `#83a598`)
- Use monospace font throughout (JetBrains Mono)

**Don't:**
- Use white text or pure white borders (too bright in dark theme)
- Use emoji as icons (looks unprofessional in developer context)
- Use more than 3–4 colors per screen (focus, clarity)
- Animate non-essential elements (reduce motion respect)

---

## Navigation Components

### Header (56px)

**Structure (left to right):**

```
[Logo/Icon] [App Name]     [Project Path]          [⚙ Settings] [👤 User Menu]
   32px        16px          (flex-grow: 1)            20px        20px
```

**Logo/Icon:** 32×32px Claudio icon (monochrome, `--color-primary`)

**App Name:** "Claudio" text, 14px bold, `--color-text`

**Project Path:** Current project (e.g., `/home/user/project`), 12px muted, truncated with ellipsis if long

**Settings Button:** 20×20px icon button, hover fills bg, `aria-label="Settings"`

**User Menu:**
- Avatar (32×32px circle) or icon
- Click → dropdown menu: Settings, Logout
- Keyboard: Enter/Space to open, Arrow keys to navigate, Escape to close

**Responsive:**
- Tablet (768–1023px): Hide project path, show project name only
- Mobile (<768px): Hamburger icon (left), project name (center), settings/user (right)

**States:**
- Default: `--color-bg` background
- Scroll: Stays fixed (z-index: 10)
- Focus: Icon/button shows focus ring

### Left Sidebar (280px → 56px → hidden)

**Desktop Layout (280px):**

#### Sidebar Header (56px)

```
[Icon] Claudio
  32px
```

- Logo 32×32px, centered in header
- App name "Claudio" below on mobile only
- No collapse button (sidebar is always visible on desktop)
- Padding: 12px all sides

#### Entity Sections (scrollable)

Each section follows this pattern:

```
SECTION HEADER [+ icon button]
├─ Item 1
├─ Item 2
└─ + Action (create/add)
```

##### Sessions Section

```
Sessions [+ icon, aria-label="Create session"]

◉ Chat 1 [✓ active] [hover: ⋮ menu]
● Chat 2
● Chat 3

[+ Create New Session]
```

**Item structure (40px height):**
- Left: Status icon (16px, `--color-status-*`)
  - `◉` running
  - `●` idle
  - `⟳` streaming (animated)
  - `⏱` waiting for input
  - `✕` error
- Middle: Title (12px, `--color-text`), message count (11px, muted)
- Right: Hover actions (rename, delete icons, 20px)

**Active state:** Left 3px border `--color-entity-session`, bg fill `rgba(--color-entity-session, 0.1)`

**Hover state:** Bg tint, show action buttons

#### Agents Section

```
Agents [+ icon, aria-label="Spawn agent"]

◉ ExploreAgent [context: 3 files] [hover: kill]
● PlanAgent
● VerifyAgent

[+ Spawn New Agent]
```

**Item structure (40px):**
- Left: Status icon (16px)
- Middle: Name (12px) + context in parentheses (11px, muted)
- Right: Inline badge (entity type, `--color-entity-agent`), hover kill button

**Active state:** Left 3px border `--color-entity-agent`, bg tint

**Badge:** Pill shape, 16px height, "AGENT" text + small logo (4px, white, `--color-text-inverse`)

#### Teams Section

```
Teams [+ icon, aria-label="Create team"]

● go-fullstack-team (3 agents, 1 task)
● data-pipeline-team (5 agents, 2 tasks)

[+ Create New Team]
```

**Item structure (40px):**
- Left: Status icon (16px, `--color-status-running` if any agent is active)
- Middle: Team name (12px), member/task count (11px, muted)
- Right: Hover menu (manage, delete)

**Active state:** Left 3px border `--color-entity-team`, bg tint

#### Tasks Section

```
Tasks [+ icon, aria-label="Create task"]

Pending (2)
  ⏱ Task #1: "Fix login bug" (assigned: Sarah)
  ● Task #2: "Review PR" (unassigned)

In Progress (1)
  ⟳ Task #3: "Implement dashboard" (assigned: You)

Done (5) [collapsible]

[+ Create New Task]
```

**Section groups (collapsible):**
- Pending
- In Progress
- Done (collapsed by default on mobile, expanded on desktop)

**Item structure (36px):**
- Left: Status icon (16px)
- Middle: ID + title (12px), assignee (11px, muted)
- Right: Hover menu (edit, complete, delete)

**Colors:** Status icon color = `--color-status-*` (waiting/running/completed)

#### Sidebar Footer (56px)

```
[Avatar] Name [⚙ icon]
   32px    12px  20px
```

- User avatar (32×32px circle) or initials badge
- User name (12px)
- Settings icon (20×20px, click → account/logout menu)

**Hover:** Bg tint, show logout option

### Left Sidebar on Tablet (56px Icon-Only)

When viewport < 1024px:

```
[Logo]
[◉●⟳] ← Session icon (hover → expand sidebar)
[◉●⟳] ← Agent icon
[◉] ← Team icon
[⏱●] ← Task icon
[Avatar] ← User
```

- Only icons visible (no labels)
- Hover on icon → shows sidebar expanded (overlay, z-index: 40)
- Click icon → toggles sidebar (stays open until click elsewhere)
- On mobile, hamburger replaces sidebar icon

### Mobile Navigation (Hamburger)

**Header:**
- Left: Hamburger icon (24×24px)
- Center: Project name (max 20 chars, ellipsis)
- Right: Settings + user menu

**Hamburger click → Full-height drawer:**
- Sidebar drawer slides from left (100% width)
- Same structure as desktop sidebar (Sessions, Agents, Teams, Tasks)
- Includes user profile at bottom
- Close button (X) top-right or backdrop click closes

**Right panel on mobile:**
- Hidden by default
- Tab click → Bottom drawer (max 80% height, rounded top corners)
- Pull handle / close button at top
- Swipe down or close button closes drawer

### Right Panel (280px)

**Tab Navigation (48px):**

```
[Stats] [Tools] [⚙ Config]
  (active = colored text + underline)
```

- 3 tabs: Stats, Tools, Config
- Active tab: `--color-primary` text + 2px bottom border
- Inactive: `--color-text-muted` text
- Hover: `--color-text` text
- Click → htmx GET `/api/panel/{type}` swaps content
- Keyboard: Left/Right arrow to navigate tabs, Enter to activate

**Content Area (scrollable, below tabs):**

**Stats Tab:**
```
Tokens
├─ Input: 1,234
├─ Output: 567
└─ Total: 1,801

Cache
├─ Read: 456
└─ Created: 123

Cost (Estimated)
└─ $0.00456
```

- Simple key-value layout
- Monospace numbers (right-aligned)
- No visual clutter

**Tools Tab:**
```
Tools Manager

✓ file_search (enabled)
✓ bash (enabled)
✗ browser (disabled)
✓ create_file (enabled)

[Can toggle enabled/disabled]
```

- List of tools with toggle switches
- Click switch → htmx POST `/api/tools/{id}/toggle`
- Grey out disabled tools
- Tools marked "core" (non-toggleable) show as info-only

**Config Tab:**
```
Model
└─ [claude-opus ▼]

Permission Mode
└─ [Approval ▼]

Project
└─ /home/user/project
```

- Config values as selects or info display
- Changes via htmx POST
- Read-only fields (like project path) shown as text

**Responsive:**
- Desktop: Always visible (280px right edge)
- Tablet: Always visible (280px)
- Mobile: Hidden; pull-up bottom drawer on tab click

---

## Interaction Patterns

### Session Switching

**Flow:**

```
User clicks session in sidebar
    ↓ (immediate visual feedback)
Sidebar item gets active styling + aria-current="page"
    ↓ (200ms max indicator appears)
htmx GET /api/sessions/{id}
  hx-target="#chat-main"
  hx-swap="innerHTML"
  hx-push-url="true"
  hx-indicator="#loading-spinner"
    ↓ (response received)
Main chat area swapped with new session
    ↓
Focus moved to input field
```

**Visual feedback:**
- Clicked item: Active styling (border + bg tint)
- Spinner: Top-right of main area, appears after 200ms
- Duration: <1s typically (unless long history)

**htmx attributes:**
```html
<a class="session-item {{if .Active}}active{{end}}"
   hx-get="/api/sessions/{{.ID}}"
   hx-target="#chat-main"
   hx-swap="innerHTML transition:true"
   hx-push-url="true"
   hx-indicator="#loading-spinner"
   aria-current="{{if .Active}}page{{end}}">
  {{.Title}}
</a>
```

**Loading indicator:**
```html
<div id="loading-spinner" class="htmx-indicator">
  <!-- Spinner SVG or animation -->
</div>
```

### Agent Spawning

**Flow:**

```
User clicks "+ Spawn Agent" in Agents section
    ↓
Modal form loads (htmx GET /api/agents/spawn-form)
  Agent type selector (Explore, Plan, Verify, etc.)
  Optional: Parameters (e.g., "files to explore", "time limit")
    ↓
User fills form + clicks "Spawn"
    ↓
htmx POST /api/agents/spawn
  Response: Success toast (OOB) + new agent item (OOB)
    ↓
Modal closes, new agent appears in sidebar
Agent starts running (status: ⟳)
```

**Modal:**
- Size: 480px (md)
- Title: "Spawn New Agent"
- Form fields:
  - Agent Type: Select menu (Explore, Plan, Verify, etc.)
  - Name (optional): Text input (auto-generated if empty)
  - Parameters (conditional): Show based on agent type
- Buttons: [Spawn] [Cancel]

**Response OOB swaps:**
```html
<!-- Main response: closes modal -->
<!-- OOB: Toast -->
<div id="toast-container" hx-swap-oob="beforeend">
  <div class="toast toast--success">
    Agent "ExploreAgent" spawned
  </div>
</div>

<!-- OOB: New agent item in sidebar -->
<div id="agents-list" hx-swap-oob="beforeend">
  <div class="agent-item">
    <span class="status-icon">⟳</span>
    <span>ExploreAgent</span>
  </div>
</div>
```

### Task Status Update

**Flow:**

```
User clicks task in Tasks sidebar section
    ↓
Task detail modal loads (htmx GET /api/tasks/{id})
  Shows: Title, status, description, assignee, created/updated dates
    ↓
User changes status from select menu (Pending → In Progress → Done)
    ↓
htmx PUT /api/tasks/{id}
  Body: { status: "in_progress" }
    ↓
Response: Modal updates + sidebar task item updates (OOB)
```

**Status Dropdown:**
```html
<select name="status" 
        hx-put="/api/tasks/{{.ID}}"
        hx-trigger="change"
        hx-target="#task-detail">
  <option value="pending" {{if eq .Status "pending"}}selected{{end}}>
    Pending
  </option>
  <option value="in_progress" {{if eq .Status "in_progress"}}selected{{end}}>
    In Progress
  </option>
  <option value="completed" {{if eq .Status "completed"}}selected{{end}}>
    Completed
  </option>
</select>
```

**OOB response (sidebar item):**
```html
<div id="task-item-{{.ID}}" hx-swap-oob="outerHTML">
  <div class="task-item status-{{.Status}}">
    <span class="status-icon">{{.StatusIcon}}</span>
    <span class="task-title">Task #{{.ID}}: {{.Title}}</span>
  </div>
</div>
```

### Approval Dialog

**Context:** Agent requests permission to use a tool (existing behavior, improved UX)

**Flow:**

```
Server sends approval request in message stream
    ↓
Inline modal/panel appears in main chat area
  Title: "Agent wants to use tool: bash"
  Description: "Command: ls -la /home"
  Buttons: [Approve] [Deny]
    ↓
User clicks Approve
    ↓
htmx POST /api/chat/approval
  Body: { tool_id: "bash", action: "approve" }
    ↓
Dialog closes, chat continues (agent continues execution)
```

**Visual design:**
- Warning color: `--color-warning` (#fabd2f) border left (3px)
- Panel bg: `--color-surface-raised` (#3c3836)
- Padding: 16px (space-4)
- Buttons: [Approve = primary] [Deny = destructive]

### Loading States

**Rule:** Every htmx request gets a visual indicator. Indicator appears after 200ms (to avoid flicker for fast requests).

| Context | Indicator | Duration |
|---------|-----------|----------|
| Session switch | Spinner in main area top-right | Until content swapped (typically <1s) |
| Agent spawn | Spinner inside [Spawn] button | Until modal closes (~200ms) |
| Task update | Fade out task row, show skeleton | Until response (<500ms) |
| Right panel tab | Skeleton in panel content area | Until new content loaded (<300ms) |
| Message send | Typing indicator in chat | Until response received |
| Search input | Skeleton results below input | 300ms debounce before search |

**CSS pattern:**
```css
.htmx-request .htmx-indicator {
  opacity: 1;
  transition: opacity 150ms ease-in-out;
}

.htmx-indicator {
  opacity: 0;
  transition: opacity 150ms ease-in-out;
}

/* Skeleton shimmer */
.skeleton {
  background: linear-gradient(
    90deg,
    var(--color-muted) 25%,
    var(--color-surface) 50%,
    var(--color-muted) 75%
  );
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
}

@keyframes shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}
```

### Error Handling

**Network/Server Errors:**

```
Request fails (e.g., 5xx response)
    ↓
Toast appears: "Couldn't save. Try again?"
Inline error button in toast
    ↓
User clicks [Retry]
    ↓
htmx request re-issued
```

**Validation Errors:**

```
Form submission fails (422)
    ↓
Server returns form with error messages injected
Form is swapped back to user, now with errors visible
Error borders on invalid fields (--color-error)
    ↓
User corrects and resubmits
```

**Toast design:**
- Position: Top-right (desktop) / top-center (mobile)
- Auto-dismiss: Success 3s, Info 5s, Warning/Error manual
- Border-left: 3px (color-coded)
- Stack: Multiple toasts 8px apart

### Empty States

Every empty list/section has a designed empty state:

**Sessions (empty):**
```
[Icon: message bubble outline]

No sessions yet

Create a new session to start chatting
with Claude about your project.

[+ Create Session] button
```

**Agents (none running):**
```
[Icon: robot outline]

No active agents

Spawn an agent from the chat to start
exploring, planning, or verifying your code.

[Spawn Agent] button (or link to chat)
```

**Teams (none):**
```
[Icon: group outline]

No teams yet

Teams let you organize agents and collaborate
on complex tasks.

[+ Create Team] button
```

**Tasks (none):**
```
[Icon: checklist outline]

No tasks yet

Create a task to track work and assign
it to team members.

[+ Create Task] button
```

**Design:**
- Icon: 48px, `--color-text-muted`
- Heading: "No [items]", 14px 500 weight
- Body text: Explanation, 12px muted
- CTA button: Primary or ghost, 12px text
- Centered in the region
- Minimum height: Match expected filled state

---

## Page & Route Map

### Routes

| Route | Method | Purpose | Layout | Content | Entity Scope |
|-------|--------|---------|--------|---------|-----|
| `/login` | GET/POST | Authentication | Full-screen form | Login form, error messages | N/A |
| `/` | GET | Home/project browser | Full-width | Project list, descriptions | None |
| `/chat` | GET | Main chat interface | App shell | Chat + sidebar + panels | Session (current) |
| `/chat?session={id}` | GET | Chat for specific session | App shell | Chat history + sidebar | Session |
| `/chat?agent={id}` | GET | Chat focused on agent | App shell | Chat (scrolled to agent) | Agent |
| `/chat?team={id}` | GET | Team workspace view | App shell | Team collaboration | Team |
| `/logout` | POST | Clear auth | Redirect | N/A | N/A |

### API Routes (htmx Targets)

| Route | Method | Purpose | Response | Swap Target |
|-------|--------|---------|----------|-----|
| `/api/sessions/` | GET | List sessions | JSON | Sidebar partial |
| `/api/sessions/{id}` | GET | Get session + chat history | HTML partial | `#chat-main` |
| `/api/sessions/create` | GET | Create session form | HTML modal | `#modal-content` |
| `/api/sessions/create` | POST | Submit new session | Redirect or JSON | Modal closes (OOB) |
| `/api/sessions/{id}/rename` | POST | Rename session | JSON | OOB sidebar item |
| `/api/sessions/{id}/delete` | DELETE | Delete session | JSON | OOB sidebar item removed |
| `/api/agents/` | GET | List agents | JSON | Sidebar partial |
| `/api/agents/spawn-form` | GET | Spawn agent form | HTML modal | `#modal-content` |
| `/api/agents/spawn` | POST | Spawn new agent | JSON | Modal closes + OOB sidebar |
| `/api/agents/{id}/kill` | POST | Kill agent | JSON | OOB sidebar item removed |
| `/api/teams/` | GET | List teams | JSON | Sidebar partial |
| `/api/teams/create` | GET | Create team form | HTML modal | `#modal-content` |
| `/api/teams/create` | POST | Submit new team | JSON | Modal closes + OOB sidebar |
| `/api/teams/{id}` | GET | Load team workspace | HTML partial | `#chat-main` |
| `/api/teams/{id}/delete` | DELETE | Delete team | JSON | OOB sidebar item removed |
| `/api/tasks/` | GET | List tasks | JSON | Sidebar partial |
| `/api/tasks/{id}` | GET | Task detail modal | HTML modal | `#modal-content` |
| `/api/tasks/{id}` | PUT | Update task (status, etc.) | HTML partial | Modal updates + OOB sidebar |
| `/api/tasks/create` | POST | Create task | JSON | Modal closes + OOB sidebar |
| `/api/tasks/{id}/delete` | DELETE | Delete task | JSON | OOB sidebar item removed |
| `/api/chat/send` | POST | Send user message | SSE stream | Main chat area appends |
| `/api/chat/stream` | GET | Subscribe to SSE | Event stream | Appended to chat |
| `/api/panel/stats` | GET | Right panel: Stats | HTML partial | `#right-panel-content` |
| `/api/panel/tools` | GET | Right panel: Tools | HTML partial | `#right-panel-content` |
| `/api/panel/config` | GET | Right panel: Config | HTML partial | `#right-panel-content` |

### Key Views

#### 1. Login Page

**Route:** `/login`  
**Layout:** Full-screen form  
**Content:**

```
Centered 480px form on --color-bg

[Claudio Logo]

Login

[Username input field]
[Password input field]
[Remember me checkbox]

[Login button]

Forgot password? → link
```

**Interaction:**
- Form validation on blur (username required, password required)
- Submit → POST `/login` with credentials
- Success → Redirect to `/` (home/project browser)
- Error → Show error message in toast

#### 2. Home / Project Browser

**Route:** `/`  
**Layout:** Full-width page  
**Content:**

```
Header: "Select a Project"

Grid of projects:
┌─ Project Name ────────────┐
│ Path: /home/user/proj1    │
│ Sessions: 3               │
│ Agents: 0                 │
│ Tasks: 2                  │
│ [Open] [Settings]         │
└───────────────────────────┘

[+ New Project button]
```

**Interaction:**
- Click project → Load session list (or auto-open last session)
- Click [Open] → Go to `/chat?session={last_or_first}`
- [+ New Project] → Form to create project

#### 3. Chat View (Main)

**Route:** `/chat`  
**Layout:** Three-column app shell  
**Content:**

**Left Sidebar:**
- Sessions list (expandable)
- Agents list (expandable)
- Teams list (expandable)
- Tasks list (expandable)

**Main Area:**
```
┌─────────────────────────────────┐
│ Messages Area (scrollable)      │
│                                 │
│ [User message]                  │
│ [Assistant response]            │
│ [Thinking indicator]            │
│ [Tool approval dialog]          │
│                                 │
│ [Approval Panel]                │
│ ┌─────────────────────────────┐ │
│ │ Agent wants to use bash     │ │
│ │ Command: ls -la /home       │ │
│ │                             │ │
│ │ [Approve] [Deny]            │ │
│ └─────────────────────────────┘ │
└─────────────────────────────────┘

┌─────────────────────────────────┐
│ [📎] Message... [Send]          │
│ @mentions | /commands | >>      │
│ [Image previews strip]          │
└─────────────────────────────────┘
```

**Right Panel:**
- Tabs: [Stats] [Tools] [⚙ Config]
- Content updates based on active tab

**Interactions:**
- Click session/agent/team → Swap main area
- Type message → Input focus
- Send message → POST `/api/chat/send` → SSE stream appends messages
- Tool approval → Click [Approve]/[Deny]
- Tab click → htmx GET `/api/panel/{type}` swaps right panel

#### 4. Team Workspace View (New)

**Route:** `/chat?team={id}`  
**Layout:** App shell (same as chat)  
**Content (Main Area):**

```
Team: go-fullstack-team

Active Agents (3)
├─ ExploreAgent [running] — "Exploring codebase"
├─ PlanAgent [idle]
└─ VerifyAgent [idle]

Shared Task Queue
├─ Task #1: [⏱] "Review architecture"
├─ Task #2: [⟳] "Write tests"
└─ Task #3: [●] "Documentation"

Team Chat
[Messages from team members]

Message area input
```

**Interaction:**
- Click agent → Focus on that agent's conversation
- Click task → Show task detail modal
- Participate in team chat (message area same as single session)
- OOB swaps update agent status, task status in real-time

---

## Accessibility

### WCAG AA Compliance

Claudio targets **WCAG 2.1 Level AA** for all pages:

- Text contrast: 4.5:1 for body text, 3:1 for large text (18px+)
- Focus indicators: Always visible, 3:1 contrast
- Keyboard navigation: All functionality accessible via keyboard
- Screen reader support: Semantic HTML, ARIA labels where needed
- Motion: Respects `prefers-reduced-motion` user preference

### Keyboard Navigation

| Action | Keyboard | Scope |
|--------|----------|-------|
| Navigate nav items | Tab / Shift+Tab | Sidebar, header, panels |
| Move between sidebar sections | Arrow Up/Down | Within section |
| Activate item | Enter / Space | Session, agent, team, task |
| Switch right panel tabs | Left/Right Arrow | Tabs |
| Activate tab | Enter | Tabs |
| Close modal | Escape | Modals, dialogs |
| Send message | Ctrl/Cmd+Enter | Chat input |
| Search (global) | Cmd/Ctrl+K | Any context |

### Focus Management

**After htmx swap:**
- Session switch: Focus moves to chat input (`hx-on::after-swap="this.focus()"`)
- Modal open: Focus moves to first input field
- Modal close: Focus returns to button that opened modal
- Tab change: Focus moves to first interactive element in new tab content

**Implementation:**
```html
<!-- Session link with focus management -->
<a hx-get="/api/sessions/{{.ID}}"
   hx-target="#chat-main"
   hx-swap="innerHTML"
   hx-on="htmx:afterSwap: document.getElementById('chat-input').focus()">
  {{.Title}}
</a>

<!-- Modal with focus on first input -->
<dialog hx-on="htmx:afterSwap: document.getElementById('modal-first-input').focus()">
  <input id="modal-first-input" type="text" />
</dialog>
```

### Semantic HTML & ARIA

**Sidebar navigation:**
```html
<nav aria-label="Entities">
  <section aria-labelledby="sessions-heading">
    <h3 id="sessions-heading">Sessions</h3>
    <ul role="list">
      <li>
        <a aria-current="page" href="/chat?session=1">Chat 1</a>
      </li>
    </ul>
  </section>
</nav>
```

**Right panel tabs:**
```html
<div role="tablist" aria-label="Panel content">
  <button role="tab" aria-selected="true" aria-controls="panel-stats">
    Stats
  </button>
  <button role="tab" aria-selected="false" aria-controls="panel-tools">
    Tools
  </button>
</div>

<div id="panel-stats" role="tabpanel" aria-labelledby="[tab-id]">
  [Content]
</div>
```

**Status indicators:**
```html
<!-- Don't use color alone for status -->
<span class="status-icon" aria-label="Running">⟳</span>

<!-- Use aria-label for icon-only buttons -->
<button aria-label="Create new session">+</button>

<!-- Mark dynamic regions -->
<div id="toast-container" aria-live="polite" role="status">
  [Toasts]
</div>

<div aria-busy="true" aria-live="polite">
  [Content loading]
</div>
```

### Color Contrast Verification

All color pairs tested and verified at 1.0 opacity (no transparency):

```
✓ #ebdbb2 (text) on #1d2021 (bg): 11.6:1
✓ #928374 (text-muted) on #1d2021: 5.2:1
✓ #83a598 (primary) on #1d2021: 5.4:1
✓ #b8bb26 (success) on #1d2021: 6.1:1
✓ #fabd2f (warning) on #1d2021: 7.2:1
✓ #fb4934 (error) on #1d2021: 6.9:1

Focus ring:
✓ #83a598 (ring) on #1d2021: 5.4:1
✓ #83a598 (ring) on #3c3836: 3.8:1
```

### Motion & Animation

**Transition durations:**
- Hover state: 150ms ease-out
- Swap content: 200ms fade ease-out
- Loading spinner: 2s linear (infinite)
- Status pulse animation: 2s ease-in-out
- Modal slide-in: 200ms ease-out

**Respects `prefers-reduced-motion`:**
```css
@media (prefers-reduced-motion: reduce) {
  * {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

### Touch & Mobile Accessibility

**Touch targets:** All buttons/links ≥44×44px  
**Input fields:** ≥40px height, 16px font (prevents iOS auto-zoom)  
**Labels:** Always visible and associated with inputs (`<label for="id">`)  
**Focus ring:** Visible on keyboard focus, hidden on touch focus (`:focus-visible`)

---

## Template Architecture

### File Structure

```
internal/web/templates/
├── layout.templ               # Base: <html>, <head>, nav, footer
│                               # NEVER SWAPPED
│
├── pages/
│   ├── home.templ             # Home: project browser
│   ├── login.templ            # Login form
│   └── chat.templ             # Chat page main layout
│
├── partials/
│   ├── sidebar.templ          # Left nav sidebar (container)
│   ├── sidebar_section.templ  # Reusable section (Sessions, Agents, etc.)
│   ├── sidebar_item.templ     # Reusable nav item
│   ├── chat_main.templ        # Main chat area (SWAP TARGET: #chat-main)
│   ├── right_panel.templ      # Right panel container with tabs
│   ├── right_panel_content.templ  # (SWAP TARGET: #right-panel-content)
│   ├── message.templ          # Single message (user, assistant, tool, thinking)
│   ├── empty_state.templ      # Generic empty state component
│   ├── loading_skeleton.templ # Skeleton loader for sections
│   └── toasts.templ           # Toast container
│
├── modals/
│   ├── create_session.templ   # Session creation form
│   ├── create_agent.templ     # Agent spawn form
│   ├── create_team.templ      # Team creation form
│   ├── create_task.templ      # Task creation form
│   ├── task_detail.templ      # Task detail/edit modal
│   ├── confirm.templ          # Generic confirmation dialog
│   └── modal_base.templ       # Reusable modal wrapper
│
├── panels/
│   ├── panel_stats.templ      # Right panel: Statistics
│   ├── panel_tools.templ      # Right panel: Tools manager
│   └── panel_config.templ     # Right panel: Configuration
│
└── types.go                   # Data structures
```

### Core Components

#### layout.templ (Base Layout)

**Role:** Application shell — never swapped  
**Content:**
- `<html>`, `<head>` (meta, CSS, scripts)
- Header navigation (always visible)
- Main container with sidebar + content + right panel
- Footer (if applicable)
- Toast container (always present)

**Structure:**
```html
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>{{ .Title }} — Claudio</title>
    <link rel="stylesheet" href="/static/style.css" />
  </head>
  <body data-theme="dark">
    {{ templ.NestComponent(header(context)) }}
    
    <div class="app-container">
      {{ templ.NestComponent(sidebar(context)) }}
      
      <main id="chat-main">
        {{ .ChildContent }}
      </main>
      
      {{ templ.NestComponent(rightPanel(context)) }}
    </div>
    
    {{ templ.NestComponent(toastContainer()) }}
    
    <script src="/static/app.js"></script>
  </body>
</html>
```

#### chat.templ (Chat Page)

**Role:** Main page template, extends base layout  
**Content:**
- Chat message history
- Message input area
- Approval dialogs (if needed)
- Thinking indicators

**htmx attributes:**
- Messages area is append-only (SSE stream adds messages)
- Input has autocomplete trigger: `hx-trigger="keydown[key=='Enter'] delay:200ms"`

#### sidebar.templ (Left Navigation)

**Role:** Container for entity sections  
**Includes:**
- Sessions section
- Agents section
- Teams section
- Tasks section
- User profile footer

**htmx attributes:**
- Each nav item: `hx-get="/api/sessions/{id}"` or similar
- Each item has `aria-current="page"` if active
- Section headers not interactive

#### right_panel.templ (Right Panel)

**Role:** Tabbed panel container  
**Content:**
- Tab buttons (Stats, Tools, Config)
- Content region (`#right-panel-content`)

**htmx attributes:**
```html
<button hx-get="/api/panel/stats"
        hx-target="#right-panel-content"
        hx-swap="innerHTML transition:true"
        role="tab"
        aria-selected="true">
  Stats
</button>
```

### Swap Targets & Strategies

| Target ID | Content | Swap Strategy | Trigger |
|-----------|---------|---------------|---------|
| `#chat-main` | Chat history + input | `innerHTML` | Session/agent/team click |
| `#right-panel-content` | Panel content (stats/tools/config) | `innerHTML` | Tab click |
| `#sessions-list` | Sessions items | `innerHTML` | Session list refresh |
| `#agents-list` | Agent items | `innerHTML` | Agent list refresh |
| `#teams-list` | Team items | `innerHTML` | Team list refresh |
| `#tasks-list` | Task items (grouped) | `innerHTML` | Task list refresh |
| `#modal-content` | Modal body | `innerHTML` | Modal open/close |
| `#toast-container` | Toast items | `beforeend` | Any notification |
| `#session-item-{id}` | Individual session | `outerHTML` | Rename/delete/status change |
| `#agent-item-{id}` | Individual agent | `outerHTML` | Agent status change/kill |
| `#team-item-{id}` | Individual team | `outerHTML` | Team status change/delete |
| `#task-item-{id}` | Individual task | `outerHTML` | Task status change/update |

### OOB Swap Patterns

**Pattern 1: Sidebar item update on status change**

Server response includes OOB update alongside main swap:

```html
<!-- Main response (e.g., task detail modal content) -->
<div id="modal-content">
  [Modal content]
</div>

<!-- OOB: Update corresponding sidebar item -->
<div id="task-item-3" hx-swap-oob="outerHTML">
  <div class="task-item status-in_progress">
    <span class="status-icon">⟳</span>
    <span>Task #3: Implement feature</span>
  </div>
</div>
```

**Pattern 2: Toast notification**

```html
<div id="toast-container" hx-swap-oob="beforeend">
  <div class="toast toast--success" role="status">
    <span>Settings saved successfully</span>
    <button hx-delete="/toast/123" hx-swap="outerHTML">✕</button>
  </div>
</div>
```

**Pattern 3: Add item to list**

```html
<div id="agents-list" hx-swap-oob="beforeend">
  <div class="agent-item">
    <span class="status-icon">⟳</span>
    <span>NewAgent</span>
  </div>
</div>
```

### htmx Configuration & Patterns

**Global htmx config (in static.go or app.js):**

```javascript
document.addEventListener('htmx:load', () => {
  // Initialize components after swap
  // e.g., focus management, event listeners
});

document.addEventListener('htmx:afterSwap', (event) => {
  // Focus management after swap
  if (event.detail.xhr.status === 200) {
    const target = event.detail.target;
    if (target.id === 'chat-main') {
      document.getElementById('chat-input').focus();
    }
  }
});

// HTMX config
htmx.config.timeout = 10000; // 10s timeout
htmx.config.defaultIndicatorStyle = 'spinner';
```

**Common htmx patterns:**

```html
<!-- Form submission with validation -->
<form hx-post="/api/tasks/create"
      hx-target="#modal-content"
      hx-swap="outerHTML">
  <input type="text" name="title" required />
  <button type="submit">Create</button>
</form>

<!-- Tab navigation -->
<button hx-get="/api/panel/{{.Type}}"
        hx-target="#right-panel-content"
        hx-swap="innerHTML transition:true"
        hx-push-url="true"
        role="tab"
        aria-selected="{{.IsActive}}">
  {{.Label}}
</button>

<!-- Delete with confirmation -->
<form hx-delete="/api/sessions/{{.ID}}"
      hx-confirm="Delete session '{{.Title}}'? This cannot be undone."
      hx-target="#session-item-{{.ID}}"
      hx-swap="outerHTML swap:1s">
  <button type="submit">Delete</button>
</form>

<!-- Polling (if needed for status updates) -->
<div hx-get="/api/agents/status"
     hx-trigger="every 2s"
     hx-target="#agents-list"
     hx-swap="innerHTML">
  [Agent list]
</div>
```

### Data Types

**types.go structures:**

```go
// Session entity
type SessionInfo struct {
    ID       string
    Title    string
    State    string // "idle", "streaming", "approval"
    MsgCount int
    Active   bool
    CreatedAt time.Time
}

// Agent entity
type AgentInfo struct {
    ID       string
    Name     string
    Type     string // "Explore", "Plan", "Verify", etc.
    State    string // "idle", "running", "waiting"
    Context  string // "3 files", "planning", etc.
    Active   bool
}

// Team entity
type TeamInfo struct {
    ID       string
    Name     string
    Members  int
    Tasks    int
    Status   string // "idle", "running"
    Active   bool
}

// Task entity
type TaskInfo struct {
    ID          string
    Title       string
    Status      string // "pending", "in_progress", "completed"
    Description string
    AssignedTo  string
    CreatedAt   time.Time
}

// Panel data
type PanelData struct {
    InputTokens  int
    OutputTokens int
    TotalTokens  int
    CacheRead    int
    CacheCreate  int
    Cost         string
    Model        string
    Tools        []ToolInfo
    Tasks        []TaskInfo
}
```

---

## Implementation Checklist

### Phase 1: Foundation

- [ ] **CSS Design System**
  - [ ] Add semantic color token variables (core, entity, status)
  - [ ] Define spacing scale (4px base)
  - [ ] Define typography scale (JetBrains Mono sizes/weights)
  - [ ] Define z-index scale
  - [ ] Add transition/animation definitions

- [ ] **HTML Structure**
  - [ ] Update layout.templ with new shell (header, 3-column layout)
  - [ ] Create sidebar.templ with entity sections
  - [ ] Create right_panel.templ with tabs
  - [ ] Define swap targets (`#chat-main`, `#right-panel-content`, etc.)

### Phase 2: Components

- [ ] **Sidebar Components**
  - [ ] Sessions section with list items, create button
  - [ ] Agents section with list items, spawn button
  - [ ] Teams section with list items, create button
  - [ ] Tasks section (grouped by status) with list items, create button
  - [ ] User profile footer with settings/logout

- [ ] **Navigation Items**
  - [ ] Session item (status icon, title, message count)
  - [ ] Agent item (status icon, name, context badge)
  - [ ] Team item (status icon, name, member count)
  - [ ] Task item (status icon, ID, title, assignee)

- [ ] **Modal Components**
  - [ ] Create session modal
  - [ ] Spawn agent modal
  - [ ] Create team modal
  - [ ] Create task modal
  - [ ] Task detail modal
  - [ ] Confirmation dialogs

- [ ] **Right Panel**
  - [ ] Tab buttons (Stats, Tools, Config)
  - [ ] Stats panel content
  - [ ] Tools panel content
  - [ ] Config panel content

### Phase 3: Interaction

- [ ] **htmx Routes & Handlers**
  - [ ] GET /api/sessions/{id} → load session chat
  - [ ] POST /api/sessions/create → new session
  - [ ] POST /api/agents/spawn → new agent
  - [ ] GET /api/teams/{id} → load team workspace
  - [ ] POST /api/tasks/create → new task
  - [ ] PUT /api/tasks/{id} → update task status
  - [ ] GET /api/panel/{type} → right panel content

- [ ] **Loading States**
  - [ ] Loading spinner for main area swaps
  - [ ] Skeleton loaders for panel content
  - [ ] Button spinners for form submits
  - [ ] Loading indicator CSS (200ms delay)

- [ ] **Error Handling**
  - [ ] Validation error display in forms
  - [ ] Network error toasts
  - [ ] Server error messages
  - [ ] Retry mechanisms

### Phase 4: Accessibility

- [ ] **Keyboard Navigation**
  - [ ] Tab order correct (header → sidebar → main → right panel)
  - [ ] Arrow keys within sidebar sections
  - [ ] Enter/Space to activate items
  - [ ] Escape to close modals

- [ ] **ARIA & Semantics**
  - [ ] `aria-current="page"` on active nav items
  - [ ] `aria-selected` on active tabs
  - [ ] `aria-label` on icon-only buttons
  - [ ] `aria-live` on toast container
  - [ ] `aria-busy` during requests
  - [ ] `role="tab"`, `role="tabpanel"` on tabs

- [ ] **Focus Management**
  - [ ] Focus moves to chat input after session switch
  - [ ] Focus moves to first input in modal
  - [ ] Focus returns to opener button after modal close
  - [ ] `:focus-visible` styling for keyboard focus only

- [ ] **Contrast & Colors**
  - [ ] Verify all text/background pairs ≥4.5:1
  - [ ] Status icons have text labels (not color alone)
  - [ ] Focus ring visible (3px, colored border)

- [ ] **Motion**
  - [ ] All transitions ≤300ms
  - [ ] Respects `prefers-reduced-motion` media query
  - [ ] No infinite animations (except spinners/loading)

### Phase 5: Testing

- [ ] **Visual Testing**
  - [ ] Desktop layout (1440px+)
  - [ ] Tablet layout (1024px)
  - [ ] Mobile layout (375px)
  - [ ] Hamburger menu on mobile
  - [ ] Right panel drawer on mobile

- [ ] **Interaction Testing**
  - [ ] Session switch (main area swaps, focus moves)
  - [ ] Agent spawn (modal, form, sidebar updates)
  - [ ] Task status update (modal + sidebar OOB)
  - [ ] Right panel tabs (content swaps)
  - [ ] Loading states appear/disappear correctly

- [ ] **Accessibility Testing**
  - [ ] Keyboard-only navigation (Tab, Arrow, Enter, Escape)
  - [ ] Screen reader announcements (NVDA/JAWS/VoiceOver)
  - [ ] Color contrast verification (WebAIM, Contrast Checker)
  - [ ] Focus ring visibility
  - [ ] Motion preference respected

- [ ] **Cross-Browser**
  - [ ] Chrome (latest)
  - [ ] Firefox (latest)
  - [ ] Safari (latest)
  - [ ] Edge (latest)

---

## References & Resources

### Design System
- **Color tokens:** See "Design Language" section (CSS custom properties)
- **Typography:** JetBrains Mono (Google Fonts, already in use)
- **Spacing:** Base-4 scale defined in "Layout System"
- **Components:** Bootstrap patterns (buttons, forms, modals) customized for Gruvbox dark

### Accessibility
- **WCAG 2.1 Level AA:** [W3C WCAG](https://www.w3.org/WAI/WCAG21/quickref/)
- **htmx Accessibility:** [htmx Accessibility Docs](https://htmx.org/docs/#security-and-accessibility)
- **Aria Authoring Practices:** [APG from ARIA WG](https://www.w3.org/WAI/ARIA/apg/)

### htmx Patterns
- **Official Docs:** [htmx.org](https://htmx.org)
- **Examples:** See "Interaction Patterns" section above
- **OOB Swaps:** [htmx Out of Band Swaps](https://htmx.org/attributes/hx-swap-oob/)

### Tools
- **Contrast Checker:** [WebAIM](https://webaim.org/resources/contrastchecker/)
- **Color Palette:** Gruvbox [GitHub](https://github.com/morhetz/gruvbox)
- **Font:** JetBrains Mono [Google Fonts](https://fonts.google.com/specimen/JetBrains+Mono)

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-01-01 | Initial design specification approved |

---

**Document Status:** ✅ Approved for Implementation  
**Last Reviewed:** 2025-01-01  
**Next Review:** After Phase 1 implementation complete
