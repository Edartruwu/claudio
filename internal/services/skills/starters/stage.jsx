// stage.jsx — Animation timeline engine for scene-based animations
// =============================================================================
// Usage:
//   <Stage width={1920} height={1080} duration={10} background="#ECE9E2">
//     <Sprite start={0} end={5}><SceneOne /></Sprite>
//     <Sprite start={5} end={10}><SceneTwo /></Sprite>
//   </Stage>
//
// Inside scenes, call useSprite() to get { localTime, progress, duration, visible }
// Use interpolate() for multi-keyframe tweens, animate() for simple A→B tweens.
// =============================================================================

const {
  useState, useEffect, useRef, useMemo, useCallback,
  createContext, useContext, memo
} = React;

// ---------------------------------------------------------------------------
// Utility: clamp
// ---------------------------------------------------------------------------
const clamp = (v, min, max) => Math.min(Math.max(v, min), max);

// ---------------------------------------------------------------------------
// Easing library — all standard easing functions
// ---------------------------------------------------------------------------
const Easing = {
  linear: t => t,

  // Quadratic
  easeInQuad: t => t * t,
  easeOutQuad: t => t * (2 - t),
  easeInOutQuad: t => t < 0.5 ? 2 * t * t : -1 + (4 - 2 * t) * t,

  // Cubic
  easeInCubic: t => t * t * t,
  easeOutCubic: t => (--t) * t * t + 1,
  easeInOutCubic: t => t < 0.5 ? 4 * t * t * t : (t - 1) * (2 * t - 2) * (2 * t - 2) + 1,

  // Quartic
  easeInQuart: t => t * t * t * t,
  easeOutQuart: t => 1 - (--t) * t * t * t,
  easeInOutQuart: t => t < 0.5 ? 8 * t * t * t * t : 1 - 8 * (--t) * t * t * t,

  // Exponential
  easeInExpo: t => t === 0 ? 0 : Math.pow(2, 10 * (t - 1)),
  easeOutExpo: t => t === 1 ? 1 : 1 - Math.pow(2, -10 * t),
  easeInOutExpo: t => {
    if (t === 0 || t === 1) return t;
    return t < 0.5
      ? Math.pow(2, 20 * t - 10) / 2
      : (2 - Math.pow(2, -20 * t + 10)) / 2;
  },

  // Sine
  easeInSine: t => 1 - Math.cos((t * Math.PI) / 2),
  easeOutSine: t => Math.sin((t * Math.PI) / 2),
  easeInOutSine: t => -(Math.cos(Math.PI * t) - 1) / 2,

  // Back (overshoot)
  easeInBack: t => {
    const c = 1.70158;
    return (c + 1) * t * t * t - c * t * t;
  },
  easeOutBack: t => {
    const c = 1.70158;
    return 1 + (c + 1) * Math.pow(t - 1, 3) + c * Math.pow(t - 1, 2);
  },
  easeInOutBack: t => {
    const c = 1.70158 * 1.525;
    return t < 0.5
      ? (Math.pow(2 * t, 2) * ((c + 1) * 2 * t - c)) / 2
      : (Math.pow(2 * t - 2, 2) * ((c + 1) * (t * 2 - 2) + c) + 2) / 2;
  },

  // Elastic
  easeOutElastic: t => {
    if (t === 0 || t === 1) return t;
    return Math.pow(2, -10 * t) * Math.sin((t * 10 - 0.75) * ((2 * Math.PI) / 3)) + 1;
  },
};

// ---------------------------------------------------------------------------
// interpolate — multi-keyframe tween function
// Maps time t across input keyframes to output values.
//   input:  [t0, t1, t2, ...] — ascending time breakpoints
//   output: [v0, v1, v2, ...] — corresponding values
//   ease:   single fn OR array of fns (one per segment)
// Returns: (t) => interpolated value
// ---------------------------------------------------------------------------
function interpolate(input, output, ease) {
  const n = input.length;
  return (t) => {
    // Clamp outside range
    if (t <= input[0]) return output[0];
    if (t >= input[n - 1]) return output[n - 1];

    // Find segment
    let i = 0;
    while (i < n - 1 && t > input[i + 1]) i++;

    // Segment progress [0..1]
    const segLen = input[i + 1] - input[i];
    let p = segLen === 0 ? 1 : (t - input[i]) / segLen;

    // Apply easing
    if (ease) {
      const fn = Array.isArray(ease) ? (ease[i] || Easing.linear) : ease;
      p = fn(p);
    }

    // Linear interpolation between output values
    return output[i] + (output[i + 1] - output[i]) * p;
  };
}

