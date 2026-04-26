/**
 * chart-kit.jsx — SVG charts, zero CDN
 * Loaded via <script type="text/babel" src="chart-kit.jsx">
 * React is global. No imports.
 *
 * Exports: LineChart, BarChart, PieChart, DonutChart, AreaChart,
 *          SparkLine, ChartTooltip, ChartLegend
 */

// ─── Palette ────────────────────────────────────────────────────────────────
const DEFAULT_COLORS = [
  '#2563eb','#16a34a','#dc2626','#d97706',
  '#7c3aed','#0891b2','#be185d','#65a30d',
];

const DEFAULT_PAD = { top: 20, right: 20, bottom: 40, left: 40 };

// ─── Helpers ─────────────────────────────────────────────────────────────────
function pad(p) {
  return { ...DEFAULT_PAD, ...p };
}

function clr(colors, i) {
  const c = colors || DEFAULT_COLORS;
  return c[i % c.length];
}

function niceMax(max) {
  if (max === 0) return 10;
  const exp = Math.pow(10, Math.floor(Math.log10(max)));
  return Math.ceil(max / exp) * exp;
}

function yTicks(max, count = 5) {
  const step = niceMax(max) / count;
  return Array.from({ length: count + 1 }, (_, i) => i * step);
}

function fmtNum(n) {
  if (Math.abs(n) >= 1e6) return (n / 1e6).toFixed(1) + 'M';
  if (Math.abs(n) >= 1e3) return (n / 1e3).toFixed(1) + 'k';
  return String(n % 1 === 0 ? n : n.toFixed(1));
}

// ─── ChartTooltip ─────────────────────────────────────────────────────────────
function ChartTooltip({ visible, x, y, label, value, color }) {
  if (!visible) return null;
  return (
    <div style={{
      position: 'absolute',
      left: x + 8,
      top: y - 10,
      background: 'rgba(0,0,0,0.82)',
      color: '#fff',
      borderRadius: 6,
      padding: '5px 10px',
      fontSize: 12,
      pointerEvents: 'none',
      whiteSpace: 'nowrap',
      zIndex: 999,
      lineHeight: 1.5,
    }}>
      {color && (
        <span style={{
          display: 'inline-block',
          width: 8, height: 8,
          borderRadius: '50%',
          background: color,
          marginRight: 5,
          verticalAlign: 'middle',
        }} />
      )}
      <span style={{ fontWeight: 600 }}>{label}</span>
      {value !== undefined && <span style={{ marginLeft: 6, opacity: 0.85 }}>{fmtNum(value)}</span>}
    </div>
  );
}

// ─── ChartLegend ──────────────────────────────────────────────────────────────
function ChartLegend({ items }) {
  if (!items || !items.length) return null;
  return (
    <div style={{
      display: 'flex', flexWrap: 'wrap', gap: '8px 16px',
      justifyContent: 'center', marginTop: 8,
    }}>
      {items.map((item, i) => (
        <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 12 }}>
          <span style={{
            width: 10, height: 10,
            borderRadius: '50%',
            background: item.color || clr(null, i),
            flexShrink: 0,
          }} />
          <span style={{ color: '#374151' }}>{item.label}</span>
        </div>
      ))}
    </div>
  );
}

// ─── Grid + Axes (shared for line/bar/area) ───────────────────────────────────
function GridAndAxes({ innerW, innerH, ticks, maxVal, showGrid, data, orientation = 'vertical' }) {
  return (
    <>
      {showGrid && ticks.map((t, i) => {
        const y = innerH - (t / maxVal) * innerH;
        return (
          <line key={i} x1={0} y1={y} x2={innerW} y2={y}
            stroke="#e5e7eb" strokeWidth={1} strokeDasharray="4 2" />
        );
      })}
      {/* Y axis */}
      <line x1={0} y1={0} x2={0} y2={innerH} stroke="#d1d5db" strokeWidth={1} />
      {/* X axis */}
      <line x1={0} y1={innerH} x2={innerW} y2={innerH} stroke="#d1d5db" strokeWidth={1} />
      {/* Y tick labels */}
      {ticks.map((t, i) => {
        const y = innerH - (t / maxVal) * innerH;
        return (
          <text key={i} x={-6} y={y + 4} textAnchor="end"
            fontSize={10} fill="#6b7280">{fmtNum(t)}</text>
        );
      })}
    </>
  );
}

