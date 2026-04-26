/**
 * motion.jsx — Animation primitives for Claudio design skills.
 *
 * Exports: Particles, Confetti, useSpring, SpringValue,
 *          PathAnimation, CountUp, TypeWriter, Morph
 *
 * React is global. No imports. Pure JS + Canvas API + SVG.
 */

/* ------------------------------------------------------------------ */
/*  useSpring — spring-physics hook via rAF                           */
/* ------------------------------------------------------------------ */

function useSpring(target, options = {}) {
  const { stiffness = 170, damping = 26, mass = 1 } = options;
  const ref = React.useRef({
    position: target,
    velocity: 0,
    target,
    raf: null,
  });
  const [value, setValue] = React.useState(target);

  React.useEffect(() => {
    const s = ref.current;
    s.target = target;

    const step = () => {
      const dt = 1 / 60;
      const displacement = s.target - s.position;
      const springForce = stiffness * displacement;
      const dampingForce = damping * s.velocity;
      const acceleration = (springForce - dampingForce) / mass;

      s.velocity += acceleration * dt;
      s.position += s.velocity * dt;

      if (
        Math.abs(s.velocity) < 0.001 &&
        Math.abs(s.target - s.position) < 0.001
      ) {
        s.position = s.target;
        s.velocity = 0;
        setValue(s.position);
        return;
      }

      setValue(s.position);
      s.raf = requestAnimationFrame(step);
    };

    cancelAnimationFrame(s.raf);
    s.raf = requestAnimationFrame(step);

    return () => cancelAnimationFrame(s.raf);
  }, [target, stiffness, damping, mass]);

  return value;
}

/* ------------------------------------------------------------------ */
/*  SpringValue — render-prop wrapper around useSpring                 */
/* ------------------------------------------------------------------ */

function SpringValue({ target, stiffness, damping, mass, children }) {
  const v = useSpring(target, { stiffness, damping, mass });
  return children(v);
}

/* ------------------------------------------------------------------ */
/*  Particles — canvas-based particle system                          */
/* ------------------------------------------------------------------ */

