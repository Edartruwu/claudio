/**
 * design-canvas.jsx — Pan/zoom Figma-style artboard canvas
 *
 * Exports: DesignCanvas, DCSection, DCArtboard, DCPostIt, DCViewport, DCFocusOverlay
 *
 * Usage:
 *   <DesignCanvas>
 *     <DCSection id="screens" title="My Screens">
 *       <DCArtboard id="home" label="01 · Home" width={1440} height={900}>
 *         <MyContent />
 *       </DCArtboard>
 *     </DCSection>
 *   </DesignCanvas>
 *
 * No imports — React/ReactDOM on global scope. Loaded via <script type="text/babel">.
 */

const { useState, useEffect, useRef, useMemo, useCallback, createContext, useContext } = React;
const { createPortal } = ReactDOM;

/* ─── Design tokens ─── */
const DC = {
  bg:        '#f0eee9',
  grid:      'rgba(0,0,0,0.06)',
  label:     '#6b5e54',
  labelHov:  '#4a3f37',
  accent:    '#c96442',
  card:      '#ffffff',
  shadow:    '0 2px 12px rgba(0,0,0,0.08), 0 1px 3px rgba(0,0,0,0.06)',
  shadowLg:  '0 8px 32px rgba(0,0,0,0.12), 0 2px 8px rgba(0,0,0,0.08)',
  radius:    2,
  overlay:   'rgba(30,28,25,0.72)',
  postIt:    '#fef3c7',
  postItBdr: '#f59e0b',
};

/* ─── CSS injection (once) ─── */
if (!document.getElementById('dc-styles')) {
  const s = document.createElement('style');
  s.id = 'dc-styles';
  s.textContent = `
    .dc-viewport { position:relative; width:100%; height:100vh; overflow:hidden;
      background:${DC.bg}; overscroll-behavior:none; touch-action:none; cursor:grab; }
    .dc-viewport.dc-grabbing { cursor:grabbing; }
    .dc-world { position:absolute; top:0; left:0; transform-origin:0 0; will-change:transform; }
    .dc-section { margin-bottom:64px; }
    .dc-section-title { font:600 18px/1.4 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;
      color:${DC.label}; margin-bottom:4px; }
    .dc-section-subtitle { font:400 13px/1.4 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;
      color:${DC.label}; opacity:0.7; margin-bottom:20px; }
    .dc-artboard-row { display:flex; align-items:flex-start; }
    .dc-frame-wrap { position:relative; flex-shrink:0; transition:transform 200ms ease; }
    .dc-frame-label { display:flex; align-items:center; gap:6px; margin-bottom:8px;
      font:500 13px/1.3 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif; color:${DC.label}; }
    .dc-grip { cursor:grab; opacity:0.4; transition:opacity 150ms; user-select:none; }
    .dc-grip:hover { opacity:0.8; }
    .dc-frame-label .dc-expand { opacity:0; transition:opacity 150ms; cursor:pointer;
      background:none; border:none; color:${DC.label}; font-size:16px; padding:2px 4px; margin-left:auto; }
    .dc-frame-wrap:hover .dc-expand { opacity:0.6; }
    .dc-frame-wrap:hover .dc-expand:hover { opacity:1; color:${DC.accent}; }
    .dc-frame { background:${DC.card}; border-radius:${DC.radius}px; box-shadow:${DC.shadow};
      overflow:hidden; position:relative; }
    .dc-frame.dc-dragging { box-shadow:${DC.shadowLg}; border:2px solid ${DC.accent};
      transform:scale(1.02); z-index:50; }
    .dc-editable { outline:none; border-radius:3px; padding:1px 4px; transition:background 150ms; cursor:text; }
    .dc-editable:hover { background:rgba(0,0,0,0.05); }
    .dc-editable:focus { background:rgba(201,100,66,0.1); box-shadow:0 0 0 2px ${DC.accent}; }
    .dc-focus-overlay { position:fixed; inset:0; z-index:100; display:flex; align-items:center;
      justify-content:center; flex-direction:column; background:${DC.overlay};
      backdrop-filter:blur(14px); -webkit-backdrop-filter:blur(14px); }
    .dc-focus-card { background:${DC.card}; border-radius:${DC.radius}px; box-shadow:${DC.shadowLg};
      overflow:hidden; position:relative; }
    .dc-focus-nav { position:absolute; top:50%; transform:translateY(-50%); background:rgba(255,255,255,0.9);
      border:none; border-radius:50%; width:40px; height:40px; font-size:20px; cursor:pointer;
      display:flex; align-items:center; justify-content:center; box-shadow:${DC.shadow};
      transition:background 150ms; color:${DC.labelHov}; }
    .dc-focus-nav:hover { background:#fff; }
    .dc-focus-nav[data-dir=left] { left:24px; }
    .dc-focus-nav[data-dir=right] { right:24px; }
    .dc-focus-close { position:absolute; top:16px; right:20px; background:none; border:none;
      color:rgba(255,255,255,0.7); font-size:24px; cursor:pointer; z-index:2; }
    .dc-focus-close:hover { color:#fff; }
    .dc-focus-dots { display:flex; gap:6px; margin-top:16px; }
    .dc-focus-dot { width:8px; height:8px; border-radius:50%; background:rgba(255,255,255,0.3);
      border:none; padding:0; cursor:pointer; transition:background 150ms; }
    .dc-focus-dot.active { background:rgba(255,255,255,0.9); }
    .dc-focus-info { color:rgba(255,255,255,0.7); font:400 13px/1.4 -apple-system,sans-serif;
      margin-top:8px; text-align:center; }
    .dc-focus-dropdown { position:absolute; top:16px; left:20px; z-index:2; }
    .dc-focus-dropdown select { background:rgba(255,255,255,0.15); color:#fff; border:1px solid rgba(255,255,255,0.2);
      border-radius:6px; padding:6px 10px; font:400 13px/1.3 -apple-system,sans-serif; cursor:pointer;
      -webkit-appearance:none; appearance:none; }
    .dc-focus-dropdown select option { color:#333; background:#fff; }
    .dc-postit { position:absolute; padding:12px 14px; background:${DC.postIt};
      border-left:3px solid ${DC.postItBdr}; border-radius:2px; box-shadow:0 1px 4px rgba(0,0,0,0.1);
      font:400 13px/1.5 -apple-system,sans-serif; color:#92400e; max-width:220px; z-index:10; }
  `;
  document.head.appendChild(s);
}