// ─── LineChart ────────────────────────────────────────────────────────────────
function LineChart({
  data = [], width = '100%', height = 220, colors,
  animate = true, showGrid = true, showLabels = true, showValues = false,
  title, padding, area = false, smooth = false,
}) {
  const [tooltip, setTooltip] = React.useState({ visible: false });
  const [mounted, setMounted] = React.useState(false);
  const svgRef = React.useRef(null);
  const p = pad(padding);

  React.useEffect(() => {
    const t = setTimeout(() => setMounted(true), 50);
    return () => clearTimeout(t);
  }, []);

  const isNum = typeof width === 'number';
  const svgW = isNum ? width : 400;
  const innerW = svgW - p.left - p.right;
  const innerH = height - p.top - p.bottom;

  const vals = data.map(d => d.value ?? d.y ?? 0);
  const maxVal = niceMax(Math.max(...vals, 0));
  const ticks = yTicks(maxVal);
  const color = clr(colors, 0);

  const pts = data.map((d, i) => ({
    x: data.length < 2 ? innerW / 2 : (i / (data.length - 1)) * innerW,
    y: innerH - ((d.value ?? d.y ?? 0) / maxVal) * innerH,
    label: d.label ?? d.x ?? i,
    value: d.value ?? d.y ?? 0,
  }));

  function pathD(points) {
    if (!points.length) return '';
    if (!smooth) {
      return points.map((pt, i) => `${i === 0 ? 'M' : 'L'}${pt.x},${pt.y}`).join(' ');
    }
    return points.reduce((acc, pt, i) => {
      if (i === 0) return `M${pt.x},${pt.y}`;
      const prev = points[i - 1];
      const cx = (prev.x + pt.x) / 2;
      return `${acc} C${cx},${prev.y} ${cx},${pt.y} ${pt.x},${pt.y}`;
    }, '');
  }

  const linePath = pathD(pts);
  const areaPath = pts.length
    ? `${linePath} L${pts[pts.length-1].x},${innerH} L${pts[0].x},${innerH} Z`
    : '';

  function handleMouseMove(e, pt) {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect) return;
    setTooltip({ visible: true, x: pt.x + p.left, y: pt.y + p.top, label: pt.label, value: pt.value, color });
  }

  const progress = animate && mounted ? 1 : (animate ? 0 : 1);

  return (
    <div style={{ position: 'relative', width, fontFamily: 'sans-serif' }}>
      {title && <div style={{ textAlign: 'center', fontWeight: 600, fontSize: 13, marginBottom: 4, color: '#1f2937' }}>{title}</div>}
      <svg ref={svgRef} width={isNum ? width : '100%'} height={height}
        viewBox={`0 0 ${svgW} ${height}`}
        onMouseLeave={() => setTooltip({ visible: false })}>
        <defs>
          <linearGradient id="lg-area" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity={0.2} />
            <stop offset="100%" stopColor={color} stopOpacity={0} />
          </linearGradient>
          <clipPath id="lc-clip">
            <rect x={0} y={0} width={innerW * progress} height={innerH + 4} />
          </clipPath>
        </defs>
        <g transform={`translate(${p.left},${p.top})`}>
          <GridAndAxes innerW={innerW} innerH={innerH} ticks={ticks} maxVal={maxVal} showGrid={showGrid} />
          {/* area fill */}
          {area && pts.length > 1 && (
            <path d={areaPath} fill="url(#lg-area)" clipPath="url(#lc-clip)"
              style={{ transition: animate ? 'none' : undefined }} />
          )}
          {/* line */}
          {pts.length > 1 && (
            <path d={linePath} fill="none" stroke={color} strokeWidth={2.5}
              strokeLinejoin="round" strokeLinecap="round"
              clipPath="url(#lc-clip)"
              style={{ transition: animate ? 'clip-path 0.6s ease' : undefined }} />
          )}
          {/* dots + hit areas */}
          {pts.map((pt, i) => (
            <g key={i} onMouseEnter={e => handleMouseMove(e, pt)}>
              <circle cx={pt.x} cy={pt.y} r={12} fill="transparent" />
              <circle cx={pt.x} cy={pt.y} r={4} fill="#fff" stroke={color} strokeWidth={2} />
              {showLabels && (
                <text x={pt.x} y={innerH + 16} textAnchor="middle" fontSize={10} fill="#6b7280">
                  {String(pt.label).slice(0, 10)}
                </text>
              )}
              {showValues && (
                <text x={pt.x} y={pt.y - 8} textAnchor="middle" fontSize={10} fill={color} fontWeight={600}>
                  {fmtNum(pt.value)}
                </text>
              )}
            </g>
          ))}
        </g>
      </svg>
      <ChartTooltip {...tooltip} />
    </div>
  );
}

