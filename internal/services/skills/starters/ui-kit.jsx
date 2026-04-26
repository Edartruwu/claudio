/**
 * ui-kit.jsx — Common UI components for Claudio design skills
 * Loaded via <script type="text/babel" src="ui-kit.jsx">
 * React + ReactDOM from global scope. No imports.
 */

// ─── CSS Injection ────────────────────────────────────────────────────────────
(function () {
  if (document.getElementById('uk-styles')) return;
  const style = document.createElement('style');
  style.id = 'uk-styles';
  style.textContent = `
    :root {
      --uk-primary: #2563eb;
      --uk-primary-hover: #1d4ed8;
      --uk-bg: #ffffff;
      --uk-surface: #f8fafc;
      --uk-border: #e2e8f0;
      --uk-text: #0f172a;
      --uk-text-muted: #64748b;
      --uk-radius: 8px;
      --uk-radius-sm: 4px;
      --uk-radius-lg: 12px;
      --uk-shadow: 0 1px 3px rgba(0,0,0,0.1), 0 1px 2px rgba(0,0,0,0.06);
      --uk-shadow-lg: 0 10px 25px rgba(0,0,0,0.1), 0 4px 10px rgba(0,0,0,0.04);
    }
    [data-theme="dark"] {
      --uk-primary: #3b82f6;
      --uk-primary-hover: #2563eb;
      --uk-bg: #0f172a;
      --uk-surface: #1e293b;
      --uk-border: #334155;
      --uk-text: #f1f5f9;
      --uk-text-muted: #94a3b8;
      --uk-shadow: 0 1px 3px rgba(0,0,0,0.4), 0 1px 2px rgba(0,0,0,0.3);
      --uk-shadow-lg: 0 10px 25px rgba(0,0,0,0.4), 0 4px 10px rgba(0,0,0,0.2);
    }
    *, *::before, *::after { box-sizing: border-box; }
    .uk-btn {
      display: inline-flex; align-items: center; justify-content: center; gap: 8px;
      border: none; cursor: pointer; font-weight: 500; transition: all 150ms ease;
      border-radius: var(--uk-radius); font-family: inherit;
    }
    .uk-btn:disabled { opacity: 0.5; cursor: not-allowed; }
    .uk-btn:focus-visible { outline: 2px solid var(--uk-primary); outline-offset: 2px; }
    .uk-btn-sm { padding: 6px 12px; font-size: 13px; }
    .uk-btn-md { padding: 9px 16px; font-size: 14px; }
    .uk-btn-lg { padding: 12px 22px; font-size: 16px; }
    .uk-btn-primary { background: var(--uk-primary); color: #fff; }
    .uk-btn-primary:hover:not(:disabled) { background: var(--uk-primary-hover); }
    .uk-btn-secondary { background: var(--uk-surface); color: var(--uk-text); border: 1px solid var(--uk-border); }
    .uk-btn-secondary:hover:not(:disabled) { background: var(--uk-border); }
    .uk-btn-ghost { background: transparent; color: var(--uk-primary); }
    .uk-btn-ghost:hover:not(:disabled) { background: rgba(37,99,235,0.08); }
    .uk-btn-danger { background: #ef4444; color: #fff; }
    .uk-btn-danger:hover:not(:disabled) { background: #dc2626; }
    .uk-btn-success { background: #22c55e; color: #fff; }
    .uk-btn-success:hover:not(:disabled) { background: #16a34a; }

    .uk-input-wrap { display: flex; flex-direction: column; gap: 4px; }
    .uk-input-label { font-size: 13px; font-weight: 500; color: var(--uk-text); }
    .uk-input-inner { display: flex; align-items: center; border: 1px solid var(--uk-border);
      border-radius: var(--uk-radius); background: var(--uk-bg); overflow: hidden; transition: border-color 150ms; }
    .uk-input-inner:focus-within { border-color: var(--uk-primary); box-shadow: 0 0 0 3px rgba(37,99,235,0.1); }
    .uk-input-inner.uk-error { border-color: #ef4444; }
    .uk-input-inner.uk-error:focus-within { box-shadow: 0 0 0 3px rgba(239,68,68,0.1); }
    .uk-input-field { flex: 1; padding: 9px 12px; border: none; outline: none; background: transparent;
      color: var(--uk-text); font-size: 14px; font-family: inherit; }
    .uk-input-field::placeholder { color: var(--uk-text-muted); }
    .uk-input-field:disabled { cursor: not-allowed; opacity: 0.6; }
    .uk-input-affix { padding: 0 10px; color: var(--uk-text-muted); display: flex; align-items: center; }
    .uk-input-hint { font-size: 12px; color: var(--uk-text-muted); }
    .uk-input-error { font-size: 12px; color: #ef4444; }

    .uk-select-wrap { display: flex; flex-direction: column; gap: 4px; }
    .uk-select-inner { position: relative; }
    .uk-select-field { width: 100%; padding: 9px 36px 9px 12px; border: 1px solid var(--uk-border);
      border-radius: var(--uk-radius); background: var(--uk-bg); color: var(--uk-text);
      font-size: 14px; font-family: inherit; outline: none; appearance: none; cursor: pointer;
      transition: border-color 150ms; }
    .uk-select-field:focus { border-color: var(--uk-primary); box-shadow: 0 0 0 3px rgba(37,99,235,0.1); }
    .uk-select-field.uk-error { border-color: #ef4444; }
    .uk-select-chevron { position: absolute; right: 10px; top: 50%; transform: translateY(-50%);
      pointer-events: none; color: var(--uk-text-muted); }

    .uk-checkbox-wrap { display: flex; align-items: center; gap: 8px; cursor: pointer; user-select: none; }
    .uk-checkbox-wrap.uk-disabled { opacity: 0.5; cursor: not-allowed; }
    .uk-checkbox-box { width: 18px; height: 18px; border: 2px solid var(--uk-border);
      border-radius: var(--uk-radius-sm); background: var(--uk-bg); display: flex; align-items: center;
      justify-content: center; flex-shrink: 0; transition: all 150ms; }
    .uk-checkbox-box.uk-checked, .uk-checkbox-box.uk-indeterminate { background: var(--uk-primary); border-color: var(--uk-primary); }
    .uk-checkbox-label { font-size: 14px; color: var(--uk-text); }

    .uk-toggle-wrap { display: flex; align-items: center; gap: 8px; cursor: pointer; user-select: none; }
    .uk-toggle-wrap.uk-disabled { opacity: 0.5; cursor: not-allowed; }
    .uk-toggle-track { width: 44px; height: 24px; border-radius: 12px; background: var(--uk-border);
      position: relative; transition: background 200ms; flex-shrink: 0; }
    .uk-toggle-track.uk-checked { background: var(--uk-primary); }
    .uk-toggle-thumb { position: absolute; top: 3px; left: 3px; width: 18px; height: 18px;
      border-radius: 50%; background: #fff; box-shadow: var(--uk-shadow); transition: transform 200ms; }
    .uk-toggle-track.uk-checked .uk-toggle-thumb { transform: translateX(20px); }
    .uk-toggle-label { font-size: 14px; color: var(--uk-text); }

    .uk-badge { display: inline-flex; align-items: center; font-weight: 500; border-radius: 999px; }
    .uk-badge-sm { padding: 2px 8px; font-size: 11px; }
    .uk-badge-md { padding: 3px 10px; font-size: 12px; }
    .uk-badge-default { background: var(--uk-surface); color: var(--uk-text); border: 1px solid var(--uk-border); }
    .uk-badge-success { background: #dcfce7; color: #166534; }
    .uk-badge-warning { background: #fef9c3; color: #854d0e; }
    .uk-badge-error { background: #fee2e2; color: #991b1b; }
    .uk-badge-info { background: #dbeafe; color: #1e40af; }
    [data-theme="dark"] .uk-badge-success { background: #14532d; color: #86efac; }
    [data-theme="dark"] .uk-badge-warning { background: #713f12; color: #fde68a; }
    [data-theme="dark"] .uk-badge-error { background: #7f1d1d; color: #fca5a5; }
    [data-theme="dark"] .uk-badge-info { background: #1e3a8a; color: #93c5fd; }

    .uk-card { background: var(--uk-bg); border-radius: var(--uk-radius-lg); overflow: hidden; }
    .uk-card-shadow { box-shadow: var(--uk-shadow); }
    .uk-card-border { border: 1px solid var(--uk-border); }
    .uk-card-header { padding: 16px 20px 0; }
    .uk-card-title { font-size: 16px; font-weight: 600; color: var(--uk-text); margin: 0 0 4px; }
    .uk-card-subtitle { font-size: 13px; color: var(--uk-text-muted); margin: 0; }
    .uk-card-body { padding: 16px 20px; }
    .uk-card-footer { padding: 12px 20px; border-top: 1px solid var(--uk-border); background: var(--uk-surface); }

    .uk-modal-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,0.5);
      display: flex; align-items: center; justify-content: center; z-index: 1000;
      animation: uk-fade-in 150ms ease; padding: 16px; }
    .uk-modal-box { background: var(--uk-bg); border-radius: var(--uk-radius-lg);
      box-shadow: var(--uk-shadow-lg); display: flex; flex-direction: column; max-height: 90vh;
      animation: uk-slide-up 200ms ease; width: 100%; }
    .uk-modal-sm { max-width: 400px; }
    .uk-modal-md { max-width: 560px; }
    .uk-modal-lg { max-width: 720px; }
    .uk-modal-xl { max-width: 960px; }
    .uk-modal-header { display: flex; align-items: center; justify-content: space-between;
      padding: 18px 20px; border-bottom: 1px solid var(--uk-border); }
    .uk-modal-title { font-size: 17px; font-weight: 600; color: var(--uk-text); margin: 0; }
    .uk-modal-close { background: none; border: none; cursor: pointer; color: var(--uk-text-muted);
      padding: 4px; border-radius: var(--uk-radius-sm); display: flex; transition: color 150ms; }
    .uk-modal-close:hover { color: var(--uk-text); }
    .uk-modal-body { padding: 20px; overflow-y: auto; flex: 1; }
    .uk-modal-footer { padding: 14px 20px; border-top: 1px solid var(--uk-border);
      display: flex; justify-content: flex-end; gap: 8px; }

    @keyframes uk-fade-in { from { opacity: 0; } to { opacity: 1; } }
    @keyframes uk-slide-up { from { opacity: 0; transform: translateY(20px); } to { opacity: 1; transform: translateY(0); } }
    @keyframes uk-slide-in-right { from { opacity: 0; transform: translateX(40px); } to { opacity: 1; transform: translateX(0); } }
    @keyframes uk-spin { to { transform: rotate(360deg); } }

    .uk-toast-container { position: fixed; bottom: 20px; right: 20px; z-index: 1100;
      display: flex; flex-direction: column; gap: 8px; pointer-events: none; }
    .uk-toast { display: flex; align-items: flex-start; gap: 10px; padding: 12px 16px;
      border-radius: var(--uk-radius); box-shadow: var(--uk-shadow-lg); min-width: 280px; max-width: 380px;
      pointer-events: all; animation: uk-slide-in-right 250ms ease; font-size: 14px; color: var(--uk-text);
      border: 1px solid var(--uk-border); background: var(--uk-bg); }
    .uk-toast-success { border-left: 3px solid #22c55e; }
    .uk-toast-error { border-left: 3px solid #ef4444; }
    .uk-toast-info { border-left: 3px solid var(--uk-primary); }
    .uk-toast-icon { flex-shrink: 0; margin-top: 1px; }
    .uk-toast-msg { flex: 1; line-height: 1.4; }
    .uk-toast-dismiss { background: none; border: none; cursor: pointer; color: var(--uk-text-muted);
      padding: 0; display: flex; transition: color 150ms; }
    .uk-toast-dismiss:hover { color: var(--uk-text); }

    .uk-tabs-list { display: flex; gap: 0; }
    .uk-tabs-underline { border-bottom: 2px solid var(--uk-border); }
    .uk-tabs-pill { background: var(--uk-surface); border-radius: var(--uk-radius); padding: 4px; gap: 4px; }
    .uk-tab-btn { display: flex; align-items: center; gap: 6px; padding: 8px 14px;
      border: none; background: none; cursor: pointer; font-size: 14px; font-weight: 500;
      color: var(--uk-text-muted); border-radius: var(--uk-radius-sm); transition: all 150ms; }
    .uk-tab-btn:focus-visible { outline: 2px solid var(--uk-primary); outline-offset: 2px; }
    .uk-tabs-underline .uk-tab-btn.uk-active { color: var(--uk-primary); border-bottom: 2px solid var(--uk-primary); border-radius: 0; margin-bottom: -2px; }
    .uk-tabs-underline .uk-tab-btn:hover:not(.uk-active) { color: var(--uk-text); }
    .uk-tabs-pill .uk-tab-btn.uk-active { background: var(--uk-bg); color: var(--uk-primary); box-shadow: var(--uk-shadow); }
    .uk-tabs-pill .uk-tab-btn:hover:not(.uk-active) { color: var(--uk-text); }

    .uk-sidebar { display: flex; flex-direction: column; height: 100%; background: var(--uk-surface);
      border-right: 1px solid var(--uk-border); transition: width 200ms ease; overflow: hidden; }
    .uk-sidebar-expanded { width: 240px; }
    .uk-sidebar-collapsed { width: 56px; }
    .uk-sidebar-item { display: flex; align-items: center; gap: 10px; padding: 9px 12px;
      border: none; background: none; cursor: pointer; width: 100%; border-radius: var(--uk-radius);
      color: var(--uk-text-muted); font-size: 14px; font-weight: 500; transition: all 150ms;
      text-decoration: none; white-space: nowrap; }
    .uk-sidebar-item:hover { background: var(--uk-border); color: var(--uk-text); }
    .uk-sidebar-item.uk-active { background: rgba(37,99,235,0.1); color: var(--uk-primary); }
    .uk-sidebar-icon { width: 20px; height: 20px; flex-shrink: 0; }
    .uk-sidebar-label { flex: 1; overflow: hidden; text-overflow: ellipsis; }
    .uk-sidebar-badge { margin-left: auto; }
    .uk-sidebar-children { padding-left: 12px; }
    .uk-sidebar-collapse-btn { display: flex; align-items: center; justify-content: center;
      padding: 10px; border: none; background: none; cursor: pointer; color: var(--uk-text-muted);
      width: 100%; border-top: 1px solid var(--uk-border); margin-top: auto; transition: color 150ms; }
    .uk-sidebar-collapse-btn:hover { color: var(--uk-text); }

    .uk-navbar { display: flex; align-items: center; padding: 0 16px; height: 56px;
      border-bottom: 1px solid var(--uk-border); background: var(--uk-bg); gap: 16px; }
    .uk-navbar.uk-dark { background: var(--uk-primary); border-color: transparent; }
    .uk-navbar-logo { flex-shrink: 0; }
    .uk-navbar-items { display: flex; align-items: center; gap: 4px; flex: 1; }
    .uk-navbar-link { padding: 6px 12px; border-radius: var(--uk-radius-sm); font-size: 14px;
      font-weight: 500; color: var(--uk-text-muted); text-decoration: none; transition: all 150ms; border: none;
      background: none; cursor: pointer; }
    .uk-navbar-link:hover { color: var(--uk-text); background: var(--uk-surface); }
    .uk-navbar-link.uk-active { color: var(--uk-primary); }
    .uk-navbar.uk-dark .uk-navbar-link { color: rgba(255,255,255,0.75); }
    .uk-navbar.uk-dark .uk-navbar-link:hover { color: #fff; background: rgba(255,255,255,0.1); }
    .uk-navbar.uk-dark .uk-navbar-link.uk-active { color: #fff; }
    .uk-navbar-right { margin-left: auto; flex-shrink: 0; }
    .uk-navbar-hamburger { display: none; background: none; border: none; cursor: pointer;
      padding: 6px; color: var(--uk-text); border-radius: var(--uk-radius-sm); }
    .uk-mobile-menu { display: none; flex-direction: column; gap: 2px;
      padding: 8px; border-bottom: 1px solid var(--uk-border); background: var(--uk-bg); }
    .uk-mobile-menu.uk-open { display: flex; }
    @media (max-width: 768px) {
      .uk-navbar-items { display: none; }
      .uk-navbar-hamburger { display: flex; }
    }

    .uk-avatar { display: inline-flex; align-items: center; justify-content: center;
      border-radius: 50%; background: var(--uk-primary); color: #fff; font-weight: 600;
      overflow: hidden; position: relative; flex-shrink: 0; }
    .uk-avatar-sm { width: 28px; height: 28px; font-size: 11px; }
    .uk-avatar-md { width: 36px; height: 36px; font-size: 14px; }
    .uk-avatar-lg { width: 48px; height: 48px; font-size: 18px; }
    .uk-avatar-xl { width: 64px; height: 64px; font-size: 24px; }
    .uk-avatar img { width: 100%; height: 100%; object-fit: cover; }
    .uk-avatar-status { position: absolute; bottom: 0; right: 0; width: 28%; height: 28%;
      border-radius: 50%; border: 2px solid var(--uk-bg); }
    .uk-status-online { background: #22c55e; }
    .uk-status-offline { background: var(--uk-text-muted); }
    .uk-status-away { background: #f59e0b; }

    .uk-dropdown { position: relative; display: inline-block; }
    .uk-dropdown-menu { position: absolute; top: calc(100% + 6px); z-index: 200;
      background: var(--uk-bg); border: 1px solid var(--uk-border); border-radius: var(--uk-radius);
      box-shadow: var(--uk-shadow-lg); min-width: 180px; padding: 4px; }
    .uk-dropdown-left { left: 0; }
    .uk-dropdown-right { right: 0; }
    .uk-dropdown-item { display: flex; align-items: center; gap: 8px; padding: 7px 10px;
      border: none; background: none; cursor: pointer; font-size: 14px; color: var(--uk-text);
      border-radius: var(--uk-radius-sm); width: 100%; text-align: left; transition: background 120ms; }
    .uk-dropdown-item:hover { background: var(--uk-surface); }
    .uk-dropdown-item.uk-danger { color: #ef4444; }
    .uk-dropdown-item.uk-danger:hover { background: #fee2e2; }
    .uk-dropdown-divider { height: 1px; background: var(--uk-border); margin: 4px 0; }

    .uk-spinner { animation: uk-spin 700ms linear infinite; }
    .uk-spinner-sm { width: 14px; height: 14px; }
    .uk-spinner-md { width: 20px; height: 20px; }
    .uk-spinner-lg { width: 32px; height: 32px; }
  `;
  document.head.appendChild(style);
})();