// ---------------------------------------------------------------------------
// animate — simple single-segment tween
//   { from, to, start, end, ease? }
// Returns: (t) => value (clamped outside [start, end])
// ---------------------------------------------------------------------------
function animate({ from, to, start, end, ease = Easing.linear }) {
  return interpolate([start, end], [from, to], ease);
}

// ---------------------------------------------------------------------------
// Timeline context — global playback state
// ---------------------------------------------------------------------------
const TimelineContext = createContext({
  time: 0,
  duration: 10,
  playing: false,
  setTime: () => {},
  setPlaying: () => {},
});

/** Get current timeline time */
const useTime = () => useContext(TimelineContext).time;

/** Get full timeline context */
const useTimeline = () => useContext(TimelineContext);

// ---------------------------------------------------------------------------
// Sprite context — per-sprite local timing
// ---------------------------------------------------------------------------
const SpriteContext = createContext({
  localTime: 0,
  progress: 0,
  duration: 0,
  visible: false,
});

/** Get sprite-local timing info */
const useSprite = () => useContext(SpriteContext);

// ---------------------------------------------------------------------------
// Sprite — time-windowed wrapper
// Renders children only when timeline time is within [start, end].
// Children can be JSX or render function: ({ localTime, progress, duration }) => JSX
// ---------------------------------------------------------------------------
function Sprite({ start = 0, end = 1, keepMounted = false, children }) {
  const { time } = useTimeline();

  const visible = time >= start && time < end;
  const duration = end - start;
  const localTime = clamp(time - start, 0, duration);
  const progress = duration > 0 ? localTime / duration : 0;

  const ctx = useMemo(() => ({
    localTime, progress, duration, visible,
  }), [localTime, progress, duration, visible]);

  if (!visible && !keepMounted) return null;

  const content = typeof children === 'function'
    ? children({ localTime, progress, duration, visible })
    : children;

  return (
    React.createElement(SpriteContext.Provider, { value: ctx },
      React.createElement('div', {
        style: {
          position: 'absolute',
          inset: 0,
          opacity: visible ? 1 : 0,
          pointerEvents: visible ? 'auto' : 'none',
        },
      }, content)
    )
  );
}

// ---------------------------------------------------------------------------
// TextSprite — absolute-positioned text with fade+slide entry/exit
// ---------------------------------------------------------------------------
function TextSprite({
  text = '',
  x = 0,
  y = 0,
  size = 48,
  color = '#1a1a1a',
  font = 'system-ui, sans-serif',
  weight = 600,
  entryDur = 0.4,
  exitDur = 0.3,
  align = 'left',
  lineHeight = 1.3,
  children,
}) {
  const { localTime, duration, visible } = useSprite();

  // Entry: fade + slide up over entryDur
  // Exit: fade + slide up over exitDur at end
  const exitStart = duration - exitDur;
  let opacity = 1;
  let translateY = 0;

  if (localTime < entryDur) {
    // Entry phase
    const p = Easing.easeOutCubic(localTime / entryDur);
    opacity = p;
    translateY = (1 - p) * 20;
  } else if (localTime > exitStart && exitDur > 0) {
    // Exit phase
    const p = Easing.easeInCubic((localTime - exitStart) / exitDur);
    opacity = 1 - p;
    translateY = -p * 15;
  }

  if (!visible) return null;

  return React.createElement('div', {
    style: {
      position: 'absolute',
      left: x,
      top: y,
      fontSize: size,
      color,
      fontFamily: font,
      fontWeight: weight,
      textAlign: align,
      lineHeight,
      opacity,
      transform: `translateY(${translateY}px)`,
      whiteSpace: 'pre-wrap',
      willChange: 'transform, opacity',
    },
  }, children || text);
}

// ---------------------------------------------------------------------------
// ImageSprite — absolute-positioned image with optional fade entry/exit
// ---------------------------------------------------------------------------
function ImageSprite({
  src = '',
  x = 0,
  y = 0,
  width = 'auto',
  height = 'auto',
  alt = '',
  entryDur = 0.3,
  exitDur = 0.2,
  objectFit = 'contain',
  borderRadius = 0,
}) {
  const { localTime, duration, visible } = useSprite();

  const exitStart = duration - exitDur;
  let opacity = 1;

  if (localTime < entryDur) {
    opacity = Easing.easeOutCubic(localTime / entryDur);
  } else if (localTime > exitStart && exitDur > 0) {
    opacity = 1 - Easing.easeInCubic((localTime - exitStart) / exitDur);
  }

  if (!visible) return null;

  return React.createElement('img', {
    src,
    alt,
    style: {
      position: 'absolute',
      left: x,
      top: y,
      width,
      height,
      objectFit,
      borderRadius,
      opacity,
      willChange: 'opacity',
    },
  });
}