// ─── BarChart ─────────────────────────────────────────────────────────────────
function BarChart({
  data = [], width = '100%', height = 220, colors,
  animate = true, showGrid = true, showLabels = true, showValues = false,
  title, padding, orientation = 'vertical',
}) {
  const [tooltip, setTooltip] = React.useState({ visible: false });
  const [hovered, setHovered] = React.useState(null);
  const [mounted, setMounted] = React.useState(false);
  const svgRef = React.useRef(null);
  const p = pad(padding);

  React.useEffect(() => {
    const t = setTimeout(() => setMounted(true), 50);
    return () => clearTimeout(t);
  }, []);

  const isNum = typeof width === 'number';
  const svgW = isNum ? width : 400;
  const innerW = svgW - p.left - p.right;
  const innerH = height - p.top - p.bottom;
  const isH = orientation === 'horizontal';

  const vals = data.map(d => d.value ?? 0);
  const maxVal = niceMax(Math.max(...vals, 0));
  const ticks = yTicks(maxVal);

  const barCount = data.length;
  const gap = 0.25;
  const barW = barCount > 0 ? (isH ? innerH : innerW) / barCount * (1 - gap) : 0;
  const step = barCount > 0 ? (isH ? innerH : innerW) / barCount : 0;

  return (
    <div style={{ position: 'relative', width, fontFamily: 'sans-serif' }}>
      {title && <div style={{ textAlign: 'center', fontWeight: 600, fontSize: 13, marginBottom: 4, color: '#1f2937' }}>{title}</div>}
      <svg ref={svgRef} width={isNum ? width : '100%'} height={height}
        viewBox={`0 0 ${svgW} ${height}`}
        onMouseLeave={() => { setTooltip({ visible: false }); setHovered(null); }}>
        <g transform={`translate(${p.left},${p.top})`}>
          {/* grid */}
          {showGrid && ticks.map((t, i) => {
            if (isH) {
              const x = (t / maxVal) * innerW;
              return <line key={i} x1={x} y1={0} x2={x} y2={innerH} stroke="#e5e7eb" strokeWidth={1} strokeDasharray="4 2" />;
            }
            const y = innerH - (t / maxVal) * innerH;
            return <line key={i} x1={0} y1={y} x2={innerW} y2={y} stroke="#e5e7eb" strokeWidth={1} strokeDasharray="4 2" />;
          })}
          {/* axes */}
          <line x1={0} y1={0} x2={0} y2={innerH} stroke="#d1d5db" strokeWidth={1} />
          <line x1={0} y1={innerH} x2={innerW} y2={innerH} stroke="#d1d5db" strokeWidth={1} />
          {/* tick labels */}
          {ticks.map((t, i) => {
            if (isH) {
              const x = (t / maxVal) * innerW;
              return <text key={i} x={x} y={innerH + 14} textAnchor="middle" fontSize={10} fill="#6b7280">{fmtNum(t)}</text>;
            }
            const y = innerH - (t / maxVal) * innerH;
            return <text key={i} x={-6} y={y + 4} textAnchor="end" fontSize={10} fill="#6b7280">{fmtNum(t)}</text>;
          })}
          {/* bars */}
          {data.map((d, i) => {
            const color = d.color || clr(colors, i);
            const val = d.value ?? 0;
            const ratio = maxVal > 0 ? val / maxVal : 0;
            const isHov = hovered === i;

            if (isH) {
              const barH = barW;
              const y0 = i * step + (step - barH) / 2;
              const bw = animate && mounted ? ratio * innerW : (animate ? 0 : ratio * innerW);
              return (
                <g key={i} style={{ cursor: 'pointer' }}
                  onMouseEnter={e => { setHovered(i); const r = svgRef.current?.getBoundingClientRect(); setTooltip({ visible: true, x: bw + p.left + 8, y: y0 + p.top, label: d.label, value: val, color }); }}
                  onMouseLeave={() => { setHovered(null); setTooltip({ visible: false }); }}>
                  <rect x={0} y={y0} width={bw} height={barH} rx={3}
                    fill={color} opacity={isHov ? 1 : 0.85}
                    style={{ transition: animate ? 'width 0.4s ease, opacity 0.15s' : 'opacity 0.15s' }} />
                  {showLabels && <text x={-6} y={y0 + barH / 2 + 4} textAnchor="end" fontSize={10} fill="#6b7280">{String(d.label ?? i).slice(0, 12)}</text>}
                  {showValues && bw > 20 && <text x={bw - 4} y={y0 + barH / 2 + 4} textAnchor="end" fontSize={10} fill="#fff" fontWeight={600}>{fmtNum(val)}</text>}
                </g>
              );
            }

            const bh = animate && mounted ? ratio * innerH : (animate ? 0 : ratio * innerH);
            const x0 = i * step + (step - barW) / 2;
            return (
              <g key={i} style={{ cursor: 'pointer' }}
                onMouseEnter={() => { setHovered(i); setTooltip({ visible: true, x: x0 + barW / 2 + p.left, y: innerH - bh + p.top - 8, label: d.label, value: val, color }); }}
                onMouseLeave={() => { setHovered(null); setTooltip({ visible: false }); }}>
                <rect x={x0} y={innerH - bh} width={barW} height={bh} rx={3}
                  fill={color} opacity={isHov ? 1 : 0.85}
                  style={{ transition: animate ? 'height 0.4s ease, y 0.4s ease, opacity 0.15s' : 'opacity 0.15s' }} />
                {showLabels && (
                  <text x={x0 + barW / 2} y={innerH + 14} textAnchor="middle" fontSize={10} fill="#6b7280">
                    {String(d.label ?? i).slice(0, 8)}
                  </text>
                )}
                {showValues && bh > 16 && (
                  <text x={x0 + barW / 2} y={innerH - bh + 12} textAnchor="middle" fontSize={10} fill="#fff" fontWeight={600}>
                    {fmtNum(val)}
                  </text>
                )}
              </g>
            );
          })}
        </g>
      </svg>
      <ChartTooltip {...tooltip} />
    </div>
  );
}