function Particles(props) {
  const {
    width = 400,
    height = 400,
    count = 80,
    speed = 1,
    color = '#000',
    shape = 'dot',
    gravity = 0,
    repelMouse = false,
    connected = false,
    connectDistance = 80,
    maxRadius = 3,
    minRadius = 1,
    opacity = 0.6,
    running = true,
  } = props;

  const canvasRef = React.useRef(null);
  const particlesRef = React.useRef([]);
  const mouseRef = React.useRef({ x: -9999, y: -9999 });
  const rafRef = React.useRef(null);

  // Init particles
  React.useEffect(() => {
    const colors = Array.isArray(color) ? color : [color];
    particlesRef.current = Array.from({ length: count }, () => ({
      x: Math.random() * width,
      y: Math.random() * height,
      vx: (Math.random() - 0.5) * 2 * speed,
      vy: (Math.random() - 0.5) * 2 * speed,
      r: minRadius + Math.random() * (maxRadius - minRadius),
      color: colors[Math.floor(Math.random() * colors.length)],
      opacity: opacity * (0.5 + Math.random() * 0.5),
    }));
  }, [count, width, height, speed, color, maxRadius, minRadius, opacity]);

  // Mouse tracking
  React.useEffect(() => {
    if (!repelMouse) return;
    const canvas = canvasRef.current;
    if (!canvas) return;
    const onMove = (e) => {
      const rect = canvas.getBoundingClientRect();
      mouseRef.current = { x: e.clientX - rect.left, y: e.clientY - rect.top };
    };
    const onLeave = () => {
      mouseRef.current = { x: -9999, y: -9999 };
    };
    canvas.addEventListener('mousemove', onMove);
    canvas.addEventListener('mouseleave', onLeave);
    return () => {
      canvas.removeEventListener('mousemove', onMove);
      canvas.removeEventListener('mouseleave', onLeave);
    };
  }, [repelMouse]);

  // Animation loop
  React.useEffect(() => {
    if (!running) {
      cancelAnimationFrame(rafRef.current);
      return;
    }
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');

    const drawStar = (cx, cy, r) => {
      const spikes = 5;
      const outerR = r;
      const innerR = r * 0.4;
      ctx.beginPath();
      for (let i = 0; i < spikes * 2; i++) {
        const rad = (Math.PI / 2) * -1 + (Math.PI / spikes) * i;
        const rr = i % 2 === 0 ? outerR : innerR;
        const method = i === 0 ? 'moveTo' : 'lineTo';
        ctx[method](cx + Math.cos(rad) * rr, cy + Math.sin(rad) * rr);
      }
      ctx.closePath();
      ctx.fill();
    };

    const loop = () => {
      ctx.clearRect(0, 0, width, height);
      const pts = particlesRef.current;

      for (let i = 0; i < pts.length; i++) {
        const p = pts[i];

        // Gravity
        p.vy += gravity * 0.01;

        // Mouse repel
        if (repelMouse) {
          const dx = p.x - mouseRef.current.x;
          const dy = p.y - mouseRef.current.y;
          const dist = Math.sqrt(dx * dx + dy * dy);
          if (dist < 80 && dist > 0) {
            const force = (80 - dist) / 80;
            p.vx += (dx / dist) * force * 0.5;
            p.vy += (dy / dist) * force * 0.5;
          }
        }

        p.x += p.vx;
        p.y += p.vy;

        // Bounce
        if (p.x < 0 || p.x > width) p.vx *= -1;
        if (p.y < 0 || p.y > height) p.vy *= -1;
        p.x = Math.max(0, Math.min(width, p.x));
        p.y = Math.max(0, Math.min(height, p.y));

        // Draw
        ctx.globalAlpha = p.opacity;
        ctx.fillStyle = p.color;

        if (shape === 'line') {
          ctx.beginPath();
          ctx.moveTo(p.x, p.y);
          ctx.lineTo(p.x + p.vx * 5, p.y + p.vy * 5);
          ctx.strokeStyle = p.color;
          ctx.lineWidth = p.r * 0.6;
          ctx.stroke();
        } else if (shape === 'star') {
          drawStar(p.x, p.y, p.r);
        } else {
          ctx.beginPath();
          ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
          ctx.fill();
        }
      }

      // Connected lines
      if (connected) {
        ctx.globalAlpha = 0.15;
        ctx.strokeStyle = Array.isArray(color) ? color[0] : color;
        ctx.lineWidth = 0.5;
        for (let i = 0; i < pts.length; i++) {
          for (let j = i + 1; j < pts.length; j++) {
            const dx = pts[i].x - pts[j].x;
            const dy = pts[i].y - pts[j].y;
            if (dx * dx + dy * dy < connectDistance * connectDistance) {
              ctx.beginPath();
              ctx.moveTo(pts[i].x, pts[i].y);
              ctx.lineTo(pts[j].x, pts[j].y);
              ctx.stroke();
            }
          }
        }
      }

      ctx.globalAlpha = 1;
      rafRef.current = requestAnimationFrame(loop);
    };

    rafRef.current = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(rafRef.current);
  }, [running, width, height, gravity, repelMouse, connected, connectDistance, color, shape]);

  return React.createElement('canvas', {
    ref: canvasRef,
    width,
    height,
    style: { display: 'block' },
  });
}

/* ------------------------------------------------------------------ */
/*  Confetti — celebration burst                                      */
/* ------------------------------------------------------------------ */

