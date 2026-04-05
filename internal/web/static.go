package web

// staticFiles maps file paths to their content (embedded at compile time).
var staticFiles = map[string]string{
	"style.css": cssContent,
	"app.js":    appJSContent,
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
.chat-layout { display: flex; flex-direction: row; height: 100dvh; height: 100vh; }
.chat-main-wrap { display: flex; flex-direction: column; flex: 1; min-width: 0; }
.chat-header {
  display: flex; align-items: center; gap: 16px; padding: 0 16px;
  height: var(--header-h); background: var(--bg0); border-bottom: 1px solid var(--bg1); flex-shrink: 0;
}
.back-link { color: var(--blue); font-size: 0.85rem; display: flex; align-items: center; gap: 4px; }
.chat-session-title {
  color: var(--fg); font-weight: 600; font-size: 0.9rem;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.header-project-path {
  color: var(--dim); font-size: 0.7rem;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1;
}
.header-actions { display: flex; align-items: center; gap: 8px; }
.mobile-panel-btn {
  display: none; background: none; border: none; color: var(--fg);
  font-size: 1.2rem; cursor: pointer; padding: 4px 8px;
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
  touch-action: manipulation;
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

/* ═══════════════════ AUTOCOMPLETE POPUP ═══════════════════ */
.ac-popup {
  position: absolute; bottom: 100%; left: 0; right: 0;
  max-height: 260px; overflow-y: auto; z-index: 100;
  background: var(--bg0); border: 1px solid var(--bg2);
  border-radius: var(--radius); box-shadow: 0 -4px 16px rgba(0,0,0,0.4);
  margin-bottom: 4px; display: none;
}
.ac-popup.visible { display: block; }
.ac-popup::-webkit-scrollbar { width: 4px; }
.ac-popup::-webkit-scrollbar-thumb { background: var(--bg2); border-radius: 2px; }
.ac-header {
  padding: 6px 10px; font-size: 0.65rem; font-weight: 700;
  color: var(--dim); text-transform: uppercase; letter-spacing: 0.5px;
  border-bottom: 1px solid var(--bg1); position: sticky; top: 0;
  background: var(--bg0);
}
.ac-item {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 10px; cursor: pointer; font-size: 0.8rem;
  transition: background var(--transition);
  -webkit-tap-highlight-color: transparent;
  touch-action: manipulation;
}
.ac-item:hover, .ac-item.selected { background: var(--bg1); }
.ac-item.selected { border-left: 2px solid var(--purple); }
.ac-item .ac-icon {
  width: 18px; text-align: center; flex-shrink: 0;
  font-size: 0.75rem;
}
.ac-item .ac-icon.cmd { color: var(--purple); }
.ac-item .ac-icon.file { color: var(--aqua); }
.ac-item .ac-icon.dir { color: var(--yellow); }
.ac-item .ac-icon.agent { color: var(--orange); }
.ac-item .ac-name {
  font-weight: 600; color: var(--fg); white-space: nowrap;
}
.ac-item .ac-desc {
  color: var(--dim); font-size: 0.7rem; flex: 1; min-width: 0;
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
}
.ac-item .ac-hint {
  color: var(--dim); font-size: 0.6rem; flex-shrink: 0;
  font-family: var(--font-mono);
}
.ac-empty {
  padding: 10px; color: var(--dim); font-size: 0.75rem; text-align: center;
}
/* Wrap the input bar in relative positioning for the popup */
.chat-input-bar { position: relative; }

/* ═══════════════════ SESSION SIDEBAR ═══════════════════ */
.session-sidebar {
  width: 260px; flex-shrink: 0; background: var(--bg0);
  border-right: 1px solid var(--bg1); display: flex; flex-direction: column;
  transition: width 200ms ease, transform 200ms ease;
}
.sidebar-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 10px 12px; border-bottom: 1px solid var(--bg1); flex-shrink: 0;
}
.sidebar-header .sidebar-title { font-size: 0.8rem; font-weight: 700; color: var(--purple); }
.session-list { flex: 1; overflow-y: auto; padding: 6px; }
.session-item {
  display: flex; align-items: center; gap: 4px;
  padding: 10px; border-radius: var(--radius); cursor: pointer;
  transition: background var(--transition); margin-bottom: 2px;
  border: 1px solid transparent;
}
.session-item:hover { background: var(--bg1); }
.session-item.active { background: var(--bg1); border-color: var(--purple); }
.session-item .session-indicator {
  width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0;
  background: var(--dim);
}
.session-item.active .session-indicator { background: var(--green); }
.session-item.streaming .session-indicator { background: var(--yellow); animation: blink 600ms infinite; }
.session-item.approval .session-indicator { background: var(--orange); animation: blink 400ms infinite; }
.session-item-top { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0; }
.session-item .session-title {
  font-size: 0.8rem; font-weight: 600; color: var(--fg);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1;
}
.session-item .session-badge {
  font-size: 0.65rem; color: var(--dim); flex-shrink: 0;
}
.session-item-actions {
  display: none; gap: 2px; flex-shrink: 0;
}
.session-item:hover .session-item-actions { display: flex; }
.btn-icon {
  background: none; border: none; color: var(--dim); cursor: pointer;
  font-size: 0.75rem; padding: 2px 4px; border-radius: 3px;
  font-family: var(--font-mono);
}
.btn-icon:hover { color: var(--fg); background: var(--bg2); }
.btn-icon-danger:hover { color: var(--red); }
.session-action-btn:hover { color: var(--fg); background: var(--bg2); }
.session-action-btn.delete:hover { color: var(--red); }

/* ── Session rename input ── */
.session-rename-input {
  width: 100%; padding: 2px 6px; border-radius: 3px;
  border: 1px solid var(--blue); background: var(--bg);
  color: var(--fg); font-family: var(--font-mono); font-size: 0.8rem;
  outline: none;
}

/* ═══════════════════ TOAST NOTIFICATIONS ═══════════════════ */
.toast-container {
  position: fixed; top: 12px; right: 12px; z-index: 300;
  display: flex; flex-direction: column; gap: 6px; pointer-events: none;
}
.toast {
  pointer-events: auto; padding: 10px 14px; border-radius: var(--radius);
  background: var(--bg0); border: 1px solid var(--bg2);
  box-shadow: 0 4px 16px rgba(0,0,0,0.4); font-size: 0.8rem; color: var(--fg);
  max-width: 340px; animation: slideInRight 200ms ease;
  display: flex; align-items: center; gap: 8px; cursor: pointer;
}
.toast:hover { background: var(--bg1); }
@keyframes slideInRight { from { opacity:0; transform:translateX(40px); } to { opacity:1; transform:translateX(0); } }
.toast .toast-icon { flex-shrink: 0; }
.toast .toast-msg { flex: 1; }
.toast.toast-info { border-color: var(--blue); }
.toast.toast-info .toast-icon { color: var(--blue); }
.toast.toast-warn { border-color: var(--yellow); }
.toast.toast-warn .toast-icon { color: var(--yellow); }
.toast.toast-error { border-color: var(--red); }
.toast.toast-error .toast-icon { color: var(--red); }
.toast.toast-success { border-color: var(--green); }
.toast.toast-success .toast-icon { color: var(--green); }

/* ═══════════════════ RECONNECT BANNER ═══════════════════ */
.reconnect-banner {
  display: none; align-items: center; justify-content: center; gap: 8px;
  padding: 6px 12px; background: rgba(251,73,52,0.15);
  border-bottom: 1px solid var(--red); font-size: 0.8rem; color: var(--red);
  flex-shrink: 0;
}
.reconnect-banner.visible { display: flex; }
.reconnect-banner .spinner { font-size: 0.8rem; }

/* ═══════════════════ AGENTS PANEL ═══════════════════ */
.agents-list { display: flex; flex-direction: column; gap: 6px; }
.agent-card {
  padding: 10px 12px; border-radius: var(--radius);
  background: var(--bg); border: 1px solid var(--bg1);
}
.agent-card .agent-name { font-weight: 600; color: var(--orange); margin-bottom: 2px; }
.agent-card .agent-type { font-size: 0.65rem; color: var(--dim); text-transform: uppercase; }
.agent-card .agent-desc { font-size: 0.75rem; color: var(--fg2); margin-top: 4px; }
.agent-card .agent-status {
  display: flex; align-items: center; gap: 4px;
  font-size: 0.65rem; margin-top: 6px; color: var(--dim);
}
.agent-card .agent-status .dot {
  width: 6px; height: 6px; border-radius: 50%;
}
.agent-card .agent-status .dot.active { background: var(--green); }
.agent-card .agent-status .dot.idle { background: var(--dim); }

/* ═══════════════════ MULTI-SESSION TABS (mobile) ═══════════════════ */
.session-tabs {
  display: none; /* shown via mobile media query */
  overflow-x: auto; white-space: nowrap;
  padding: 4px 8px; background: var(--bg0); border-bottom: 1px solid var(--bg1);
  -webkit-overflow-scrolling: touch; flex-shrink: 0;
}
.session-tabs::-webkit-scrollbar { display: none; }
.session-tab {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 6px 12px; border-radius: var(--radius);
  background: transparent; border: 1px solid transparent;
  color: var(--dim); font-family: var(--font-mono); font-size: 0.75rem;
  cursor: pointer; white-space: nowrap;
}
.session-tab:hover { color: var(--fg); background: var(--bg1); }
.session-tab.active { color: var(--purple); border-color: var(--purple); background: var(--bg1); }
.session-tab .tab-dot {
  width: 6px; height: 6px; border-radius: 50%; background: var(--dim);
}
.session-tab .tab-dot.streaming { background: var(--yellow); animation: blink 600ms infinite; }
.session-tab .tab-dot.approval { background: var(--orange); animation: blink 400ms infinite; }
.session-tab .tab-close {
  margin-left: 4px; color: var(--dim); font-size: 0.65rem; padding: 0 2px;
}
.session-tab .tab-close:hover { color: var(--red); }

/* ═══════════════════ SIDEBAR OVERLAY (mobile) ═══════════════════ */
.sidebar-overlay {
  display: none; position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 49;
}
.sidebar-overlay.visible { display: block; }

/* ═══════════════════ RESPONSIVE — iPhone 15 Pro (393×852) ═══════════════════ */
@media (max-width: 768px) {
  :root {
    --header-h: 48px;
    --status-h: 28px;
  }

  html { font-size: 14px; }

  /* ── Show mobile buttons, hide desktop panel toggles ── */
  .mobile-menu-btn { display: flex; }
  .mobile-panel-btn { display: flex; }
  .panel-toggles { display: none; }

  /* ── Chat layout stacks on mobile ── */
  .chat-layout { flex-direction: column; }

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
  .header-project-path { display: none; }
  .chat-session-title { font-size: 0.8rem; flex: 1; }
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

  /* ── Session sidebar as off-canvas drawer ── */
  .session-sidebar {
    position: fixed; top: 0; left: 0; bottom: 0;
    width: 280px; z-index: 50;
    transform: translateX(-100%);
    will-change: transform;
  }
  .session-sidebar.open { transform: translateX(0); }
  .sidebar-overlay.visible { display: block; z-index: 49; }
  .session-item { min-height: 44px; touch-action: manipulation; -webkit-tap-highlight-color: transparent; }
  .session-item-actions { display: flex; } /* always show on mobile — no hover */

  /* ── Session tabs visible on mobile ── */
  .session-tabs { display: flex; }

  /* ── Toast position on mobile ── */
  .toast-container { top: auto; bottom: 70px; right: 8px; left: 8px; }
  .toast { max-width: 100%; }
}
`

const appJSContent = `
(function(){
  var app=document.getElementById('chat-app');
  if(!app)return;
  var PROJECT=app.dataset.project;
  var SESSION=app.dataset.session;
  var es=null,rd=null,txt='',think='',tIn=0,tOut=0;
  var lastSeq=0,isStreaming=false,reconnecting=false;

  function $(id){return document.getElementById(id);}
  function mk(t,c){var e=document.createElement(t);e.className=c;return e;}
  function sb(){$('messages').scrollTop=$('messages').scrollHeight;}
  function esc(s){var d=document.createElement('div');d.textContent=s;return d.innerHTML;}
  function trn(s,m){if(!s||s.length<=m)return s||'';return s.substring(0,m)+'... ('+s.length+' chars)';}
  function fn(n){if(n>=1e6)return(n/1e6).toFixed(1)+'M';if(n>=1e3)return(n/1e3).toFixed(1)+'K';return''+n;}
  function checkAuth(r){if(!r.ok){if(r.status===401){window.location.href='/login';throw new Error('unauthorized');}throw new Error('request failed: '+r.status);}return r;}
  function md(t){
    return t.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
      .replace(/` + "```" + `(\\w*)\\n([\\s\\S]*?)` + "```" + `/g,function(_,l,c){return'<pre><code>'+c+'</code></pre>';})
      .replace(/` + "`" + `([^` + "`" + `]+)` + "`" + `/g,'<code>$1</code>')
      .replace(/\*\*(.+?)\*\*/g,'<strong>$1</strong>')
      .replace(/^### (.+)$/gm,'<h3>$1</h3>')
      .replace(/^## (.+)$/gm,'<h2>$1</h2>')
      .replace(/^# (.+)$/gm,'<h1>$1</h1>')
      .replace(/\n/g,'<br>');
  }
  function ibc(el){var c=rd.querySelector('.msg-content'),cur=c.querySelector('.cursor');if(cur)c.insertBefore(el,cur);else c.appendChild(el);}
  function rr(){if(!rd)return;var c=rd.querySelector('.msg-content'),el=c.querySelector('.response-text');if(!el){el=mk('div','response-text');var cur=c.querySelector('.cursor');if(cur)c.insertBefore(el,cur);else c.appendChild(el);}el.innerHTML=md(txt);sb();}
  function rt(){if(!rd)return;var c=rd.querySelector('.msg-content'),b=c.querySelector('.thinking-block');if(!b){b=mk('div','thinking-block');b.innerHTML='<div class="thinking-toggle" onclick="toggleThinking(this)"><span class="arrow">&#9654;</span> Thinking...</div><div class="thinking-content"></div>';c.insertBefore(b,c.firstChild);}b.querySelector('.thinking-content').textContent=think;sb();}

  function setConn(state){
    var dot=$('conn-dot');
    dot.className='connection-dot'+(state==='ok'?'':state==='disconnected'?' disconnected':' reconnecting');
    var banner=$('reconnect-banner');
    if(banner){
      if(state==='ok')banner.classList.remove('visible');
      else if(state==='disconnected'||state==='reconnecting')banner.classList.add('visible');
    }
  }

  function fin(){
    if(es){es.close();es=null;}
    isStreaming=false;
    if(rd){var c=rd.querySelector('.cursor');if(c)c.remove();$('messages').appendChild(rd);$('stream-area').innerHTML='';}
    rd=null;
    $('send-btn').disabled=false;$('chat-input').disabled=false;$('chat-input').focus();
    $('status-state').textContent='Ready';$('status-state').className='status-ok';
    setConn('ok');sb();
    updateSidebarState(SESSION,'idle');
  }

  /* ── Process a single SSE event ── */
  function processEvent(evtName,evtData,seq){
    if(seq&&seq>lastSeq)lastSeq=seq;
    if(evtName==='text'){txt+=evtData;rr();}
    else if(evtName==='thinking'){think+=evtData;rt();}
    else if(evtName==='tool_start'){
      try{var d=JSON.parse(evtData);var tc=mk('div','tool-card');tc.id='tool-'+d.id;
      tc.innerHTML='<div class="tool-header" onclick="toggleToolBody(this)"><span class="tool-icon">&#9881;</span><span class="tool-name">'+esc(d.name)+'</span><span class="tool-status running">running</span><span class="tool-expand">&#9654;</span></div><div class="tool-body"><div class="tool-input"><pre>'+esc(trn(d.input,500))+'</pre></div></div>';
      ibc(tc);sb();}catch(x){}
    }
    else if(evtName==='tool_end'){
      try{var d=JSON.parse(evtData);var tc=document.getElementById('tool-'+d.id);if(tc){var s=tc.querySelector('.tool-status');s.className='tool-status '+(d.is_error?'error':'done');s.textContent=d.is_error?'error':'done';if(d.content){var o=mk('div','tool-output'+(d.is_error?' error-output':''));o.innerHTML='<pre>'+esc(trn(d.content,2000))+'</pre>';tc.querySelector('.tool-body').appendChild(o);}}sb();}catch(x){}
    }
    else if(evtName==='approval_needed'){
      try{var d=JSON.parse(evtData);
      $('approval-area').innerHTML='<div class="approval-overlay" onclick="if(event.target===this)doApproval(false)"><div class="approval-dialog"><div class="approval-dialog-header"><span class="icon">&#9888;</span><span class="title">Tool Requires Approval</span></div><div class="approval-dialog-body"><div class="approval-tool-name">'+esc(d.name)+'</div><pre class="approval-input-preview">'+esc(trn(d.input,1000))+'</pre></div><div class="approval-actions"><button class="btn btn-deny" onclick="doApproval(false)">Deny</button><button class="btn btn-approve" onclick="doApproval(true)">Approve</button></div></div></div>';
      updateSidebarState(SESSION,'approval');
      }catch(x){}
    }
    else if(evtName==='approval_result'){$('approval-area').innerHTML='';updateSidebarState(SESSION,'streaming');}
    else if(evtName==='done'){
      try{var d=JSON.parse(evtData);tIn+=(d.input_tokens||0);tOut+=(d.output_tokens||0);$('status-in').textContent=fn(tIn);$('status-out').textContent=fn(tOut);$('status-total').textContent=fn(tIn+tOut);}catch(x){}
      fin();return true;
    }
    else if(evtName==='error'){
      if(evtData){txt+='\n**Error:** '+evtData;rr();}
      fin();return true;
    }
    return false;
  }

  /* ── Ensure the streaming response div exists ── */
  function ensureStreamDiv(){
    if(rd)return;
    rd=mk('div','msg msg-assistant');
    rd.innerHTML='<div class="msg-label">Assistant</div><div class="msg-content"><span class="cursor blink">&#9608;</span></div>';
    $('stream-area').appendChild(rd);sb();
  }

  /* ── Connect SSE with replay support ── */
  function connectSSE(){
    if(es)es.close();
    var url='/api/chat/stream?session='+encodeURIComponent(SESSION);
    if(lastSeq>0)url+='&since='+lastSeq;
    es=new EventSource(url);
    setConn('ok');
    ['text','thinking','tool_start','tool_end','approval_needed','approval_result','done','error'].forEach(function(evt){
      es.addEventListener(evt,function(e){
        ensureStreamDiv();
        var seq=e.lastEventId?parseInt(e.lastEventId,10):0;
        processEvent(evt,e.data,seq);
      });
    });
    es.onerror=function(){
      if(!isStreaming)return fin();
      es.close();es=null;
      setConn('disconnected');
      $('status-state').textContent='Reconnecting...';$('status-state').className='status-value yellow';
      tryReconnect();
    };
  }

  /* ── Reconnect: check server status, replay missed events, reattach SSE ── */
  function tryReconnect(){
    if(reconnecting)return;
    reconnecting=true;
    setConn('reconnecting');
    setTimeout(function doCheck(){
      fetch('/api/chat/status?session='+encodeURIComponent(SESSION))
      .then(checkAuth).then(function(r){return r.json();})
      .then(function(data){
        if(!data.running){
          return fetch('/api/chat/replay?session='+encodeURIComponent(SESSION)+'&since='+lastSeq)
          .then(checkAuth).then(function(r){return r.json();})
          .then(function(replay){
            ensureStreamDiv();
            var events=replay.events||[];
            for(var i=0;i<events.length;i++){
              processEvent(events[i].event,events[i].data,events[i].seq);
            }
            if(!events.some(function(e){return e.event==='done'||e.event==='error';})){
              fin();
            }
            reconnecting=false;
          });
        }else{
          reconnecting=false;
          connectSSE();
        }
      })
      .catch(function(){
        setTimeout(doCheck,2000);
      });
    },500);
  }

  /* ── Send message ── */
  window.sendMessage=function(){
    acClose();
    var input=$('chat-input');
    var msg=input.value.trim();
    if(!msg)return;
    input.value='';resetTextarea();

    // Check for slash command
    if(msg.charAt(0)==='/'){
      handleSlashCommand(msg);
      return;
    }

    // Send as normal chat message
    sendChatMessage(msg);
  };
  var sendBtn=$('send-btn');
  if(sendBtn)sendBtn.addEventListener('click',function(){sendMessage();});

  function handleSlashCommand(msg){
    var parts=msg.substring(1).split(/\s+/,2);
    var cmd=parts[0];
    var args=msg.substring(1+cmd.length).trim();

    // Client-only commands
    if(cmd==='help'){
      appendSystemMsg(ac.cache.commands?ac.cache.commands.map(function(c){return'/'+c.name+' — '+c.desc;}).join('\n'):'Loading...');
      if(!ac.cache.commands){
        fetch('/api/autocomplete/commands').then(checkAuth).then(function(r){return r.json();}).then(function(data){
          ac.cache.commands=data;
          appendSystemMsg(data.map(function(c){return'/'+c.name+' — '+c.desc;}).join('\n'));
        });
      }
      return;
    }
    if(cmd==='tasks'){togglePanel('tasks');return;}
    if(cmd==='agents'){togglePanel('agents');return;}
    if(cmd==='analytics'||cmd==='cost'){togglePanel('analytics');return;}

    // Server-side commands
    var fd=new FormData();
    fd.append('session',SESSION);
    fd.append('command',cmd);
    fd.append('args',args);
    fetch('/api/commands/execute',{method:'POST',body:fd})
    .then(checkAuth).then(function(r){return r.json();})
    .then(function(res){
      if(res.status==='redirect'){
        window.location.href=res.data;return;
      }
      if(res.status==='error'){
        showToast('error',res.message);return;
      }
      // Handle actions
      if(res.action==='clear_messages'){
        $('messages').innerHTML='';
        appendSystemMsg(res.message);
      }else if(res.action==='rename'){
        $('chat-session-title').textContent=res.data;
        var item=document.querySelector('.session-item[data-id="'+SESSION+'"]');
        if(item){var t=item.querySelector('.session-title');if(t)t.textContent=res.data;}
        showToast('success',res.message);
      }else if(res.action==='undo'){
        // Reload messages from server
        window.location.reload();
      }else if(res.action==='export'){
        // Create download
        var blob=new Blob([res.data],{type:'text/markdown'});
        var url=URL.createObjectURL(blob);
        var a=document.createElement('a');a.href=url;a.download='conversation.md';a.click();
        URL.revokeObjectURL(url);
        showToast('success',res.message);
      }else if(res.action==='send_as_message'){
        // Command not handled server-side, send as regular message to AI
        sendChatMessage(res.data);
      }else if(res.message){
        appendSystemMsg(res.message);
      }
    });
  }

  function sendChatMessage(msg){
    // Append user message to DOM
    var um=mk('div','msg msg-user');
    um.innerHTML='<div class="msg-label">You</div><div class="msg-content">'+esc(msg)+'</div>';
    $('messages').appendChild(um);sb();

    // Send to server
    var fd=new FormData();
    fd.append('session',SESSION);
    fd.append('message',msg);
    fetch('/api/chat/send',{method:'POST',body:fd})
    .then(checkAuth).then(function(r){return r.json();})
    .then(function(data){
      if(data.status==='streaming'){
        isStreaming=true;lastSeq=0;
        $('send-btn').disabled=true;$('chat-input').disabled=true;
        $('status-state').textContent='Streaming...';$('status-state').className='status-value yellow';
        $('stream-area').innerHTML='';
        rd=null;txt='';think='';
        ensureStreamDiv();
        updateSidebarState(SESSION,'streaming');
        connectSSE();
      }
    })
    .catch(function(err){
      showToast('error','Failed to send: '+err.message);
    });
  }

  function appendSystemMsg(text){
    var d=mk('div','msg msg-system');
    d.innerHTML='<div class="msg-label" style="color:var(--purple)">System</div><div class="msg-content" style="color:var(--dim);white-space:pre-wrap">'+esc(text)+'</div>';
    $('messages').appendChild(d);sb();
  }

  /* ── Textarea auto-resize ── */
  function resetTextarea(){var ta=$('chat-input');ta.style.height='auto';ta.rows=1;}

  /* ═══════════════════ AUTOCOMPLETE ENGINE ═══════════════════ */
  var ac={
    popup:null, items:[], sel:0, mode:null, query:'',
    cache:{commands:null,agents:null}, fetchTimer:null
  };

  function acInit(){
    ac.popup=$('ac-popup');
  }

  /* ── Detect which autocomplete mode to activate ── */
  function acDetect(){
    var ta=$('chat-input');
    var val=ta.value;
    var cur=ta.selectionStart;
    var before=val.substring(0,cur);

    // / commands: only at start of input, no space yet
    if(/^\/[^\s]*$/.test(before)){
      var q=before.substring(1);
      if(ac.mode!=='cmd'||ac.query!==q){ac.mode='cmd';ac.query=q;acFetchCommands(q);}
      return;
    }

    // >> agent: starts with >> at beginning
    if(/^>>[^\s]*$/.test(before)){
      var q=before.substring(2);
      if(ac.mode!=='agent'||ac.query!==q){ac.mode='agent';ac.query=q;acFetchAgents(q);}
      return;
    }

    // @ file: find last @ not followed by space
    var atIdx=before.lastIndexOf('@');
    if(atIdx>=0){
      var after=before.substring(atIdx+1);
      if(!/\s/.test(after)){
        if(ac.mode!=='file'||ac.query!==after){ac.mode='file';ac.query=after;acFetchFiles(after);}
        return;
      }
    }

    // Nothing matched — close
    if(ac.mode){acClose();}
  }

  /* ── Fetch commands (cached) ── */
  function acFetchCommands(q){
    if(ac.cache.commands){
      acRenderCommands(q,ac.cache.commands);return;
    }
    fetch('/api/autocomplete/commands')
    .then(checkAuth).then(function(r){return r.json();})
    .then(function(data){
      ac.cache.commands=data;
      acRenderCommands(q,data);
    });
  }

  function acRenderCommands(q,cmds){
    var fq=q.toLowerCase();
    var filtered=cmds.filter(function(c){
      return !fq||acFuzzy(c.name.toLowerCase(),fq);
    });
    // Sort: prefix matches first, then fuzzy
    filtered.sort(function(a,b){
      var ap=a.name.toLowerCase().indexOf(fq)===0?0:1;
      var bp=b.name.toLowerCase().indexOf(fq)===0?0:1;
      if(ap!==bp)return ap-bp;
      return a.name.localeCompare(b.name);
    });
    ac.items=filtered.map(function(c){
      return {type:'cmd',name:'/'+c.name,desc:c.desc,value:'/'+c.name+' ',raw:c};
    });
    ac.sel=0;
    acRender('Commands');
  }

  /* ── Fetch agents (cached) ── */
  function acFetchAgents(q){
    if(ac.cache.agents){
      acRenderAgents(q,ac.cache.agents);return;
    }
    fetch('/api/autocomplete/agents')
    .then(checkAuth).then(function(r){return r.json();})
    .then(function(data){
      ac.cache.agents=data;
      acRenderAgents(q,data);
    });
  }

  function acRenderAgents(q,list){
    var fq=q.toLowerCase();
    var filtered=list.filter(function(a){
      return !fq||acFuzzy(a.name.toLowerCase(),fq);
    });
    ac.items=filtered.map(function(a){
      return {type:'agent',name:a.name,desc:a.desc,value:'>>'+a.name+' '};
    });
    ac.sel=0;
    acRender('Agents');
  }

  /* ── Fetch files (debounced, server-side) ── */
  function acFetchFiles(q){
    clearTimeout(ac.fetchTimer);
    ac.fetchTimer=setTimeout(function(){
      fetch('/api/autocomplete/files?project='+encodeURIComponent(PROJECT)+'&q='+encodeURIComponent(q))
      .then(checkAuth).then(function(r){return r.json();})
      .then(function(data){
        if(ac.mode!=='file')return;
        ac.items=(data||[]).map(function(f){
          return {
            type:f.is_dir?'dir':'file',
            name:f.path+(f.is_dir?'/':''),
            desc:f.is_dir?'directory':'',
            value:'@'+f.path+(f.is_dir?'/':' ')
          };
        });
        ac.sel=0;
        acRender('Files');
      });
    },150);
  }

  /* ── Fuzzy match (same as backend) ── */
  function acFuzzy(str,pat){
    var pi=0;
    for(var si=0;si<str.length&&pi<pat.length;si++){
      if(str[si]===pat[pi])pi++;
    }
    return pi===pat.length;
  }

  /* ── Render popup ── */
  function acRender(title){
    if(!ac.items.length){
      ac.popup.innerHTML='<div class="ac-empty">No matches</div>';
      ac.popup.classList.add('visible');
      return;
    }
    var icons={cmd:'/',file:'\\u2756',dir:'\\u25B6',agent:'\\u00BB'};
    var html='<div class="ac-header">'+esc(title)+' <span style="float:right;font-weight:400">\\u2191\\u2193 navigate \\u21E5 select</span></div>';
    var max=Math.min(ac.items.length,10);
    for(var i=0;i<max;i++){
      var it=ac.items[i];
      var cls='ac-item'+(i===ac.sel?' selected':'');
      html+='<div class="'+cls+'" data-idx="'+i+'">';
      html+='<span class="ac-icon '+it.type+'">'+(icons[it.type]||'')+'</span>';
      html+='<span class="ac-name">'+esc(it.name)+'</span>';
      if(it.desc)html+='<span class="ac-desc">'+esc(it.desc)+'</span>';
      html+='</div>';
    }
    if(ac.items.length>max){
      html+='<div class="ac-empty">'+(ac.items.length-max)+' more...</div>';
    }
    ac.popup.innerHTML=html;
    ac.popup.classList.add('visible');

    // Click handlers (pointerdown for touch+mouse, mousedown fallback)
    var els=ac.popup.querySelectorAll('.ac-item');
    for(var i=0;i<els.length;i++){
      (function(idx){
        var handler=function(e){
          e.preventDefault();
          ac.sel=idx;
          acSelect();
        };
        if(window.PointerEvent){
          els[idx].addEventListener('pointerdown',handler);
        }else{
          els[idx].addEventListener('touchend',handler);
          els[idx].addEventListener('mousedown',handler);
        }
      })(i);
    }
  }

  /* ── Select current item ── */
  function acSelect(){
    if(!ac.items.length||ac.sel>=ac.items.length)return;
    var it=ac.items[ac.sel];
    var ta=$('chat-input');
    var val=ta.value;
    var cur=ta.selectionStart;

    if(ac.mode==='cmd'){
      // Replace from start
      ta.value=it.value;
      ta.selectionStart=ta.selectionEnd=it.value.length;
    }else if(ac.mode==='agent'){
      // Replace from start (>>)
      ta.value=it.value;
      ta.selectionStart=ta.selectionEnd=it.value.length;
    }else if(ac.mode==='file'){
      // Replace from the @ position
      var before=val.substring(0,cur);
      var atIdx=before.lastIndexOf('@');
      var after=val.substring(cur);
      ta.value=val.substring(0,atIdx)+it.value+after;
      ta.selectionStart=ta.selectionEnd=atIdx+it.value.length;

      // If directory selected, re-trigger autocomplete
      if(it.type==='dir'){
        ac.mode='file';
        ac.query=it.value.substring(1); // strip @
        acFetchFiles(ac.query);
        return;
      }
    }

    acClose();
    ta.focus();
    ta.dispatchEvent(new Event('input'));
  }

  /* ── Close popup ── */
  function acClose(){
    ac.mode=null;ac.query='';ac.items=[];ac.sel=0;
    if(ac.popup)ac.popup.classList.remove('visible');
    clearTimeout(ac.fetchTimer);
  }

  /* ── Input event: detect + auto-resize ── */
  $('chat-input').addEventListener('input',function(){
    this.style.height='auto';
    this.style.height=Math.min(this.scrollHeight,200)+'px';
    acDetect();
  });
  /* ── keyup fallback for iOS Safari where input event is unreliable ── */
  $('chat-input').addEventListener('keyup',function(){acDetect();});

  /* ── Keydown: autocomplete nav or normal submit ── */
  $('chat-input').addEventListener('keydown',function(e){
    if(ac.mode&&ac.items.length){
      if(e.key==='ArrowDown'||e.key==='Down'){
        e.preventDefault();ac.sel=(ac.sel+1)%Math.min(ac.items.length,10);acRender(ac.mode==='cmd'?'Commands':ac.mode==='agent'?'Agents':'Files');return;
      }
      if(e.key==='ArrowUp'||e.key==='Up'){
        e.preventDefault();ac.sel--;if(ac.sel<0)ac.sel=Math.min(ac.items.length,10)-1;acRender(ac.mode==='cmd'?'Commands':ac.mode==='agent'?'Agents':'Files');return;
      }
      if(e.key==='Tab'){
        e.preventDefault();acSelect();return;
      }
      if(e.key==='Enter'&&!e.shiftKey){
        e.preventDefault();acSelect();return;
      }
      if(e.key==='Escape'){
        e.preventDefault();acClose();return;
      }
    }
    // Normal enter = submit
    if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();sendMessage();}
  });

  /* ── Tool approval ── */
  window.doApproval=function(approved){
    var fd=new FormData();
    fd.append('session',SESSION);
    var url=approved?'/api/chat/approve':'/api/chat/deny';
    fetch(url,{method:'POST',body:fd});
    $('approval-area').innerHTML='';
  };

  /* ── Tool card expand/collapse ── */
  window.toggleToolBody=function(hdr){
    var body=hdr.nextElementSibling;
    var arrow=hdr.querySelector('.tool-expand');
    if(body.style.display==='none'||body.style.display===''){
      body.style.display='block';arrow.innerHTML='&#9660;';
    }else{
      body.style.display='none';arrow.innerHTML='&#9654;';
    }
  };

  /* ── Thinking toggle ── */
  window.toggleThinking=function(el){
    var content=el.nextElementSibling;
    var arrow=el.querySelector('.arrow');
    if(content.style.display==='block'){
      content.style.display='none';arrow.innerHTML='&#9654;';
    }else{
      content.style.display='block';arrow.innerHTML='&#9660;';
    }
  };

  /* ── Panel management ── */
  var currentPanel=null;
  window.togglePanel=function(name){
    var panel=$('side-panel');
    if(currentPanel===name){
      panel.classList.remove('open');currentPanel=null;return;
    }
    currentPanel=name;
    $('panel-title').textContent=name.charAt(0).toUpperCase()+name.slice(1);
    panel.classList.add('open');
    $('panel-content').innerHTML='<div style="color:var(--dim);padding:12px">Loading...</div>';
    fetch('/api/panel/'+name+'?session='+encodeURIComponent(SESSION))
    .then(checkAuth).then(function(r){return r.text();})
    .then(function(html){$('panel-content').innerHTML=html;});
  };
  window.closePanel=function(){
    $('side-panel').classList.remove('open');currentPanel=null;
  };

  /* ── Mobile drawer ── */
  window.toggleMobileDrawer=function(){
    $('mobile-panel-drawer').classList.toggle('open');
    $('mobile-drawer-overlay').classList.toggle('visible');
  };
  window.closeMobileDrawer=function(){
    $('mobile-panel-drawer').classList.remove('open');
    $('mobile-drawer-overlay').classList.remove('visible');
  };

  /* ── Session sidebar (mobile) ── */
  window.toggleSidebar=function(){
    $('session-sidebar').classList.toggle('open');
    $('sidebar-overlay').classList.toggle('visible');
  };
  window.closeSidebar=function(){
    $('session-sidebar').classList.remove('open');
    $('sidebar-overlay').classList.remove('visible');
  };

  /* ── Multi-session: switch session ── */
  window.switchSession=function(id){
    if(id===SESSION)return;
    closeSidebar();
    window.location.href='/chat?project='+encodeURIComponent(PROJECT)+'&session='+encodeURIComponent(id);
  };

  /* ── Multi-session: create session ── */
  window.createSession=function(){
    var fd=new FormData();
    fd.append('project',PROJECT);
    fetch('/api/sessions/create',{method:'POST',body:fd})
    .then(checkAuth).then(function(r){return r.json();})
    .then(function(data){
      if(data.id){
        window.location.href='/chat?project='+encodeURIComponent(PROJECT)+'&session='+encodeURIComponent(data.id);
      }
    })
    .catch(function(err){showToast('error','Failed to create session');});
  };

  /* ── Multi-session: delete session ── */
  window.deleteSession=function(id){
    if(!confirm('Delete this session?'))return;
    var fd=new FormData();
    fd.append('session',id);
    fetch('/api/sessions/delete',{method:'POST',body:fd})
    .then(function(){
      if(id===SESSION){
        window.location.href='/chat?project='+encodeURIComponent(PROJECT);
      }else{
        var el=document.querySelector('.session-item[data-id="'+id+'"]');
        if(el)el.remove();
        showToast('success','Session deleted');
      }
    });
  };

  /* ── Multi-session: rename session ── */
  window.renameSession=function(id){
    var item=document.querySelector('.session-item[data-id="'+id+'"]');
    if(!item)return;
    var titleEl=item.querySelector('.session-title');
    var old=titleEl.textContent;
    var input=document.createElement('input');
    input.type='text';input.value=old;input.className='session-rename-input';
    titleEl.replaceWith(input);input.focus();input.select();

    function save(){
      var val=input.value.trim()||old;
      var span=document.createElement('span');
      span.className='session-title';span.textContent=val;
      input.replaceWith(span);
      if(val!==old){
        var fd=new FormData();
        fd.append('session',id);fd.append('title',val);
        fetch('/api/sessions/rename',{method:'POST',body:fd});
        if(id===SESSION){$('chat-session-title').textContent=val;}
      }
    }
    input.addEventListener('blur',save);
    input.addEventListener('keydown',function(e){
      if(e.key==='Enter'){e.preventDefault();save();}
      if(e.key==='Escape'){input.value=old;save();}
    });
  };

  /* ── Update sidebar session state indicators ── */
  function updateSidebarState(id,state){
    var item=document.querySelector('.session-item[data-id="'+id+'"]');
    if(!item)return;
    item.classList.remove('streaming','approval');
    if(state==='streaming')item.classList.add('streaming');
    else if(state==='approval')item.classList.add('approval');
  }

  /* ── Toast notifications ── */
  window.showToast=function(type,msg){
    var container=$('toast-container');
    if(!container)return;
    var icons={info:'\\u2139',warn:'\\u26A0',error:'\\u2718',success:'\\u2714'};
    var t=mk('div','toast toast-'+type);
    t.innerHTML='<span class="toast-icon">'+(icons[type]||'')+'</span><span class="toast-msg">'+esc(msg)+'</span>';
    container.appendChild(t);
    t.addEventListener('click',function(){t.remove();});
    setTimeout(function(){if(t.parentNode)t.remove();},5000);
  };

  /* ── Model display ── */
  $('status-model').textContent='claude-sonnet-4-6';

  /* ── iOS keyboard: adjust layout when visualViewport changes ── */
  if(window.visualViewport){
    var inputBar=document.querySelector('.chat-input-bar');
    window.visualViewport.addEventListener('resize',function(){
      var vv=window.visualViewport;
      var offset=window.innerHeight-vv.height-vv.offsetTop;
      if(offset>0){
        inputBar.style.paddingBottom=(offset+10)+'px';
      }else{
        inputBar.style.paddingBottom='';
      }
      sb();
    });
  }

  /* ── Init ── */
  acInit();
  sb();
  $('chat-input').focus();
})();
`