// ─── Pie / Donut shared ───────────────────────────────────────────────────────
function arcPath(cx, cy, r, startAngle, endAngle) {
  const s = (startAngle - Math.PI / 2);
  const e = (endAngle - Math.PI / 2);
  const x1 = cx + r * Math.cos(s), y1 = cy + r * Math.sin(s);
  const x2 = cx + r * Math.cos(e), y2 = cy + r * Math.sin(e);
  const large = endAngle - startAngle > Math.PI ? 1 : 0;
  return `M${cx},${cy} L${x1},${y1} A${r},${r} 0 ${large},1 ${x2},${y2} Z`;
}

function donutArcPath(cx, cy, r, ir, startAngle, endAngle) {
  const s = startAngle - Math.PI / 2;
  const e = endAngle - Math.PI / 2;
  const x1 = cx + r * Math.cos(s), y1 = cy + r * Math.sin(s);
  const x2 = cx + r * Math.cos(e), y2 = cy + r * Math.sin(e);
  const ix1 = cx + ir * Math.cos(e), iy1 = cy + ir * Math.sin(e);
  const ix2 = cx + ir * Math.cos(s), iy2 = cy + ir * Math.sin(s);
  const large = endAngle - startAngle > Math.PI ? 1 : 0;
  return `M${x1},${y1} A${r},${r} 0 ${large},1 ${x2},${y2} L${ix1},${iy1} A${ir},${ir} 0 ${large},0 ${ix2},${iy2} Z`;
}