/* ─── Dot-grid SVG background (120x120 tile) ─── */
const dotGridBg = (() => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="120" height="120">
    <circle cx="60" cy="60" r="1" fill="${DC.grid}"/>
  </svg>`;
  return `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
})();

/* ─── Context ─── */
const DCCtx = createContext(null);

/* ─── Persistence bridge ─── */
const STATE_FILE = '.design-canvas.state.json';

function loadState() {
  try {
    if (window.omelette?.readFile) {
      const raw = window.omelette.readFile(STATE_FILE);
      return raw ? JSON.parse(raw) : {};
    }
  } catch (_) {}
  try { return JSON.parse(localStorage.getItem('dc-state') || '{}'); } catch (_) { return {}; }
}

function saveState(state) {
  try {
    if (window.omelette?.writeFile) { window.omelette.writeFile(STATE_FILE, JSON.stringify(state, null, 2)); return; }
  } catch (_) {}
  try { localStorage.setItem('dc-state', JSON.stringify(state)); } catch (_) {}
}

/* ═══════════════════════════════════════════
   DCEditable — contentEditable inline text
   ═══════════════════════════════════════════ */
function DCEditable({ value, onChange, style, className = '' }) {
  const ref = useRef(null);
  const commit = useCallback(() => {
    if (ref.current && onChange) {
      const text = ref.current.textContent.trim();
      if (text && text !== value) onChange(text);
      else ref.current.textContent = value; // revert if empty
    }
  }, [value, onChange]);

  return React.createElement('span', {
    ref,
    className: `dc-editable ${className}`,
    contentEditable: true,
    suppressContentEditableWarning: true,
    style,
    onBlur: commit,
    onKeyDown: (e) => { if (e.key === 'Enter') { e.preventDefault(); ref.current?.blur(); } },
    onPointerDown: (e) => e.stopPropagation(),
  }, value);
}

/* ═══════════════════════════════════════════
   DCViewport — pan/zoom container
   ═══════════════════════════════════════════ */