// ---------------------------------------------------------------------------
// RectSprite — absolute-positioned rectangle with optional fade
// ---------------------------------------------------------------------------
function RectSprite({
  x = 0,
  y = 0,
  width = 100,
  height = 100,
  fill = '#333',
  borderRadius = 0,
  entryDur = 0.2,
  exitDur = 0.15,
  border = 'none',
  children,
}) {
  const { localTime, duration, visible } = useSprite();

  const exitStart = duration - exitDur;
  let opacity = 1;

  if (localTime < entryDur) {
    opacity = Easing.easeOutCubic(localTime / entryDur);
  } else if (localTime > exitStart && exitDur > 0) {
    opacity = 1 - Easing.easeInCubic((localTime - exitStart) / exitDur);
  }

  if (!visible) return null;

  return React.createElement('div', {
    style: {
      position: 'absolute',
      left: x,
      top: y,
      width,
      height,
      backgroundColor: fill,
      borderRadius,
      border,
      opacity,
      willChange: 'opacity',
    },
  }, children);
}

// ---------------------------------------------------------------------------
// PlaybackBar — scrub bar with play/pause, reset, time display
// Dark theme, compact. Can be used standalone with TimelineContext.
// ---------------------------------------------------------------------------
const PlaybackBar = memo(function PlaybackBar({ height: barHeight = 44 }) {
  const { time, duration, playing, setTime, setPlaying } = useTimeline();
  const trackRef = useRef(null);
  const draggingRef = useRef(false);
  const [hoverX, setHoverX] = useState(null);

  // Format time as M:SS.cs (centiseconds)
  const fmt = useCallback((t) => {
    const mins = Math.floor(t / 60);
    const secs = Math.floor(t % 60);
    const cs = Math.floor((t % 1) * 100);
    return `${mins}:${String(secs).padStart(2, '0')}.${String(cs).padStart(2, '0')}`;
  }, []);

  // Seek from pointer event
  const seekFromEvent = useCallback((e) => {
    const track = trackRef.current;
    if (!track) return;
    const rect = track.getBoundingClientRect();
    const x = clamp(e.clientX - rect.left, 0, rect.width);
    const t = (x / rect.width) * duration;
    setTime(t);
  }, [duration, setTime]);

  // Pointer handlers for scrub track
  const onPointerDown = useCallback((e) => {
    draggingRef.current = true;
    e.currentTarget.setPointerCapture(e.pointerId);
    seekFromEvent(e);
  }, [seekFromEvent]);

  const onPointerMove = useCallback((e) => {
    const track = trackRef.current;
    if (!track) return;
    const rect = track.getBoundingClientRect();
    setHoverX(clamp(e.clientX - rect.left, 0, rect.width));
    if (draggingRef.current) seekFromEvent(e);
  }, [seekFromEvent]);

  const onPointerUp = useCallback(() => {
    draggingRef.current = false;
  }, []);

  const progress = duration > 0 ? (time / duration) * 100 : 0;

  // Icon: play triangle or pause bars
  const playIcon = playing
    ? React.createElement('svg', { width: 16, height: 16, viewBox: '0 0 16 16', fill: 'currentColor' },
        React.createElement('rect', { x: 2, y: 2, width: 4, height: 12, rx: 1 }),
        React.createElement('rect', { x: 10, y: 2, width: 4, height: 12, rx: 1 }),
      )
    : React.createElement('svg', { width: 16, height: 16, viewBox: '0 0 16 16', fill: 'currentColor' },
        React.createElement('path', { d: 'M4 2l10 6-10 6z' }),
      );

  // Icon: reset (skip to start)
  const resetIcon = React.createElement('svg', { width: 14, height: 14, viewBox: '0 0 16 16', fill: 'currentColor' },
    React.createElement('rect', { x: 1, y: 2, width: 3, height: 12, rx: 1 }),
    React.createElement('path', { d: 'M14 2L6 8l8 6z' }),
  );

  const btnStyle = {
    width: 28, height: 28,
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    background: 'rgba(255,255,255,0.08)',
    border: '1px solid rgba(255,255,255,0.12)',
    borderRadius: 6,
    color: '#e0e0e0',
    cursor: 'pointer',
    flexShrink: 0,
    padding: 0,
  };

  const timeStyle = {
    fontFamily: '"SF Mono", "Cascadia Code", "Fira Code", monospace',
    fontSize: 12,
    color: '#b0b0b0',
    minWidth: 62,
    textAlign: 'center',
    flexShrink: 0,
    userSelect: 'none',
  };

  return React.createElement('div', {
    style: {
      height: barHeight,
      background: 'rgba(20,20,20,0.92)',
      backdropFilter: 'blur(8px)',
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      padding: '0 12px',
      borderTop: '1px solid rgba(255,255,255,0.06)',
      flexShrink: 0,
      userSelect: 'none',
    },
    // Keyboard handling at bar level for standalone usage
    tabIndex: 0,
    role: 'toolbar',
    'aria-label': 'Playback controls',
  },
    // Play/pause button
    React.createElement('button', {
      onClick: () => setPlaying(!playing),
      style: btnStyle,
      'aria-label': playing ? 'Pause' : 'Play',
      title: playing ? 'Pause (Space)' : 'Play (Space)',
    }, playIcon),

    // Reset button
    React.createElement('button', {
      onClick: () => { setTime(0); setPlaying(false); },
      style: btnStyle,
      'aria-label': 'Reset to start',
      title: 'Reset (Home / 0)',
    }, resetIcon),

    // Current time
    React.createElement('span', { style: timeStyle }, fmt(time)),

    // Scrub track
    React.createElement('div', {
      ref: trackRef,
      onPointerDown,
      onPointerMove,
      onPointerUp,
      onPointerLeave: () => { setHoverX(null); draggingRef.current = false; },
      style: {
        flex: 1,
        height: 20,
        display: 'flex',
        alignItems: 'center',
        cursor: 'pointer',
        position: 'relative',
      },
      role: 'slider',
      'aria-label': 'Playback position',
      'aria-valuemin': 0,
      'aria-valuemax': Math.round(duration * 100) / 100,
      'aria-valuenow': Math.round(time * 100) / 100,
      'aria-valuetext': fmt(time),
    },
      // Track background
      React.createElement('div', {
        style: {
          position: 'absolute',
          left: 0, right: 0,
          height: 4,
          background: 'rgba(255,255,255,0.1)',
          borderRadius: 2,
        },
      }),
      // Progress fill
      React.createElement('div', {
        style: {
          position: 'absolute',
          left: 0,
          width: `${progress}%`,
          height: 4,
          background: '#4a9eff',
          borderRadius: 2,
          transition: draggingRef.current ? 'none' : 'width 0.05s linear',
        },
      }),
      // Hover preview indicator
      hoverX !== null && React.createElement('div', {
        style: {
          position: 'absolute',
          left: hoverX - 1,
          top: '50%',
          transform: 'translateY(-50%)',
          width: 2,
          height: 10,
          background: 'rgba(255,255,255,0.25)',
          borderRadius: 1,
          pointerEvents: 'none',
        },
      }),
      // Playhead knob
      React.createElement('div', {
        style: {
          position: 'absolute',
          left: `${progress}%`,
          top: '50%',
          transform: 'translate(-50%, -50%)',
          width: 10,
          height: 10,
          background: '#4a9eff',
          borderRadius: '50%',
          border: '2px solid #1a1a1a',
          boxShadow: '0 0 4px rgba(0,0,0,0.5)',
          pointerEvents: 'none',
        },
      }),
    ),

    // Duration
    React.createElement('span', { style: timeStyle }, fmt(duration)),
  );
});