function PieDonutBase({ data = [], width = '100%', height = 220, colors, animate = true,
  showValues = false, title, padding, donut = false, innerRadius = 0.6, centerLabel }) {
  const [tooltip, setTooltip] = React.useState({ visible: false });
  const [hovered, setHovered] = React.useState(null);
  const [mounted, setMounted] = React.useState(false);

  React.useEffect(() => {
    const t = setTimeout(() => setMounted(true), 60);
    return () => clearTimeout(t);
  }, []);

  const isNum = typeof width === 'number';
  const svgW = isNum ? width : 300;
  const p = pad(padding);
  const cx = svgW / 2, cy = (height - p.bottom) / 2;
  const r = Math.min(cx - p.left, cy - p.top) - 4;
  const ir = donut ? r * innerRadius : 0;

  const total = data.reduce((s, d) => s + (d.value ?? 0), 0);
  let cursor = 0;
  const slices = data.map((d, i) => {
    const angle = total > 0 ? ((d.value ?? 0) / total) * 2 * Math.PI : 0;
    const start = cursor;
    cursor += angle;
    return { ...d, start, end: cursor, color: d.color || clr(colors, i) };
  });

  const legendItems = slices.map(s => ({ label: s.label, color: s.color }));

  // animate: stroke-dasharray trick for donut, scale for pie
  const animProgress = animate && mounted ? 1 : (animate ? 0 : 1);

  return (
    <div style={{ position: 'relative', width, fontFamily: 'sans-serif' }}>
      {title && <div style={{ textAlign: 'center', fontWeight: 600, fontSize: 13, marginBottom: 4, color: '#1f2937' }}>{title}</div>}
      <svg width={isNum ? width : '100%'} height={height} viewBox={`0 0 ${svgW} ${height}`}
        onMouseLeave={() => { setTooltip({ visible: false }); setHovered(null); }}>
        <g>
          {slices.map((sl, i) => {
            const isHov = hovered === i;
            const scale = isHov ? 1.04 : (animate && !mounted ? 0 : 1);
            const d = donut
              ? donutArcPath(cx, cy, r, ir, sl.start, sl.end)
              : arcPath(cx, cy, r, sl.start, sl.end);
            const midA = (sl.start + sl.end) / 2 - Math.PI / 2;
            const labelR = donut ? (r + ir) / 2 : r * 0.65;
            const lx = cx + labelR * Math.cos(midA);
            const ly = cy + labelR * Math.sin(midA);
            return (
              <g key={i} style={{ cursor: 'pointer',
                transform: `scale(${scale})`, transformOrigin: `${cx}px ${cy}px`,
                transition: animate ? 'transform 0.4s cubic-bezier(0.34,1.56,0.64,1)' : undefined }}
                onMouseEnter={e => { setHovered(i); setTooltip({ visible: true, x: cx, y: cy - r - 10, label: sl.label, value: sl.value, color: sl.color }); }}
                onMouseLeave={() => { setHovered(null); setTooltip({ visible: false }); }}>
                <path d={d} fill={sl.color} stroke="#fff" strokeWidth={2} />
                {showValues && sl.end - sl.start > 0.35 && (
                  <text x={lx} y={ly} textAnchor="middle" dominantBaseline="middle"
                    fontSize={11} fill="#fff" fontWeight={700} pointerEvents="none">
                    {Math.round((sl.value / total) * 100)}%
                  </text>
                )}
              </g>
            );
          })}
          {/* donut center label */}
          {donut && centerLabel && (
            <text x={cx} y={cy} textAnchor="middle" dominantBaseline="middle"
              fontSize={15} fontWeight={700} fill="#1f2937">{centerLabel}</text>
          )}
        </g>
      </svg>
      <ChartLegend items={legendItems} />
      <ChartTooltip {...tooltip} />
    </div>
  );
}

function PieChart(props) { return <PieDonutBase {...props} donut={false} />; }
function DonutChart(props) { return <PieDonutBase {...props} donut={true} />; }