function DCViewport({ children, initialX = -200, initialY = -100, initialScale = 0.6 }) {
  const vpRef = useRef(null);
  const worldRef = useRef(null);
  // Transform stored in ref for 60fps DOM writes (no React re-render)
  const tf = useRef({ x: initialX, y: initialY, s: initialScale });
  const dragging = useRef(false);
  const dragStart = useRef({ x: 0, y: 0 });
  const minScale = 0.1, maxScale = 8;

  const applyTransform = useCallback(() => {
    const { x, y, s } = tf.current;
    if (worldRef.current) {
      worldRef.current.style.transform = `translate3d(${x}px,${y}px,0) scale(${s})`;
    }
  }, []);

  useEffect(() => { applyTransform(); }, [applyTransform]);

  /* ── Wheel: trackpad pan (deltaX/Y) + pinch/ctrl+wheel zoom ── */
  const onWheel = useCallback((e) => {
    e.preventDefault();
    const { x, y, s } = tf.current;
    if (e.ctrlKey || e.metaKey) {
      // Pinch-zoom (trackpad) or ctrl+wheel
      const rect = vpRef.current.getBoundingClientRect();
      const mx = e.clientX - rect.left;
      const my = e.clientY - rect.top;
      const delta = -e.deltaY * 0.01;
      const ns = Math.min(maxScale, Math.max(minScale, s * (1 + delta)));
      // Keep world point under cursor fixed
      tf.current.x = mx - (mx - x) * (ns / s);
      tf.current.y = my - (my - y) * (ns / s);
      tf.current.s = ns;
    } else {
      // Two-finger scroll = pan
      tf.current.x = x - e.deltaX;
      tf.current.y = y - e.deltaY;
    }
    applyTransform();
  }, [applyTransform]);

  /* ── Mouse drag: middle-click or primary on background ── */
  const onPointerDown = useCallback((e) => {
    // Only start pan if clicking on viewport bg (not artboard content)
    if (e.button === 1 || (e.button === 0 && e.target === vpRef.current)) {
      e.preventDefault();
      dragging.current = true;
      dragStart.current = { x: e.clientX - tf.current.x, y: e.clientY - tf.current.y };
      vpRef.current?.classList.add('dc-grabbing');
      vpRef.current?.setPointerCapture(e.pointerId);
    }
  }, []);

  const onPointerMove = useCallback((e) => {
    if (!dragging.current) return;
    tf.current.x = e.clientX - dragStart.current.x;
    tf.current.y = e.clientY - dragStart.current.y;
    applyTransform();
  }, [applyTransform]);

  const onPointerUp = useCallback((e) => {
    if (!dragging.current) return;
    dragging.current = false;
    vpRef.current?.classList.remove('dc-grabbing');
    vpRef.current?.releasePointerCapture(e.pointerId);
  }, []);

  /* ── Safari native pinch gestures ── */
  useEffect(() => {
    const el = vpRef.current;
    if (!el) return;
    let gestureScale = 1;
    const onGS = (e) => { e.preventDefault(); gestureScale = 1; };
    const onGC = (e) => {
      e.preventDefault();
      const rect = el.getBoundingClientRect();
      const mx = (rect.width / 2);
      const my = (rect.height / 2);
      const { x, y, s } = tf.current;
      const ns = Math.min(maxScale, Math.max(minScale, s * e.scale / gestureScale));
      tf.current.x = mx - (mx - x) * (ns / s);
      tf.current.y = my - (my - y) * (ns / s);
      tf.current.s = ns;
      gestureScale = e.scale;
      applyTransform();
    };
    const onGE = (e) => { e.preventDefault(); };
    el.addEventListener('gesturestart', onGS);
    el.addEventListener('gesturechange', onGC);
    el.addEventListener('gestureend', onGE);
    return () => {
      el.removeEventListener('gesturestart', onGS);
      el.removeEventListener('gesturechange', onGC);
      el.removeEventListener('gestureend', onGE);
    };
  }, [applyTransform]);

  /* ── Passive: false for wheel to allow preventDefault ── */
  useEffect(() => {
    const el = vpRef.current;
    if (!el) return;
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [onWheel]);

  return React.createElement('div', {
    ref: vpRef,
    className: 'dc-viewport',
    style: { backgroundImage: dotGridBg },
    onPointerDown,
    onPointerMove,
    onPointerUp,
  },
    React.createElement('div', { ref: worldRef, className: 'dc-world' }, children)
  );
}

/* ═══════════════════════════════════════════
   DCArtboard — marker component (renders null)
   Props read by DCSection: id, label, width, height, children
   ═══════════════════════════════════════════ */
function DCArtboard() { return null; }

/* ═══════════════════════════════════════════
   DCArtboardFrame — rendered card for each artboard
   ═══════════════════════════════════════════ */
function DCArtboardFrame({ id, label, width, height, children, index, total, onFocus, onReorder }) {
  const frameRef = useRef(null);
  const [editLabel, setEditLabel] = useState(label);
  const [isDragging, setIsDragging] = useState(false);

  /* ── Grip drag to reorder ── */
  const onGripDown = useCallback((e) => {
    e.preventDefault();
    e.stopPropagation();
    const wrap = frameRef.current;
    if (!wrap) return;
    const parent = wrap.parentElement;
    const siblings = Array.from(parent.children);
    const idx = siblings.indexOf(wrap);
    const rects = siblings.map(s => s.getBoundingClientRect());
    const startX = e.clientX;
    setIsDragging(true);

    let currentIdx = idx;

    const onMove = (ev) => {
      const dx = ev.clientX - startX;
      wrap.style.transform = `translateX(${dx}px) scale(1.02)`;
      wrap.style.zIndex = '50';
      // Check if crossed midpoint of a sibling
      siblings.forEach((sib, i) => {
        if (i === idx) return;
        const mid = rects[i].left + rects[i].width / 2;
        const myCenter = rects[idx].left + rects[idx].width / 2 + dx;
        if (i < idx && myCenter < mid) {
          sib.style.transform = `translateX(${rects[idx].width + 48}px)`;
          currentIdx = Math.min(currentIdx, i);
        } else if (i > idx && myCenter > mid) {
          sib.style.transform = `translateX(-${rects[idx].width + 48}px)`;
          currentIdx = Math.max(currentIdx, i);
        } else {
          sib.style.transform = '';
        }
      });
    };

    const onUp = () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
      // Reset transforms
      siblings.forEach(s => { s.style.transform = ''; s.style.zIndex = ''; });
      setIsDragging(false);
      if (currentIdx !== idx && onReorder) onReorder(idx, currentIdx);
    };

    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
  }, [onReorder]);

  return React.createElement('div', {
    ref: frameRef,
    className: 'dc-frame-wrap',
    'data-artboard-id': id,
  },
    // Label row above card
    React.createElement('div', { className: 'dc-frame-label' },
      React.createElement('span', {
        className: 'dc-grip',
        onPointerDown: onGripDown,
        title: 'Drag to reorder',
        'aria-label': 'Drag to reorder artboard',
      }, '⠿'),
      React.createElement(DCEditable, {
        value: editLabel,
        onChange: setEditLabel,
        style: { cursor: 'pointer' },
      }),
      React.createElement('button', {
        className: 'dc-expand',
        onClick: () => onFocus && onFocus(index),
        title: 'Expand artboard',
        'aria-label': `Expand ${editLabel}`,
      }, '⤢')
    ),
    // Card
    React.createElement('div', {
      className: `dc-frame${isDragging ? ' dc-dragging' : ''}`,
      style: { width, height },
      onClick: () => onFocus && onFocus(index),
    }, children)
  );
}