// ---------------------------------------------------------------------------
// Stage — root animation canvas with playback engine
// Provides TimelineContext to all children.
// ---------------------------------------------------------------------------
function Stage({
  width = 1920,
  height = 1080,
  duration = 10,
  background = '#ECE9E2',
  loop = true,
  autoplay = true,
  persistKey = null,
  children,
}) {
  // ---- State ----
  // Time stored in ref for rAF loop (avoids stale closures).
  // A separate state mirror triggers re-renders at ~60fps.
  const timeRef = useRef(0);
  const [time, setTimeState] = useState(0);
  const [playing, setPlaying] = useState(autoplay);
  const containerRef = useRef(null);
  const canvasRef = useRef(null);
  const rafRef = useRef(null);
  const prevFrameRef = useRef(null);
  const [scale, setScale] = useState(1);

  // Persistent playhead — restore from localStorage
  useEffect(() => {
    if (!persistKey) return;
    try {
      const saved = localStorage.getItem(`stage_time_${persistKey}`);
      if (saved !== null) {
        const t = parseFloat(saved);
        if (!isNaN(t) && t >= 0 && t <= duration) {
          timeRef.current = t;
          setTimeState(t);
        }
      }
    } catch (e) { /* localStorage unavailable */ }
  }, [persistKey, duration]);

  // Persistent playhead — save on time change
  useEffect(() => {
    if (!persistKey) return;
    try {
      localStorage.setItem(`stage_time_${persistKey}`, String(time));
    } catch (e) { /* ignore */ }
  }, [persistKey, time]);

  // ---- setTime exposed to context (also updates ref) ----
  const setTime = useCallback((t) => {
    const clamped = clamp(t, 0, duration);
    timeRef.current = clamped;
    setTimeState(clamped);
  }, [duration]);

  // ---- rAF animation loop ----
  useEffect(() => {
    if (!playing) {
      prevFrameRef.current = null;
      return;
    }

    const tick = (now) => {
      if (prevFrameRef.current !== null) {
        const dt = (now - prevFrameRef.current) / 1000;
        let next = timeRef.current + dt;

        if (next >= duration) {
          if (loop) {
            next = next % duration;
          } else {
            next = duration;
            setPlaying(false);
          }
        }

        timeRef.current = next;
        setTimeState(next);
      }

      prevFrameRef.current = now;
      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, [playing, duration, loop]);

  // ---- Responsive scaling via ResizeObserver ----
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const updateScale = () => {
      const rect = container.getBoundingClientRect();
      // Account for PlaybackBar height (~44px)
      const availH = rect.height - 48;
      const availW = rect.width;
      if (availW <= 0 || availH <= 0) return;
      const s = Math.min(availW / width, availH / height);
      setScale(s);
    };

    const ro = new ResizeObserver(updateScale);
    ro.observe(container);
    updateScale();
    return () => ro.disconnect();
  }, [width, height]);

  // ---- Keyboard shortcuts ----
  useEffect(() => {
    const onKey = (e) => {
      // Only handle when stage or its children have focus,
      // or when nothing specific is focused (body)
      const tag = e.target.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;

      switch (e.code) {
        case 'Space':
          e.preventDefault();
          setPlaying(p => !p);
          break;
        case 'ArrowLeft': {
          e.preventDefault();
          const step = e.shiftKey ? 1 : 0.1;
          setTime(timeRef.current - step);
          break;
        }
        case 'ArrowRight': {
          e.preventDefault();
          const step = e.shiftKey ? 1 : 0.1;
          setTime(timeRef.current + step);
          break;
        }
        case 'Digit0':
        case 'Numpad0':
        case 'Home':
          e.preventDefault();
          setTime(0);
          setPlaying(false);
          break;
      }
    };

    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [setTime]);

  // ---- Context value (memoized) ----
  const ctx = useMemo(() => ({
    time, duration, playing, setTime, setPlaying,
  }), [time, duration, playing, setTime]);

  // ---- Render ----
  return React.createElement(TimelineContext.Provider, { value: ctx },
    React.createElement('div', {
      ref: containerRef,
      style: {
        position: 'fixed',
        inset: 0,
        background: '#0a0a0a',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
      },
    },
      // Canvas area (flex: 1, centered)
      React.createElement('div', {
        style: {
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          overflow: 'hidden',
          position: 'relative',
        },
      },
        // Scaled canvas
        React.createElement('div', {
          ref: canvasRef,
          style: {
            width,
            height,
            background,
            position: 'relative',
            overflow: 'hidden',
            transform: `scale(${scale})`,
            transformOrigin: 'center center',
            boxShadow: '0 4px 60px rgba(0,0,0,0.5)',
            flexShrink: 0,
          },
        }, children),
      ),

      // PlaybackBar (fixed at bottom)
      React.createElement(PlaybackBar, null),
    ),
  );
}

// ---------------------------------------------------------------------------
// Export everything to global scope
// ---------------------------------------------------------------------------
Object.assign(window, {
  Stage,
  Sprite,
  SpriteContext,
  useSprite,
  TimelineContext,
  useTime,
  useTimeline,
  TextSprite,
  ImageSprite,
  RectSprite,
  PlaybackBar,
  Easing,
  interpolate,
  animate,
  clamp,
});