// ─── AreaChart ────────────────────────────────────────────────────────────────
function AreaChart({
  data = [], width = '100%', height = 220, colors,
  animate = true, showGrid = true, showLabels = true, showValues = false,
  title, padding,
}) {
  // data: [{label, value}] or [{name, points:[{x,y}], color}] multi-series
  const isMulti = data.length > 0 && Array.isArray(data[0]?.points);
  const series = isMulti
    ? data
    : [{ name: '', points: data.map(d => ({ x: d.label, y: d.value ?? 0 })), color: null }];

  const [tooltip, setTooltip] = React.useState({ visible: false });
  const [mounted, setMounted] = React.useState(false);
  const svgRef = React.useRef(null);
  const p = pad(padding);

  React.useEffect(() => {
    const t = setTimeout(() => setMounted(true), 60);
    return () => clearTimeout(t);
  }, []);

  const isNum = typeof width === 'number';
  const svgW = isNum ? width : 400;
  const innerW = svgW - p.left - p.right;
  const innerH = height - p.top - p.bottom;

  const allY = series.flatMap(s => s.points.map(pt => pt.y ?? 0));
  const maxVal = niceMax(Math.max(...allY, 0));
  const ticks = yTicks(maxVal);
  const maxX = Math.max(...series.flatMap(s => s.points.length)) - 1;

  function toSVG(pt, ptCount) {
    const n = ptCount - 1 || 1;
    const idx = typeof pt.x === 'number' ? pt.x : 0;
    return {
      x: (idx / n) * innerW,
      y: innerH - ((pt.y ?? 0) / maxVal) * innerH,
    };
  }

  return (
    <div style={{ position: 'relative', width, fontFamily: 'sans-serif' }}>
      {title && <div style={{ textAlign: 'center', fontWeight: 600, fontSize: 13, marginBottom: 4, color: '#1f2937' }}>{title}</div>}
      <svg ref={svgRef} width={isNum ? width : '100%'} height={height}
        viewBox={`0 0 ${svgW} ${height}`}
        onMouseLeave={() => setTooltip({ visible: false })}>
        <defs>
          {series.map((s, si) => {
            const color = s.color || clr(colors, si);
            return (
              <linearGradient key={si} id={`area-grad-${si}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity={0.35} />
                <stop offset="100%" stopColor={color} stopOpacity={0.02} />
              </linearGradient>
            );
          })}
        </defs>
        <g transform={`translate(${p.left},${p.top})`}>
          <GridAndAxes innerW={innerW} innerH={innerH} ticks={ticks} maxVal={maxVal} showGrid={showGrid} />
          {series.map((s, si) => {
            const color = s.color || clr(colors, si);
            const pts = s.points.map((pt, pi) => ({
              ...toSVG({ x: pi, y: pt.y ?? 0 }, s.points.length),
              label: pt.x ?? pi, value: pt.y ?? 0,
            }));
            if (!pts.length) return null;
            const linePts = pts.map((pt, i) => `${i === 0 ? 'M' : 'L'}${pt.x},${pt.y}`).join(' ');
            const areaPath = `${linePts} L${pts[pts.length-1].x},${innerH} L${pts[0].x},${innerH} Z`;
            return (
              <g key={si}>
                <path d={areaPath} fill={`url(#area-grad-${si})`}
                  style={{ transition: animate && mounted ? 'opacity 0.5s' : undefined, opacity: animate && !mounted ? 0 : 1 }} />
                <path d={linePts} fill="none" stroke={color} strokeWidth={2.5}
                  strokeLinejoin="round" strokeLinecap="round"
                  style={{ transition: animate && mounted ? 'opacity 0.5s' : undefined, opacity: animate && !mounted ? 0 : 1 }} />
                {pts.map((pt, pi) => (
                  <circle key={pi} cx={pt.x} cy={pt.y} r={4} fill="#fff" stroke={color} strokeWidth={2}
                    style={{ cursor: 'pointer' }}
                    onMouseEnter={() => setTooltip({ visible: true, x: pt.x + p.left, y: pt.y + p.top - 10, label: pt.label, value: pt.value, color })} />
                ))}
              </g>
            );
          })}
          {showLabels && series[0]?.points.map((pt, pi) => {
            const x = (pi / Math.max(series[0].points.length - 1, 1)) * innerW;
            return <text key={pi} x={x} y={innerH + 16} textAnchor="middle" fontSize={10} fill="#6b7280">{String(pt.x ?? pi).slice(0, 10)}</text>;
          })}
        </g>
      </svg>
      {isMulti && <ChartLegend items={series.map((s, si) => ({ label: s.name, color: s.color || clr(colors, si) }))} />}
      <ChartTooltip {...tooltip} />
    </div>
  );
}

// ─── SparkLine ────────────────────────────────────────────────────────────────
function SparkLine({ data = [], width = 60, height = 20, color = DEFAULT_COLORS[0] }) {
  const vals = data.map(d => d.value ?? d.y ?? (typeof d === 'number' ? d : 0));
  const min = Math.min(...vals);
  const max = Math.max(...vals);
  const range = max - min || 1;
  const pts = vals.map((v, i) => {
    const x = (i / Math.max(vals.length - 1, 1)) * width;
    const y = height - ((v - min) / range) * (height - 2) - 1;
    return `${x},${y}`;
  }).join(' ');

  return (
    <svg width={width} height={height} style={{ display: 'inline-block', verticalAlign: 'middle' }}>
      {pts && <polyline points={pts} fill="none" stroke={color} strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />}
    </svg>
  );
}

// ─── Namespace export (no imports env) ───────────────────────────────────────
window.ChartKit = {
  LineChart, BarChart, PieChart, DonutChart,
  AreaChart, SparkLine, ChartTooltip, ChartLegend,
  DEFAULT_COLORS,
};