/* ═══════════════════════════════════════════
   DCSection — groups artboards, manages order
   ═══════════════════════════════════════════ */
function DCSection({ id, title, subtitle, gap = 48, children }) {
  const ctx = useContext(DCCtx);
  const [editTitle, setEditTitle] = useState(title);

  // Extract DCArtboard children props
  const artboards = useMemo(() => {
    const items = [];
    React.Children.forEach(children, (child) => {
      if (child && child.type === DCArtboard) {
        items.push({ ...child.props });
      }
    });
    return items;
  }, [children]);

  // Persisted order: array of artboard IDs
  const [order, setOrder] = useState(() => {
    const saved = ctx?.state?.[id]?.order;
    if (saved && Array.isArray(saved)) return saved;
    return artboards.map(a => a.id);
  });

  // Sync order when artboards change (new ones appended)
  useEffect(() => {
    const ids = artboards.map(a => a.id);
    const missing = ids.filter(i => !order.includes(i));
    if (missing.length) setOrder(prev => [...prev, ...missing]);
  }, [artboards, order]);

  // Ordered artboards
  const ordered = useMemo(() => {
    const map = {};
    artboards.forEach(a => { map[a.id] = a; });
    return order.filter(id => map[id]).map(id => map[id]);
  }, [artboards, order]);

  // Persist on order change
  useEffect(() => {
    if (ctx?.saveSection) ctx.saveSection(id, { order });
  }, [order, id, ctx]);

  const handleReorder = useCallback((fromIdx, toIdx) => {
    setOrder(prev => {
      const next = [...prev];
      const [moved] = next.splice(fromIdx, 1);
      next.splice(toIdx, 0, moved);
      return next;
    });
  }, []);

  const handleFocus = useCallback((idx) => {
    if (ctx?.openFocus) ctx.openFocus(id, idx);
  }, [ctx, id]);

  return React.createElement('div', { className: 'dc-section', 'data-section-id': id },
    React.createElement('div', { className: 'dc-section-title' },
      React.createElement(DCEditable, { value: editTitle, onChange: setEditTitle })
    ),
    subtitle && React.createElement('div', { className: 'dc-section-subtitle' }, subtitle),
    React.createElement('div', { className: 'dc-artboard-row', style: { gap } },
      ordered.map((a, i) => React.createElement(DCArtboardFrame, {
        key: a.id, id: a.id, label: a.label || a.id,
        width: a.width || 1440, height: a.height || 900,
        children: a.children, index: i, total: ordered.length,
        onFocus: handleFocus, onReorder: handleReorder,
      }))
    )
  );
}

