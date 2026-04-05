package web

// staticFiles maps file paths to their content (embedded at compile time).
var staticFiles = map[string]string{
	"style.css": cssContent,
}

const cssContent = `
/* ═══════════════════════════════════════════════════════════
   Claudio Web UI — Gruvbox Dark Theme
   ═══════════════════════════════════════════════════════════ */

:root {
  --bg:          #1d2021;
  --bg0:         #282828;
  --bg1:         #3c3836;
  --bg2:         #504945;
  --bg3:         #665c54;
  --fg:          #ebdbb2;
  --fg2:         #bdae93;
  --fg3:         #a89984;
  --dim:         #928374;
  --red:         #fb4934;
  --green:       #b8bb26;
  --yellow:      #fabd2f;
  --blue:        #83a598;
  --purple:      #d3869b;
  --aqua:        #8ec07c;
  --orange:      #fe8019;

  --radius:      6px;
  --radius-lg:   10px;
  --font-mono:   'JetBrains Mono', 'Fira Code', 'Cascadia Code', ui-monospace, monospace;
  --sidebar-w:   340px;
  --header-h:    48px;
  --status-h:    28px;
  --transition:  150ms ease;
}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
html { font-size: 14px; }
body {
  font-family: var(--font-mono);
  background: var(--bg);
  color: var(--fg);
  line-height: 1.6;
  overflow: hidden;
  height: 100dvh;
  height: 100vh; /* fallback */
  padding: env(safe-area-inset-top) env(safe-area-inset-right) env(safe-area-inset-bottom) env(safe-area-inset-left);
}

@supports (height: 100dvh) {
  body { height: 100dvh; }
}
a { color: var(--blue); text-decoration: none; }
a:hover { text-decoration: underline; }

::-webkit-scrollbar { width: 6px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--bg2); border-radius: 3px; }
::-webkit-scrollbar-thumb:hover { background: var(--bg3); }

/* ── Buttons ── */
.btn {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 6px 14px;
  border-radius: var(--radius);
  border: 1px solid var(--bg2);
  background: var(--bg1);
  color: var(--fg);
  font-family: var(--font-mono);
  font-size: 0.85rem;
  cursor: pointer;
  transition: all var(--transition);
  white-space: nowrap;
}
.btn:hover { background: var(--bg2); border-color: var(--bg3); }
.btn-primary { background: var(--blue); color: var(--bg); border-color: var(--blue); font-weight: 600; }
.btn-primary:hover { background: #9abfb0; }
.btn-ghost { background: transparent; border-color: transparent; color: var(--dim); }
.btn-ghost:hover { color: var(--fg); background: var(--bg1); }
.btn-sm { padding: 3px 8px; font-size: 0.75rem; }
.btn-approve { background: var(--green); color: var(--bg); border-color: var(--green); font-weight: 600; }
.btn-approve:hover { opacity: 0.85; }
.btn-deny { background: var(--red); color: var(--bg); border-color: var(--red); font-weight: 600; }
.btn-deny:hover { opacity: 0.85; }

/* ── Inputs ── */
.input {
  padding: 8px 12px;
  border-radius: var(--radius);
  border: 1px solid var(--bg2);
  background: var(--bg0);
  color: var(--fg);
  font-family: var(--font-mono);
  font-size: 0.9rem;
  outline: none;
  transition: border-color var(--transition);
}
.input:focus { border-color: var(--blue); }
.input-lg { padding: 10px 14px; font-size: 1rem; }

/* ── Badges ── */
.badge {
  display: inline-block; padding: 2px 8px; border-radius: 99px;
  font-size: 0.7rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px;
  background: var(--bg2); color: var(--dim);
}
.badge-ok { background: rgba(184,187,38,0.15); color: var(--green); }
.badge-warn { background: rgba(250,189,47,0.15); color: var(--yellow); }
.badge-error { background: rgba(251,73,52,0.15); color: var(--red); }
.badge-info { background: rgba(131,165,152,0.15); color: var(--blue); }
.badge-purple { background: rgba(211,134,155,0.15); color: var(--purple); }

/* ═══════════════════ LOGIN ═══════════════════ */
.login-container {
  display: flex; align-items: center; justify-content: center; height: 100dvh; height: 100vh;
}
.login-card {
  width: 380px; max-width: calc(100vw - 32px); padding: 40px; border-radius: var(--radius-lg);
  background: var(--bg0); border: 1px solid var(--bg1); box-shadow: 0 8px 32px rgba(0,0,0,0.4);
}
.login-header { text-align: center; margin-bottom: 28px; }
.login-header h1 { font-size: 2rem; color: var(--purple); font-weight: 700; letter-spacing: -0.5px; }
.login-header .subtitle { color: var(--dim); font-size: 0.85rem; margin-top: 4px; }
.login-form { display: flex; flex-direction: column; gap: 12px; }
.login-form .input { width: 100%; }
.login-form .btn-primary { width: 100%; justify-content: center; padding: 10px; }

/* ═══════════════════ HOME ═══════════════════ */
.app-container { display: flex; flex-direction: column; height: 100dvh; height: 100vh; }
.app-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 12px 24px; background: var(--bg0); border-bottom: 1px solid var(--bg1); height: var(--header-h);
}
.header-left { display: flex; align-items: center; gap: 12px; }
.header-left h1 { font-size: 1.2rem; color: var(--purple); font-weight: 700; }
.version { color: var(--dim); font-size: 0.75rem; }
.home-main {
  flex: 1; overflow-y: auto; padding: 32px 24px; max-width: 900px; margin: 0 auto; width: 100%;
}
.home-main h2 { font-size: 1rem; color: var(--fg2); font-weight: 600; margin-bottom: 12px; }
.init-section { margin-bottom: 32px; }
.init-form { display: flex; gap: 8px; }
.init-form .input { flex: 1; }
.project-grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px;
}
.project-card {
  display: block; padding: 16px; border-radius: var(--radius);
  background: var(--bg0); border: 1px solid var(--bg1);
  transition: all var(--transition); text-decoration: none !important;
}
.project-card:hover { border-color: var(--blue); background: var(--bg1); }
.project-name { font-weight: 600; color: var(--aqua); margin-bottom: 4px; }
.project-path { font-size: 0.75rem; color: var(--dim); margin-bottom: 8px; word-break: break-all; }
.empty-state { padding: 40px; text-align: center; color: var(--dim); }
.error-msg {
  padding: 10px 14px; border-radius: var(--radius);
  background: rgba(251,73,52,0.1); border: 1px solid var(--red); color: var(--red);
  font-size: 0.85rem; margin-bottom: 12px;
}

/* ═══════════════════ CHAT LAYOUT ═══════════════════ */
.chat-layout { display: flex; flex-direction: column; height: 100dvh; height: 100vh; }
.chat-header {
  display: flex; align-items: center; gap: 16px; padding: 0 16px;
  height: var(--header-h); background: var(--bg0); border-bottom: 1px solid var(--bg1); flex-shrink: 0;
}
.back-link { color: var(--blue); font-size: 0.85rem; display: flex; align-items: center; gap: 4px; }
.chat-header .project-name {
  color: var(--aqua); font-weight: 600; font-size: 0.9rem;
  flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}

/* Panel toggle buttons */
.panel-toggles { display: flex; gap: 2px; }
.panel-toggle {
  padding: 4px 10px; border-radius: var(--radius); border: 1px solid transparent;
  background: transparent; color: var(--dim); font-family: var(--font-mono);
  font-size: 0.75rem; cursor: pointer; transition: all var(--transition);
}
.panel-toggle:hover { color: var(--fg); background: var(--bg1); }
.panel-toggle.active { color: var(--purple); background: var(--bg1); border-color: var(--bg2); }

/* Body = chat + sidebar */
.chat-body { display: flex; flex: 1; overflow: hidden; }
.chat-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.messages-area { flex: 1; overflow-y: auto; padding: 16px; scroll-behavior: smooth; }
.stream-area { padding: 0 16px; }

/* ── Messages ── */
.msg { margin-bottom: 16px; max-width: 100%; animation: fadeIn 200ms ease; }
@keyframes fadeIn { from { opacity:0; transform:translateY(4px); } to { opacity:1; transform:translateY(0); } }

.msg-user {
  padding: 10px 14px; border-left: 3px solid var(--blue);
  background: rgba(131,165,152,0.06); border-radius: 0 var(--radius) var(--radius) 0;
}
.msg-user .msg-label {
  font-size: 0.7rem; font-weight: 700; text-transform: uppercase;
  letter-spacing: 1px; color: var(--blue); margin-bottom: 4px;
}

.msg-assistant {
  padding: 10px 14px; border-left: 3px solid var(--purple);
  background: rgba(211,134,155,0.04); border-radius: 0 var(--radius) var(--radius) 0;
}
.msg-assistant .msg-label {
  font-size: 0.7rem; font-weight: 700; text-transform: uppercase;
  letter-spacing: 1px; color: var(--purple); margin-bottom: 4px;
}

/* ── Thinking ── */
.thinking-block {
  margin: 8px 0; border-left: 2px solid var(--bg2);
  background: rgba(146,131,116,0.05); border-radius: 0 var(--radius) var(--radius) 0;
}
.thinking-toggle {
  display: flex; align-items: center; gap: 6px; padding: 6px 10px;
  cursor: pointer; font-size: 0.8rem; color: var(--dim); font-style: italic;
  user-select: none;
}
.thinking-toggle:hover { color: var(--fg2); }
.thinking-toggle .arrow { font-size: 0.65rem; transition: transform var(--transition); }
.thinking-toggle .arrow.open { transform: rotate(90deg); }
.thinking-content {
  display: none; padding: 4px 10px 8px; font-size: 0.8rem;
  color: var(--dim); font-style: italic; white-space: pre-wrap;
}
.thinking-content.open { display: block; }

/* ── Tool cards ── */
.tool-card {
  margin: 8px 0; border: 1px solid var(--bg2); border-radius: var(--radius);
  background: var(--bg0); overflow: hidden; font-size: 0.85rem;
}
.tool-header {
  display: flex; align-items: center; gap: 8px; padding: 8px 12px;
  background: var(--bg1); cursor: pointer; user-select: none;
}
.tool-header:hover { background: var(--bg2); }
.tool-icon { color: var(--yellow); font-size: 0.9rem; }
.tool-name { font-weight: 600; color: var(--yellow); }
.tool-status {
  margin-left: auto; font-size: 0.7rem; font-weight: 600;
  text-transform: uppercase; letter-spacing: 0.5px;
}
.tool-status.running { color: var(--yellow); }
.tool-status.done { color: var(--green); }
.tool-status.error { color: var(--red); }
.tool-expand {
  margin-left: 4px; color: var(--dim); transition: transform var(--transition); font-size: 0.7rem;
}
.tool-expand.open { transform: rotate(90deg); }
.tool-body { display: none; border-top: 1px solid var(--bg2); }
.tool-body.open { display: block; }
.tool-input, .tool-output {
  padding: 8px 12px; font-size: 0.8rem; overflow-x: auto;
  white-space: pre-wrap; word-break: break-all; color: var(--fg2);
  max-height: 300px; overflow-y: auto;
}
.tool-input pre, .tool-output pre { margin: 0; font-size: inherit; color: inherit; background: transparent; }
.tool-output { border-top: 1px solid var(--bg2); background: rgba(29,32,33,0.5); }
.tool-output.error-output { color: var(--red); }

/* ── Approval ── */
.approval-overlay {
  position: fixed; inset: 0; background: rgba(0,0,0,0.6);
  display: flex; align-items: center; justify-content: center; z-index: 100;
  animation: fadeIn 150ms ease;
}
.approval-dialog {
  width: 520px; max-width: 90vw; max-height: 80vh; overflow-y: auto;
  border-radius: var(--radius-lg); background: var(--bg0);
  border: 1px solid var(--yellow); box-shadow: 0 12px 40px rgba(0,0,0,0.5);
}
.approval-dialog-header {
  padding: 14px 18px; background: rgba(250,189,47,0.08);
  border-bottom: 1px solid var(--bg2); display: flex; align-items: center; gap: 8px;
}
.approval-dialog-header .icon { color: var(--yellow); font-size: 1.1rem; }
.approval-dialog-header .title { font-weight: 700; color: var(--yellow); }
.approval-dialog-body { padding: 14px 18px; }
.approval-tool-name { color: var(--orange); font-weight: 600; margin-bottom: 8px; }
.approval-input-preview {
  padding: 10px; border-radius: var(--radius); background: var(--bg);
  border: 1px solid var(--bg2); font-size: 0.8rem; max-height: 200px;
  overflow-y: auto; white-space: pre-wrap; word-break: break-all; color: var(--fg2);
}
.approval-actions {
  display: flex; gap: 8px; justify-content: flex-end;
  padding: 12px 18px; border-top: 1px solid var(--bg2);
}
.approval-result { padding: 4px 10px; font-size: 0.75rem; color: var(--dim); font-style: italic; }

/* ── Input bar ── */
.chat-input-bar {
  padding: 8px 16px 12px; background: var(--bg0); border-top: 1px solid var(--bg1); flex-shrink: 0;
}
.chat-input-form { display: flex; gap: 8px; align-items: center; }
.chat-input {
  flex: 1; padding: 10px 14px; border-radius: var(--radius);
  border: 1px solid var(--bg2); background: var(--bg); color: var(--fg);
  font-family: var(--font-mono); font-size: 0.9rem; outline: none;
  transition: border-color var(--transition);
}
.chat-input:focus { border-color: var(--purple); }
.chat-input:disabled { opacity: 0.5; }

/* ── Status bar ── */
.status-bar {
  display: flex; align-items: center; gap: 16px; padding: 0 16px;
  height: var(--status-h); background: var(--bg1); border-top: 1px solid var(--bg2);
  font-size: 0.7rem; color: var(--dim); flex-shrink: 0;
}
.status-item { display: flex; align-items: center; gap: 4px; }
.status-label { color: var(--bg3); }
.status-value { color: var(--fg2); }
.status-sep { color: var(--bg2); }
.status-model { color: var(--purple); font-weight: 600; }
.status-ok { color: var(--green); }

/* ═══════════════════ SIDE PANEL ═══════════════════ */
.side-panel {
  width: 0; overflow: hidden; border-left: 1px solid var(--bg1);
  background: var(--bg0); transition: width 200ms ease;
  display: flex; flex-direction: column; flex-shrink: 0;
}
.side-panel.open { width: var(--sidebar-w); }
.panel-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 14px; border-bottom: 1px solid var(--bg1); flex-shrink: 0;
}
.panel-title { font-size: 0.85rem; font-weight: 700; color: var(--purple); }
.panel-close {
  background: none; border: none; color: var(--dim); cursor: pointer;
  font-size: 1rem; padding: 2px; font-family: var(--font-mono);
}
.panel-close:hover { color: var(--fg); }
.panel-content { flex: 1; overflow-y: auto; padding: 12px 14px; font-size: 0.8rem; }

.panel-section { margin-bottom: 16px; }
.panel-section-title {
  font-size: 0.7rem; font-weight: 700; text-transform: uppercase;
  letter-spacing: 1px; color: var(--dim); margin-bottom: 8px;
  padding-bottom: 4px; border-bottom: 1px solid var(--bg1);
}
.panel-row { display: flex; justify-content: space-between; align-items: center; padding: 4px 0; }
.panel-row .label { color: var(--fg2); }
.panel-row .value { color: var(--fg); font-weight: 600; }
.panel-row .value.green { color: var(--green); }
.panel-row .value.yellow { color: var(--yellow); }
.panel-row .value.purple { color: var(--purple); }
.panel-bar { margin: 6px 0; }
.panel-bar-track { height: 4px; border-radius: 2px; background: var(--bg2); overflow: hidden; }
.panel-bar-fill { height: 100%; border-radius: 2px; transition: width 300ms ease; }

.config-item { padding: 6px 0; border-bottom: 1px solid var(--bg1); }
.config-item:last-child { border-bottom: none; }
.config-key { color: var(--aqua); font-size: 0.75rem; }
.config-val { color: var(--fg); margin-top: 2px; }

.memory-entry {
  padding: 8px 10px; border-radius: var(--radius); background: var(--bg);
  border: 1px solid var(--bg1); margin-bottom: 6px;
}
.memory-entry .mem-title { font-weight: 600; color: var(--fg); margin-bottom: 2px; }
.memory-entry .mem-scope { font-size: 0.65rem; color: var(--dim); text-transform: uppercase; }
.memory-entry .mem-preview { font-size: 0.75rem; color: var(--fg2); margin-top: 4px; }

.skill-item {
  display: flex; align-items: center; justify-content: space-between;
  padding: 8px 10px; border-radius: var(--radius); margin-bottom: 4px;
  cursor: pointer; transition: background var(--transition);
}
.skill-item:hover { background: var(--bg1); }
.skill-name { font-weight: 600; color: var(--orange); }
.skill-source { font-size: 0.65rem; color: var(--dim); }

.task-item {
  padding: 8px 10px; border-radius: var(--radius); background: var(--bg);
  border: 1px solid var(--bg1); margin-bottom: 6px;
}
.task-item .task-title { font-weight: 600; color: var(--fg); margin-bottom: 2px; }
.task-item .task-status { font-size: 0.65rem; text-transform: uppercase; font-weight: 600; }
.task-item .task-status.pending { color: var(--yellow); }
.task-item .task-status.running { color: var(--blue); }
.task-item .task-status.done { color: var(--green); }
.task-item .task-status.failed { color: var(--red); }

/* ── Markdown ── */
.msg-content code {
  background: var(--bg1); padding: 1px 5px; border-radius: 3px;
  font-size: 0.85em; color: var(--orange);
}
.msg-content pre {
  margin: 8px 0; padding: 12px; border-radius: var(--radius);
  background: var(--bg); border: 1px solid var(--bg1);
  overflow-x: auto; font-size: 0.8rem; line-height: 1.5;
}
.msg-content pre code { background: transparent; padding: 0; color: var(--fg); }
.msg-content strong { color: var(--fg); font-weight: 700; }
.msg-content h1, .msg-content h2, .msg-content h3 { color: var(--aqua); font-weight: 700; margin: 12px 0 6px; }
.msg-content h1 { font-size: 1.1rem; }
.msg-content h2 { font-size: 1rem; }
.msg-content h3 { font-size: 0.9rem; }
.msg-content ul, .msg-content ol { padding-left: 20px; margin: 6px 0; }
.msg-content li { margin-bottom: 2px; }
.msg-content blockquote {
  border-left: 3px solid var(--bg3); padding-left: 12px; color: var(--fg2); margin: 8px 0;
}
.msg-content table { border-collapse: collapse; margin: 8px 0; font-size: 0.8rem; }
.msg-content th, .msg-content td { padding: 4px 10px; border: 1px solid var(--bg2); }
.msg-content th { background: var(--bg1); color: var(--fg2); font-weight: 600; }

/* ── Cursor ── */
.cursor { color: var(--purple); }
@keyframes blink { 0%,50% { opacity: 1; } 51%,100% { opacity: 0; } }
.cursor.blink { animation: blink 800ms infinite; }

@keyframes spin { to { transform: rotate(360deg); } }
.spinner { display: inline-block; animation: spin 1s linear infinite; }

/* ── Mobile menu button (hamburger) ── */
.mobile-menu-btn {
  display: none; /* hidden on desktop */
  align-items: center; justify-content: center;
  width: 44px; height: 44px; min-width: 44px;
  background: none; border: none; color: var(--fg);
  font-size: 1.2rem; cursor: pointer; padding: 0;
  -webkit-tap-highlight-color: transparent;
}
.mobile-menu-btn:active { color: var(--purple); }

/* ── Mobile bottom drawer for panels ── */
.mobile-panel-drawer {
  display: none;
  position: fixed; left: 0; right: 0; bottom: 0;
  background: var(--bg0); border-top: 1px solid var(--bg1);
  z-index: 200; padding: 12px; padding-bottom: calc(12px + env(safe-area-inset-bottom));
  box-shadow: 0 -4px 20px rgba(0,0,0,0.4);
  border-radius: var(--radius-lg) var(--radius-lg) 0 0;
  animation: slideUp 200ms ease;
}
.mobile-panel-drawer.open { display: block; }
@keyframes slideUp { from { transform: translateY(100%); } to { transform: translateY(0); } }
.mobile-panel-drawer .drawer-items { display: flex; flex-direction: column; gap: 4px; }
.mobile-panel-drawer .drawer-item {
  display: flex; align-items: center; gap: 10px;
  padding: 14px; border-radius: var(--radius);
  background: var(--bg1); color: var(--fg);
  font-family: var(--font-mono); font-size: 0.9rem;
  cursor: pointer; border: none; width: 100%; text-align: left;
  -webkit-tap-highlight-color: transparent;
  min-height: 48px;
}
.mobile-panel-drawer .drawer-item:active { background: var(--bg2); }
.mobile-panel-drawer .drawer-item .icon { color: var(--purple); font-size: 1rem; }
.mobile-drawer-overlay {
  display: none;
  position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 199;
}
.mobile-drawer-overlay.open { display: block; }

/* ── Chat input as textarea ── */
.chat-input-textarea {
  flex: 1; padding: 10px 14px; border-radius: var(--radius);
  border: 1px solid var(--bg2); background: var(--bg); color: var(--fg);
  font-family: var(--font-mono); font-size: 0.9rem; outline: none;
  transition: border-color var(--transition);
  resize: none; overflow-y: hidden; min-height: 42px; max-height: 120px;
  line-height: 1.5;
}
.chat-input-textarea:focus { border-color: var(--purple); }
.chat-input-textarea:disabled { opacity: 0.5; }

/* ── Connection status indicator ── */
.connection-indicator {
  display: inline-flex; align-items: center; gap: 4px;
}
.connection-dot {
  width: 6px; height: 6px; border-radius: 50%;
  background: var(--green);
}
.connection-dot.disconnected { background: var(--red); }
.connection-dot.reconnecting { background: var(--yellow); animation: blink 600ms infinite; }

/* ═══════════════════ RESPONSIVE — iPhone 15 Pro (393×852) ═══════════════════ */
@media (max-width: 768px) {
  :root {
    --header-h: 48px;
    --status-h: 28px;
  }

  html { font-size: 14px; }

  /* ── Show mobile menu button, hide desktop panel toggles ── */
  .mobile-menu-btn { display: flex; }
  .panel-toggles { display: none; }

  /* ── Side panels as full-screen overlay ── */
  .side-panel {
    position: fixed; top: 0; right: 0; bottom: 0; z-index: 50;
    width: 0; max-width: 100vw;
  }
  .side-panel.open { width: 100vw; }

  /* ── Login ── */
  .login-card { padding: 28px 20px; }
  .login-form .input { padding: 12px; min-height: 48px; }
  .login-form .btn-primary { min-height: 48px; font-size: 0.95rem; }

  /* ── App header (home) ── */
  .app-header { padding: 0 12px; }
  .header-left h1 { font-size: 1.05rem; }
  .header-right .btn { min-height: 44px; }

  /* ── Home main ── */
  .home-main { padding: 20px 14px; }
  .home-main h2 { font-size: 0.95rem; }

  /* ── Init form stacks vertically ── */
  .init-form { flex-direction: column; }
  .init-form .input { min-height: 48px; }
  .init-form .btn { min-height: 48px; justify-content: center; }

  /* ── Project grid single column ── */
  .project-grid { grid-template-columns: 1fr; gap: 10px; }
  .project-card { padding: 14px; min-height: 48px; }

  /* ── Chat header ── */
  .chat-header { padding: 0 10px; gap: 6px; }
  .chat-header .project-name {
    font-size: 0.8rem; min-width: 0;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .back-link { font-size: 0.8rem; min-width: 44px; min-height: 44px; display: flex; align-items: center; }

  /* ── Messages ── */
  .messages-area { padding: 12px 10px; }
  .msg-user, .msg-assistant { padding: 8px 10px; }
  .msg-content pre {
    padding: 10px; font-size: 0.75rem;
    -webkit-overflow-scrolling: touch;
  }
  .msg-content ul, .msg-content ol { padding-left: 16px; }

  /* ── Tool cards ── */
  .tool-card { font-size: 0.8rem; }
  .tool-header { padding: 8px 10px; min-height: 44px; }
  .tool-input, .tool-output { padding: 8px 10px; font-size: 0.75rem; }

  /* ── Approval dialog — fullscreen on mobile ── */
  .approval-dialog { max-width: calc(100vw - 16px); margin: 8px; }
  .approval-dialog-header { padding: 14px 16px; }
  .approval-dialog-body { padding: 14px 16px; }
  .approval-input-preview { max-height: 40vh; }
  .approval-actions { padding: 12px 16px; flex-wrap: wrap; }
  .approval-actions .btn { min-height: 48px; flex: 1; justify-content: center; font-size: 0.9rem; }

  /* ── Chat input bar ── */
  .chat-input-bar {
    padding: 8px 10px;
    padding-bottom: calc(10px + env(safe-area-inset-bottom));
  }
  .chat-input, .chat-input-textarea { padding: 10px 12px; min-height: 48px; }
  .chat-input-form { gap: 6px; align-items: flex-end; }
  .chat-input-form .btn { min-height: 48px; min-width: 48px; padding: 6px 14px; }

  /* ── Status bar ── */
  .status-bar {
    padding: 0 10px; gap: 8px; font-size: 0.65rem;
    overflow-x: auto; white-space: nowrap;
    -webkit-overflow-scrolling: touch;
    padding-bottom: env(safe-area-inset-bottom);
  }
  .status-bar::-webkit-scrollbar { display: none; }

  /* ── Thinking block ── */
  .thinking-content { font-size: 0.75rem; }
  .thinking-toggle { font-size: 0.75rem; min-height: 44px; display: flex; align-items: center; }

  /* ── Prevent iOS zoom on focus (16px minimum) ── */
  input[type="text"], input[type="password"], textarea, select {
    font-size: 16px !important;
  }

  /* ── Touch-friendly all interactive elements ── */
  .btn, button { min-height: 44px; }
}
`