// ─── Spinner ──────────────────────────────────────────────────────────────────
function Spinner({ size = 'md', color = 'currentColor' }) {
  const sz = { sm: 14, md: 20, lg: 32 }[size] || 20;
  return (
    <svg className={`uk-spinner uk-spinner-${size}`} width={sz} height={sz} viewBox="0 0 24 24" fill="none" aria-label="Loading">
      <circle cx="12" cy="12" r="10" stroke={color} strokeWidth="3" strokeOpacity="0.25" />
      <path d="M12 2a10 10 0 0 1 10 10" stroke={color} strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}

// ─── Button ───────────────────────────────────────────────────────────────────
function Button({ variant = 'primary', size = 'md', loading = false, disabled = false, icon, onClick, children, type = 'button', style }) {
  return (
    <button
      type={type}
      className={`uk-btn uk-btn-${variant} uk-btn-${size}`}
      disabled={disabled || loading}
      onClick={onClick}
      style={style}
      aria-busy={loading}
    >
      {loading ? <Spinner size="sm" color="currentColor" /> : icon}
      {children}
    </button>
  );
}

// ─── Input ────────────────────────────────────────────────────────────────────
function Input({ label, placeholder, value, onChange, error, hint, prefix, suffix, disabled = false, type = 'text', id, name }) {
  const inputId = id || (label ? label.toLowerCase().replace(/\s+/g, '-') : undefined);
  return (
    <div className="uk-input-wrap">
      {label && <label className="uk-input-label" htmlFor={inputId}>{label}</label>}
      <div className={`uk-input-inner${error ? ' uk-error' : ''}`}>
        {prefix && <span className="uk-input-affix">{prefix}</span>}
        <input
          id={inputId}
          name={name}
          type={type}
          className="uk-input-field"
          placeholder={placeholder}
          value={value}
          onChange={onChange}
          disabled={disabled}
          aria-invalid={!!error}
          aria-describedby={error ? `${inputId}-error` : hint ? `${inputId}-hint` : undefined}
        />
        {suffix && <span className="uk-input-affix">{suffix}</span>}
      </div>
      {error && <span id={`${inputId}-error`} className="uk-input-error" role="alert">{error}</span>}
      {hint && !error && <span id={`${inputId}-hint`} className="uk-input-hint">{hint}</span>}
    </div>
  );
}

// ─── Select ───────────────────────────────────────────────────────────────────
function Select({ label, options = [], value, onChange, error, disabled = false, id }) {
  const selectId = id || (label ? label.toLowerCase().replace(/\s+/g, '-') : undefined);
  return (
    <div className="uk-select-wrap">
      {label && <label className="uk-input-label" htmlFor={selectId}>{label}</label>}
      <div className="uk-select-inner">
        <select
          id={selectId}
          className={`uk-select-field${error ? ' uk-error' : ''}`}
          value={value}
          onChange={onChange}
          disabled={disabled}
          aria-invalid={!!error}
        >
          {options.map(opt => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
        <span className="uk-select-chevron">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </span>
      </div>
      {error && <span className="uk-input-error" role="alert">{error}</span>}
    </div>
  );
}

// ─── Checkbox ─────────────────────────────────────────────────────────────────
function Checkbox({ label, checked, onChange, disabled = false, indeterminate = false }) {
  const ref = React.useRef(null);
  React.useEffect(() => {
    if (ref.current) ref.current.indeterminate = indeterminate;
  }, [indeterminate]);

  return (
    <label className={`uk-checkbox-wrap${disabled ? ' uk-disabled' : ''}`}>
      <input ref={ref} type="checkbox" checked={checked} onChange={onChange} disabled={disabled} style={{ position: 'absolute', opacity: 0, width: 0, height: 0 }} aria-checked={indeterminate ? 'mixed' : checked} />
      <span className={`uk-checkbox-box${checked || indeterminate ? (indeterminate ? ' uk-indeterminate' : ' uk-checked') : ''}`}>
        {indeterminate ? (
          <svg width="10" height="2" viewBox="0 0 10 2" fill="none"><rect x="0" y="0" width="10" height="2" rx="1" fill="white" /></svg>
        ) : checked ? (
          <svg width="10" height="8" viewBox="0 0 10 8" fill="none"><polyline points="1,4 4,7 9,1" stroke="white" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" /></svg>
        ) : null}
      </span>
      {label && <span className="uk-checkbox-label">{label}</span>}
    </label>
  );
}

// ─── Toggle ───────────────────────────────────────────────────────────────────
function Toggle({ checked, onChange, label, disabled = false }) {
  return (
    <label className={`uk-toggle-wrap${disabled ? ' uk-disabled' : ''}`}>
      <input type="checkbox" checked={checked} onChange={onChange} disabled={disabled} style={{ position: 'absolute', opacity: 0, width: 0, height: 0 }} role="switch" aria-checked={checked} />
      <span className={`uk-toggle-track${checked ? ' uk-checked' : ''}`}>
        <span className="uk-toggle-thumb" />
      </span>
      {label && <span className="uk-toggle-label">{label}</span>}
    </label>
  );
}

// ─── Badge ────────────────────────────────────────────────────────────────────
function Badge({ variant = 'default', size = 'md', children }) {
  return (
    <span className={`uk-badge uk-badge-${size} uk-badge-${variant}`}>{children}</span>
  );
}

// ─── Card ─────────────────────────────────────────────────────────────────────
function Card({ title, subtitle, footer, padding = '20px', children, shadow = true, border = true, style }) {
  const cls = ['uk-card', shadow && 'uk-card-shadow', border && 'uk-card-border'].filter(Boolean).join(' ');
  return (
    <div className={cls} style={style}>
      {(title || subtitle) && (
        <div className="uk-card-header">
          {title && <h3 className="uk-card-title">{title}</h3>}
          {subtitle && <p className="uk-card-subtitle">{subtitle}</p>}
        </div>
      )}
      <div className="uk-card-body" style={{ padding }}>{children}</div>
      {footer && <div className="uk-card-footer">{footer}</div>}
    </div>
  );
}

// ─── Modal ────────────────────────────────────────────────────────────────────
function Modal({ open, onClose, title, size = 'md', children, footer }) {
  const boxRef = React.useRef(null);

  React.useEffect(() => {
    if (!open) return;
    const prev = document.activeElement;
    const focusable = () => boxRef.current
      ? Array.from(boxRef.current.querySelectorAll('button,input,select,textarea,a,[tabindex]:not([tabindex="-1"])'))
      : [];

    const first = focusable()[0];
    if (first) first.focus();

    const handleKey = (e) => {
      if (e.key === 'Escape') { onClose(); return; }
      if (e.key === 'Tab') {
        const els = focusable();
        if (!els.length) return;
        if (e.shiftKey) {
          if (document.activeElement === els[0]) { e.preventDefault(); els[els.length - 1].focus(); }
        } else {
          if (document.activeElement === els[els.length - 1]) { e.preventDefault(); els[0].focus(); }
        }
      }
    };
    document.addEventListener('keydown', handleKey);
    return () => { document.removeEventListener('keydown', handleKey); if (prev) prev.focus(); };
  }, [open, onClose]);

  if (!open) return null;
  return ReactDOM.createPortal(
    <div className="uk-modal-backdrop" role="dialog" aria-modal="true" aria-label={title} onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className={`uk-modal-box uk-modal-${size}`} ref={boxRef}>
        <div className="uk-modal-header">
          <h2 className="uk-modal-title">{title}</h2>
          <button className="uk-modal-close" onClick={onClose} aria-label="Close modal">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
          </button>
        </div>
        <div className="uk-modal-body">{children}</div>
        {footer && <div className="uk-modal-footer">{footer}</div>}
      </div>
    </div>,
    document.body
  );
}

// ─── Toast / useToast ─────────────────────────────────────────────────────────
const _toastListeners = [];
let _toastId = 0;

function _emitToast(type, message) {
  const item = { id: ++_toastId, type, message };
  _toastListeners.forEach(fn => fn(item));
}

const toast = {
  success: (msg) => _emitToast('success', msg),
  error:   (msg) => _emitToast('error', msg),
  info:    (msg) => _emitToast('info', msg),
};

function useToast() {
  return { toast };
}

const _toastIcons = {
  success: <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#22c55e" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="20 6 9 17 4 12" /></svg>,
  error:   <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#ef4444" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><line x1="15" y1="9" x2="9" y2="15" /><line x1="9" y1="9" x2="15" y2="15" /></svg>,
  info:    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#2563eb" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" /></svg>,
};

function ToastContainer() {
  const [items, setItems] = React.useState([]);

  React.useEffect(() => {
    const handler = (item) => {
      setItems(prev => [...prev, item]);
      setTimeout(() => setItems(prev => prev.filter(t => t.id !== item.id)), 3000);
    };
    _toastListeners.push(handler);
    return () => { const i = _toastListeners.indexOf(handler); if (i > -1) _toastListeners.splice(i, 1); };
  }, []);

  return ReactDOM.createPortal(
    <div className="uk-toast-container" aria-live="polite" aria-atomic="false">
      {items.map(item => (
        <div key={item.id} className={`uk-toast uk-toast-${item.type}`} role="status">
          <span className="uk-toast-icon">{_toastIcons[item.type]}</span>
          <span className="uk-toast-msg">{item.message}</span>
          <button className="uk-toast-dismiss" onClick={() => setItems(prev => prev.filter(t => t.id !== item.id))} aria-label="Dismiss">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
          </button>
        </div>
      ))}
    </div>,
    document.body
  );
}

// ─── Tabs ─────────────────────────────────────────────────────────────────────
function Tabs({ tabs = [], activeTab, onChange, variant = 'underline' }) {
  return (
    <div className={`uk-tabs-list uk-tabs-${variant}`} role="tablist">
      {tabs.map(tab => (
        <button
          key={tab.id}
          role="tab"
          aria-selected={activeTab === tab.id}
          className={`uk-tab-btn${activeTab === tab.id ? ' uk-active' : ''}`}
          onClick={() => onChange(tab.id)}
        >
          {tab.icon && tab.icon}
          {tab.label}
        </button>
      ))}
    </div>
  );
}

// ─── Sidebar ──────────────────────────────────────────────────────────────────
function Sidebar({ items = [], activeId, onSelect, collapsed = false, onCollapse }) {
  const [expanded, setExpanded] = React.useState({});

  const toggle = (id) => setExpanded(prev => ({ ...prev, [id]: !prev[id] }));

  const renderItem = (item, depth = 0) => {
    const hasChildren = item.children && item.children.length > 0;
    const isExpanded = expanded[item.id];
    return (
      <React.Fragment key={item.id}>
        <button
          className={`uk-sidebar-item${activeId === item.id ? ' uk-active' : ''}`}
          onClick={() => { hasChildren ? toggle(item.id) : onSelect(item.id); }}
          title={collapsed ? item.label : undefined}
          aria-expanded={hasChildren ? isExpanded : undefined}
          style={{ paddingLeft: collapsed ? undefined : `${12 + depth * 12}px` }}
        >
          {item.icon && <span className="uk-sidebar-icon">{item.icon}</span>}
          {!collapsed && <span className="uk-sidebar-label">{item.label}</span>}
          {!collapsed && item.badge != null && <Badge variant="info" size="sm">{item.badge}</Badge>}
          {!collapsed && hasChildren && (
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" style={{ marginLeft: 'auto', transform: isExpanded ? 'rotate(90deg)' : 'none', transition: 'transform 150ms' }}>
              <polyline points="9 18 15 12 9 6" />
            </svg>
          )}
        </button>
        {!collapsed && hasChildren && isExpanded && (
          <div className="uk-sidebar-children">
            {item.children.map(child => renderItem(child, depth + 1))}
          </div>
        )}
      </React.Fragment>
    );
  };

  return (
    <nav className={`uk-sidebar ${collapsed ? 'uk-sidebar-collapsed' : 'uk-sidebar-expanded'}`} aria-label="Sidebar navigation">
      <div style={{ padding: '8px', flex: 1, overflowY: 'auto' }}>
        {items.map(item => renderItem(item))}
      </div>
      {onCollapse && (
        <button className="uk-sidebar-collapse-btn" onClick={onCollapse} aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" style={{ transform: collapsed ? 'rotate(180deg)' : 'none', transition: 'transform 200ms' }}>
            <polyline points="15 18 9 12 15 6" />
          </svg>
        </button>
      )}
    </nav>
  );
}

// ─── NavBar ───────────────────────────────────────────────────────────────────
function NavBar({ logo, items = [], rightSlot, dark = false }) {
  const [mobileOpen, setMobileOpen] = React.useState(false);

  return (
    <>
      <header className={`uk-navbar${dark ? ' uk-dark' : ''}`} role="banner">
        {logo && <div className="uk-navbar-logo">{logo}</div>}
        <nav className="uk-navbar-items" aria-label="Primary navigation">
          {items.map((item, i) => (
            <a
              key={i}
              href={item.href || '#'}
              className={`uk-navbar-link${item.active ? ' uk-active' : ''}`}
              aria-current={item.active ? 'page' : undefined}
            >
              {item.label}
            </a>
          ))}
        </nav>
        {rightSlot && <div className="uk-navbar-right">{rightSlot}</div>}
        <button
          className="uk-navbar-hamburger"
          onClick={() => setMobileOpen(v => !v)}
          aria-expanded={mobileOpen}
          aria-label="Toggle navigation menu"
        >
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
            {mobileOpen
              ? <><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></>
              : <><line x1="3" y1="6" x2="21" y2="6" /><line x1="3" y1="12" x2="21" y2="12" /><line x1="3" y1="18" x2="21" y2="18" /></>
            }
          </svg>
        </button>
      </header>
      <nav className={`uk-mobile-menu${mobileOpen ? ' uk-open' : ''}`} aria-label="Mobile navigation">
        {items.map((item, i) => (
          <a
            key={i}
            href={item.href || '#'}
            className={`uk-navbar-link${item.active ? ' uk-active' : ''}`}
            aria-current={item.active ? 'page' : undefined}
            onClick={() => setMobileOpen(false)}
            style={{ color: 'var(--uk-text)' }}
          >
            {item.label}
          </a>
        ))}
      </nav>
    </>
  );
}

// ─── Avatar ───────────────────────────────────────────────────────────────────
function Avatar({ src, name, size = 'md', status }) {
  const initials = name
    ? name.trim().split(/\s+/).map(w => w[0]).slice(0, 2).join('').toUpperCase()
    : '?';
  return (
    <span className={`uk-avatar uk-avatar-${size}`} aria-label={name}>
      {src ? <img src={src} alt={name || 'Avatar'} /> : initials}
      {status && <span className={`uk-avatar-status uk-status-${status}`} aria-label={status} />}
    </span>
  );
}

// ─── Dropdown ─────────────────────────────────────────────────────────────────
function Dropdown({ trigger, items = [], align = 'left' }) {
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef(null);

  React.useEffect(() => {
    if (!open) return;
    const handler = (e) => { if (ref.current && !ref.current.contains(e.target)) setOpen(false); };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  React.useEffect(() => {
    if (!open) return;
    const handler = (e) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open]);

  return (
    <div className="uk-dropdown" ref={ref}>
      <div onClick={() => setOpen(v => !v)} style={{ display: 'inline-flex' }}>
        {trigger}
      </div>
      {open && (
        <div className={`uk-dropdown-menu uk-dropdown-${align}`} role="menu">
          {items.map((item, i) =>
            item.divider ? (
              <div key={i} className="uk-dropdown-divider" role="separator" />
            ) : (
              <button
                key={i}
                className={`uk-dropdown-item${item.danger ? ' uk-danger' : ''}`}
                role="menuitem"
                onClick={() => { setOpen(false); item.onClick && item.onClick(); }}
              >
                {item.icon && <span>{item.icon}</span>}
                {item.label}
              </button>
            )
          )}
        </div>
      )}
    </div>
  );
}