/* ═══════════════════════════════════════════
   DCFocusOverlay — full-screen artboard viewer
   ═══════════════════════════════════════════ */
function DCFocusOverlay({ sections, sectionId, artboardIdx, onClose }) {
  const [curSection, setCurSection] = useState(sectionId);
  const [curIdx, setCurIdx] = useState(artboardIdx);

  // Current section's artboards
  const sec = sections.find(s => s.id === curSection);
  const boards = sec?.artboards || [];
  const board = boards[curIdx];

  // Section list for dropdown
  const sectionList = sections.map(s => ({ id: s.id, title: s.title }));

  // Clamp idx when switching sections
  useEffect(() => {
    const s = sections.find(s => s.id === curSection);
    if (s && curIdx >= s.artboards.length) setCurIdx(0);
  }, [curSection, sections, curIdx]);

  // Keyboard nav
  useEffect(() => {
    const handler = (e) => {
      if (e.key === 'Escape') { onClose(); return; }
      if (e.key === 'ArrowLeft') setCurIdx(i => Math.max(0, i - 1));
      if (e.key === 'ArrowRight') setCurIdx(i => Math.min((boards.length || 1) - 1, i + 1));
      if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
        e.preventDefault();
        const sIdx = sections.findIndex(s => s.id === curSection);
        const next = e.key === 'ArrowDown'
          ? Math.min(sections.length - 1, sIdx + 1)
          : Math.max(0, sIdx - 1);
        if (next !== sIdx) { setCurSection(sections[next].id); setCurIdx(0); }
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [boards, curSection, sections, onClose]);

  if (!board) return null;

  // Scale to fit viewport w/ padding
  const padX = 200, padTop = 64, padBot = 56;
  const vw = window.innerWidth - padX * 2;
  const vh = window.innerHeight - padTop - padBot;
  const bw = board.width || 1440, bh = board.height || 900;
  const scale = Math.min(1, vw / bw, vh / bh);

  return createPortal(
    React.createElement('div', {
      className: 'dc-focus-overlay',
      onClick: (e) => { if (e.target === e.currentTarget) onClose(); },
      role: 'dialog',
      'aria-modal': 'true',
      'aria-label': `Viewing ${board.label || board.id}`,
    },
      // Section dropdown
      React.createElement('div', { className: 'dc-focus-dropdown' },
        React.createElement('select', {
          value: curSection,
          onChange: (e) => { setCurSection(e.target.value); setCurIdx(0); },
          'aria-label': 'Select section',
        }, sectionList.map(s => React.createElement('option', { key: s.id, value: s.id }, s.title)))
      ),
      // Close button
      React.createElement('button', {
        className: 'dc-focus-close',
        onClick: onClose,
        'aria-label': 'Close overlay',
      }, '✕'),
      // Left/right arrows
      curIdx > 0 && React.createElement('button', {
        className: 'dc-focus-nav', 'data-dir': 'left',
        onClick: () => setCurIdx(i => i - 1),
        'aria-label': 'Previous artboard',
      }, '‹'),
      curIdx < boards.length - 1 && React.createElement('button', {
        className: 'dc-focus-nav', 'data-dir': 'right',
        onClick: () => setCurIdx(i => i + 1),
        'aria-label': 'Next artboard',
      }, '›'),
      // Card
      React.createElement('div', {
        className: 'dc-focus-card',
        style: { width: bw * scale, height: bh * scale, transform: `scale(1)` },
      },
        React.createElement('div', {
          style: { width: bw, height: bh, transform: `scale(${scale})`, transformOrigin: '0 0' },
        }, board.children)
      ),
      // Info label
      React.createElement('div', { className: 'dc-focus-info' },
        `${board.label || board.id}  ·  ${curIdx + 1} / ${boards.length}`
      ),
      // Dots navigation
      React.createElement('div', { className: 'dc-focus-dots', role: 'tablist', 'aria-label': 'Artboard navigation' },
        boards.map((_, i) => React.createElement('button', {
          key: i,
          className: `dc-focus-dot${i === curIdx ? ' active' : ''}`,
          onClick: () => setCurIdx(i),
          role: 'tab',
          'aria-selected': i === curIdx,
          'aria-label': `Artboard ${i + 1}`,
        }))
      )
    ),
    document.body
  );
}

/* ═══════════════════════════════════════════
   DCPostIt — sticky note annotation
   ═══════════════════════════════════════════ */
function DCPostIt({ top, left, right, bottom, rotate = -1, width = 200, children }) {
  const posStyle = {};
  if (top != null) posStyle.top = top;
  if (left != null) posStyle.left = left;
  if (right != null) posStyle.right = right;
  if (bottom != null) posStyle.bottom = bottom;
  if (width) posStyle.width = width;
  posStyle.transform = `rotate(${rotate}deg)`;

  return React.createElement('div', {
    className: 'dc-postit',
    style: posStyle,
  }, children);
}

/* ═══════════════════════════════════════════
   DesignCanvas — root orchestrator
   ═══════════════════════════════════════════ */
function DesignCanvas({ children }) {
  const [state, setState] = useState(() => loadState());
  const [focus, setFocus] = useState(null); // { sectionId, artboardIdx }

  // Save state on change
  useEffect(() => { saveState(state); }, [state]);

  const saveSection = useCallback((sectionId, data) => {
    setState(prev => ({ ...prev, [sectionId]: { ...prev[sectionId], ...data } }));
  }, []);

  const openFocus = useCallback((sectionId, artboardIdx) => {
    setFocus({ sectionId, artboardIdx });
  }, []);

  const closeFocus = useCallback(() => { setFocus(null); }, []);

  // Collect section info for focus overlay
  const sections = useMemo(() => {
    const result = [];
    React.Children.forEach(children, (child) => {
      if (child && child.type === DCSection) {
        const boards = [];
        React.Children.forEach(child.props.children, (ab) => {
          if (ab && ab.type === DCArtboard) boards.push({ ...ab.props });
        });
        // Apply persisted order
        const saved = state[child.props.id]?.order;
        let ordered = boards;
        if (saved && Array.isArray(saved)) {
          const map = {};
          boards.forEach(b => { map[b.id] = b; });
          ordered = saved.filter(id => map[id]).map(id => map[id]);
          const missing = boards.filter(b => !saved.includes(b.id));
          ordered = [...ordered, ...missing];
        }
        result.push({ id: child.props.id, title: child.props.title || child.props.id, artboards: ordered });
      }
    });
    return result;
  }, [children, state]);

  const ctxValue = useMemo(() => ({ state, saveSection, openFocus }), [state, saveSection, openFocus]);

  return React.createElement(DCCtx.Provider, { value: ctxValue },
    React.createElement(DCViewport, null, children),
    focus && React.createElement(DCFocusOverlay, {
      sections,
      sectionId: focus.sectionId,
      artboardIdx: focus.artboardIdx,
      onClose: closeFocus,
    })
  );
}

/* ─── Export to global scope ─── */
Object.assign(window, { DesignCanvas, DCSection, DCArtboard, DCFocusOverlay, DCViewport, DCPostIt });