function Confetti(props) {
  const {
    active = false,
    count = 150,
    gravity = 0.3,
    spread = 120,
    origin = { x: 0.5, y: 0.3 },
    colors = ['#ff577f', '#ff884b', '#ffd384', '#fff9b0', '#3dc2ec', '#7b68ee'],
    onDone,
  } = props;

  const canvasRef = React.useRef(null);
  const rafRef = React.useRef(null);
  const particlesRef = React.useRef([]);

  React.useEffect(() => {
    if (!active) {
      cancelAnimationFrame(rafRef.current);
      const canvas = canvasRef.current;
      if (canvas) {
        const ctx = canvas.getContext('2d');
        ctx.clearRect(0, 0, canvas.width, canvas.height);
      }
      particlesRef.current = [];
      return;
    }

    const canvas = canvasRef.current;
    if (!canvas) return;
    const parent = canvas.parentElement;
    if (!parent) return;
    canvas.width = parent.offsetWidth;
    canvas.height = parent.offsetHeight;
    const ctx = canvas.getContext('2d');

    const ox = canvas.width * origin.x;
    const oy = canvas.height * origin.y;
    const spreadRad = (spread / 360) * Math.PI;

    particlesRef.current = Array.from({ length: count }, () => {
      const angle = -Math.PI / 2 + (Math.random() - 0.5) * spreadRad * 2;
      const vel = 4 + Math.random() * 6;
      return {
        x: ox,
        y: oy,
        vx: Math.cos(angle) * vel,
        vy: Math.sin(angle) * vel,
        w: 4 + Math.random() * 6,
        h: 3 + Math.random() * 4,
        color: colors[Math.floor(Math.random() * colors.length)],
        rotation: Math.random() * Math.PI * 2,
        rotSpeed: (Math.random() - 0.5) * 0.2,
        isCircle: Math.random() > 0.6,
        opacity: 1,
      };
    });

    const loop = () => {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      let alive = 0;
      const pts = particlesRef.current;

      for (let i = 0; i < pts.length; i++) {
        const p = pts[i];
        p.vy += gravity * 0.15;
        p.vx *= 0.99;
        p.x += p.vx;
        p.y += p.vy;
        p.rotation += p.rotSpeed;

        // Fade as leaving canvas
        if (p.y > canvas.height * 0.7) {
          p.opacity -= 0.02;
        }
        if (p.opacity <= 0 || p.y > canvas.height + 20) continue;
        alive++;

        ctx.save();
        ctx.globalAlpha = Math.max(0, p.opacity);
        ctx.translate(p.x, p.y);
        ctx.rotate(p.rotation);
        ctx.fillStyle = p.color;

        if (p.isCircle) {
          ctx.beginPath();
          ctx.arc(0, 0, p.w / 2, 0, Math.PI * 2);
          ctx.fill();
        } else {
          ctx.fillRect(-p.w / 2, -p.h / 2, p.w, p.h);
        }
        ctx.restore();
      }

      if (alive > 0) {
        rafRef.current = requestAnimationFrame(loop);
      } else {
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        if (onDone) onDone();
      }
    };

    rafRef.current = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(rafRef.current);
  }, [active, count, gravity, spread, origin, colors, onDone]);

  return React.createElement('canvas', {
    ref: canvasRef,
    style: {
      position: 'absolute',
      top: 0,
      left: 0,
      width: '100%',
      height: '100%',
      pointerEvents: 'none',
    },
  });
}

/* ------------------------------------------------------------------ */
/*  PathAnimation — animate element along SVG path                    */
/* ------------------------------------------------------------------ */

const EASING_FNS = {
  linear: (t) => t,
  easeInOut: (t) => (t < 0.5 ? 2 * t * t : -1 + (4 - 2 * t) * t),
  easeOut: (t) => t * (2 - t),
  easeIn: (t) => t * t,
  easeOutExpo: (t) => (t === 1 ? 1 : 1 - Math.pow(2, -10 * t)),
};

function PathAnimation(props) {
  const {
    path,
    duration = 2,
    loop = false,
    easing = 'easeInOut',
    children,
  } = props;

  const [pos, setPos] = React.useState({ x: 0, y: 0, angle: 0 });
  const rafRef = React.useRef(null);
  const startRef = React.useRef(null);
  const svgPathRef = React.useRef(null);

  React.useEffect(() => {
    // Create offscreen SVG path element to use getPointAtLength
    if (!svgPathRef.current) {
      const ns = 'http://www.w3.org/2000/svg';
      const svg = document.createElementNS(ns, 'svg');
      svgPathRef.current = document.createElementNS(ns, 'path');
      svg.appendChild(svgPathRef.current);
    }
    svgPathRef.current.setAttribute('d', path);
    const totalLength = svgPathRef.current.getTotalLength();
    const easeFn = EASING_FNS[easing] || EASING_FNS.linear;
    const durationMs = duration * 1000;

    const animate = (timestamp) => {
      if (!startRef.current) startRef.current = timestamp;
      let elapsed = timestamp - startRef.current;

      if (loop) {
        elapsed = elapsed % durationMs;
      } else if (elapsed > durationMs) {
        elapsed = durationMs;
      }

      const t = easeFn(Math.min(elapsed / durationMs, 1));
      const len = t * totalLength;
      const pt = svgPathRef.current.getPointAtLength(len);

      // Tangent angle via small delta
      const delta = 0.5;
      const pt2 = svgPathRef.current.getPointAtLength(
        Math.min(len + delta, totalLength)
      );
      const angle = (Math.atan2(pt2.y - pt.y, pt2.x - pt.x) * 180) / Math.PI;

      setPos({ x: pt.x, y: pt.y, angle });

      if (loop || elapsed < durationMs) {
        rafRef.current = requestAnimationFrame(animate);
      }
    };

    startRef.current = null;
    rafRef.current = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(rafRef.current);
  }, [path, duration, loop, easing]);

  return children(pos);
}

/* ------------------------------------------------------------------ */
/*  CountUp — animated number counter                                 */
/* ------------------------------------------------------------------ */

function CountUp(props) {
  const {
    from = 0,
    to,
    duration = 2,
    easing = 'easeOutExpo',
    separator = '',
    decimals = 0,
    children,
  } = props;

  const [display, setDisplay] = React.useState(formatNumber(from, decimals, separator));
  const rafRef = React.useRef(null);
  const startRef = React.useRef(null);

  React.useEffect(() => {
    const easeFn = EASING_FNS[easing] || EASING_FNS.easeOutExpo;
    const durationMs = duration * 1000;

    const animate = (timestamp) => {
      if (!startRef.current) startRef.current = timestamp;
      const elapsed = timestamp - startRef.current;
      const t = Math.min(elapsed / durationMs, 1);
      const easedT = easeFn(t);
      const current = from + (to - from) * easedT;
      setDisplay(formatNumber(current, decimals, separator));

      if (t < 1) {
        rafRef.current = requestAnimationFrame(animate);
      }
    };

    startRef.current = null;
    rafRef.current = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(rafRef.current);
  }, [from, to, duration, easing, separator, decimals]);

  if (children) return children(display);
  return React.createElement('span', null, display);
}

function formatNumber(n, decimals, separator) {
  const fixed = n.toFixed(decimals);
  if (!separator) return fixed;
  const [intPart, decPart] = fixed.split('.');
  const withSep = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, separator);
  return decPart !== undefined ? withSep + '.' + decPart : withSep;
}

/* ------------------------------------------------------------------ */
/*  TypeWriter — character-by-character text reveal                    */
/* ------------------------------------------------------------------ */

const CURSOR_STYLE = `
@keyframes tw-blink { 0%,100%{opacity:1} 50%{opacity:0} }
.tw-cursor { animation: tw-blink 0.7s step-end infinite; }
`;

function TypeWriter(props) {
  const {
    text,
    speed = 50,
    cursor = '|',
    onComplete,
    children,
  } = props;

  const [index, setIndex] = React.useState(0);
  const [done, setDone] = React.useState(false);
  const styleInjected = React.useRef(false);

  // Inject cursor blink keyframes once
  React.useEffect(() => {
    if (styleInjected.current) return;
    styleInjected.current = true;
    const style = document.createElement('style');
    style.textContent = CURSOR_STYLE;
    document.head.appendChild(style);
  }, []);

  React.useEffect(() => {
    setIndex(0);
    setDone(false);
  }, [text]);

  React.useEffect(() => {
    if (index >= text.length) {
      setDone(true);
      if (onComplete) onComplete();
      return;
    }
    const timer = setTimeout(() => setIndex((i) => i + 1), speed);
    return () => clearTimeout(timer);
  }, [index, text, speed, onComplete]);

  const displayed = text.slice(0, index);

  if (children) return children(displayed, done);

  return React.createElement(
    'span',
    null,
    displayed,
    !done &&
      React.createElement(
        'span',
        { className: 'tw-cursor', 'aria-hidden': 'true' },
        cursor
      )
  );
}

/* ------------------------------------------------------------------ */
/*  Morph — basic SVG path morphing via linear interpolation          */
/* ------------------------------------------------------------------ */

function Morph({ from, to, progress = 0, children }) {
  const interpolated = React.useMemo(() => {
    return interpolatePaths(from, to, progress);
  }, [from, to, progress]);

  return children(interpolated);
}

function interpolatePaths(fromD, toD, t) {
  const fromTokens = tokenizePath(fromD);
  const toTokens = tokenizePath(toD);

  // Fallback — mismatched structure → snap
  if (fromTokens.length !== toTokens.length) {
    return t < 0.5 ? fromD : toD;
  }

  return fromTokens
    .map((token, i) => {
      const toToken = toTokens[i];
      // Command letter — keep from's
      if (typeof token === 'string') return token;
      // Number — lerp
      if (typeof token === 'number' && typeof toToken === 'number') {
        return (token + (toToken - token) * t).toFixed(2);
      }
      return token;
    })
    .join(' ');
}

function tokenizePath(d) {
  const tokens = [];
  const re = /([a-zA-Z])|(-?\d*\.?\d+(?:e[+-]?\d+)?)/g;
  let m;
  while ((m = re.exec(d)) !== null) {
    if (m[1]) tokens.push(m[1]);
    else tokens.push(parseFloat(m[2]));
  }
  return tokens;
}

/* ------------------------------------------------------------------ */
/*  Exports                                                           */
/* ------------------------------------------------------------------ */

window.Particles = Particles;
window.Confetti = Confetti;
window.useSpring = useSpring;
window.SpringValue = SpringValue;
window.PathAnimation = PathAnimation;
window.CountUp = CountUp;
window.TypeWriter = TypeWriter;
window.Morph = Morph;
