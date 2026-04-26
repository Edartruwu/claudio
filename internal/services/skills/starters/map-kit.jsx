/**
 * map-kit.jsx — SVG world map with embedded country data
 *
 * Zero network requests. All geographic data baked in.
 * React available as global — no imports.
 *
 * Exports: WorldMap, RegionMap, DotMap, MapTooltip
 */

/* ---------- projection helpers ---------- */
const toX = (lon, w = 800) => ((lon + 180) / 360) * w;
const toY = (lat, h = 400) => ((90 - lat) / 180) * h;

const polyToPath = (pts) => {
  if (!pts || pts.length === 0) return '';
  return (
    'M ' +
    pts.map(([lon, lat]) => `${toX(lon).toFixed(1)},${toY(lat).toFixed(1)}`).join(' L ') +
    ' Z'
  );
};

/* ---------- embedded country data ---------- */
// Simplified polygon outlines (lon,lat pairs), equirectangular to 800x400.
// Visually recognizable, not cartographically precise.

const RAW = {
  US: { name: 'United States', pts: [[-125,48],[-123,46],[-122,42],[-120,35],[-117,33],[-115,32],[-110,31],[-108,32],[-104,33],[-103,37],[-102,40],[-100,37],[-97,36],[-95,29],[-90,29],[-89,30],[-85,30],[-82,25],[-80,25],[-80,31],[-75,35],[-73,41],[-70,42],[-67,45],[-67,47],[-75,45],[-80,43],[-83,46],[-85,47],[-90,48],[-95,49],[-100,49],[-110,49],[-120,49],[-125,48]] },
  CA: { name: 'Canada', pts: [[-140,60],[-137,59],[-131,55],[-128,50],[-123,49],[-110,49],[-100,49],[-95,49],[-90,48],[-85,47],[-80,43],[-75,45],[-67,47],[-60,47],[-55,47],[-52,50],[-56,52],[-59,55],[-64,58],[-68,60],[-75,62],[-80,63],[-85,65],[-90,68],[-95,70],[-100,72],[-110,73],[-120,72],[-130,70],[-135,68],[-140,66],[-141,62],[-140,60]] },
  MX: { name: 'Mexico', pts: [[-117,33],[-115,32],[-110,31],[-108,32],[-104,33],[-103,29],[-100,26],[-97,26],[-97,20],[-96,17],[-93,16],[-91,17],[-90,19],[-90,21],[-87,21],[-87,18],[-91,15],[-94,15],[-97,16],[-100,19],[-104,20],[-106,23],[-109,24],[-110,27],[-112,29],[-115,30],[-117,33]] },
  BR: { name: 'Brazil', pts: [[-74,-5],[-70,2],[-65,2],[-60,5],[-52,3],[-50,1],[-47,-1],[-44,-2],[-40,-3],[-38,-4],[-35,-6],[-35,-10],[-38,-13],[-39,-15],[-40,-20],[-42,-23],[-44,-23],[-48,-25],[-48,-28],[-50,-29],[-53,-33],[-55,-33],[-58,-30],[-57,-26],[-58,-22],[-60,-20],[-62,-15],[-68,-12],[-70,-10],[-74,-5]] },
  AR: { name: 'Argentina', pts: [[-65,-22],[-64,-25],[-64,-28],[-65,-30],[-65,-33],[-66,-35],[-67,-38],[-66,-42],[-65,-45],[-66,-50],[-68,-53],[-68,-55],[-65,-55],[-60,-52],[-57,-48],[-58,-40],[-59,-37],[-58,-35],[-58,-33],[-57,-30],[-56,-27],[-58,-24],[-62,-22],[-65,-22]] },
  CO: { name: 'Colombia', pts: [[-77,8],[-75,11],[-73,11],[-72,12],[-72,10],[-70,7],[-68,6],[-67,2],[-68,1],[-70,-1],[-72,-2],[-74,-1],[-77,1],[-79,2],[-78,4],[-77,7],[-77,8]] },
  PE: { name: 'Peru', pts: [[-81,-4],[-79,-2],[-76,0],[-74,-1],[-72,-2],[-70,-2],[-70,-10],[-70,-15],[-69,-17],[-71,-18],[-75,-17],[-76,-14],[-78,-10],[-80,-6],[-81,-4]] },
  CL: { name: 'Chile', pts: [[-69,-18],[-69,-22],[-70,-25],[-70,-30],[-71,-33],[-71,-38],[-72,-42],[-73,-46],[-74,-50],[-72,-53],[-69,-53],[-68,-50],[-67,-42],[-66,-35],[-65,-30],[-67,-27],[-68,-22],[-69,-18]] },
  VE: { name: 'Venezuela', pts: [[-73,11],[-72,12],[-70,12],[-67,11],[-63,11],[-61,10],[-60,8],[-62,7],[-63,4],[-65,2],[-67,2],[-68,6],[-70,7],[-72,10],[-73,11]] },
  BO: { name: 'Bolivia', pts: [[-69,-11],[-65,-10],[-62,-13],[-60,-14],[-58,-16],[-58,-20],[-60,-22],[-63,-22],[-65,-22],[-68,-20],[-69,-17],[-69,-15],[-69,-11]] },
  GB: { name: 'United Kingdom', pts: [[-5,50],[-5,52],[-3,53],[-3,55],[-4,56],[-5,58],[-3,58],[-2,57],[0,53],[1,52],[0,51],[-1,50],[-5,50]] },
  FR: { name: 'France', pts: [[-1,46],[-2,48],[-4,48],[-5,48],[0,50],[2,51],[3,50],[5,49],[7,48],[8,47],[7,44],[6,43],[3,43],[1,43],[-1,44],[-2,46],[-1,46]] },
  DE: { name: 'Germany', pts: [[6,51],[6,54],[8,55],[10,55],[12,54],[14,54],[15,52],[15,51],[14,49],[13,48],[13,47],[10,48],[8,48],[7,49],[6,51]] },
  ES: { name: 'Spain', pts: [[-9,37],[-9,39],[-8,42],[-5,43],[-2,43],[0,42],[2,42],[3,41],[3,39],[0,37],[-2,37],[-5,36],[-7,37],[-9,37]] },
  IT: { name: 'Italy', pts: [[7,44],[8,46],[10,46],[12,47],[13,46],[14,45],[16,42],[16,40],[15,38],[16,38],[17,39],[18,40],[16,40],[15,38],[13,38],[12,37],[15,37],[13,38],[11,40],[10,43],[8,44],[7,44]] },
  PL: { name: 'Poland', pts: [[14,54],[14,52],[15,51],[17,51],[19,50],[22,50],[24,52],[23,54],[21,55],[18,55],[15,54],[14,54]] },
  UA: { name: 'Ukraine', pts: [[22,52],[24,52],[27,52],[30,51],[33,52],[36,50],[38,48],[40,47],[38,46],[35,45],[33,46],[30,46],[28,48],[25,49],[23,49],[22,50],[22,52]] },
  SE: { name: 'Sweden', pts: [[11,56],[12,58],[14,59],[16,62],[18,64],[20,66],[22,66],[24,65],[20,63],[18,60],[18,59],[16,57],[14,56],[12,56],[11,56]] },
  NO: { name: 'Norway', pts: [[5,58],[6,62],[8,63],[10,64],[14,66],[16,68],[18,70],[22,71],[28,71],[30,70],[25,68],[20,66],[16,62],[12,58],[8,58],[5,58]] },
  FI: { name: 'Finland', pts: [[22,60],[23,62],[25,64],[27,66],[28,68],[29,70],[27,70],[25,68],[24,66],[23,64],[22,62],[20,60],[22,60]] },
  RU: { name: 'Russia', pts: [[28,71],[33,69],[40,68],[50,68],[60,68],[70,70],[80,72],[90,73],[100,72],[110,70],[120,68],[130,65],[140,62],[150,60],[160,62],[170,64],[180,66],[180,72],[170,72],[160,73],[140,73],[120,74],[100,74],[80,73],[60,70],[50,68],[40,65],[37,55],[38,48],[36,50],[33,52],[30,51],[27,52],[24,52],[22,52],[22,55],[25,57],[28,60],[30,65],[28,71]] },
  CN: { name: 'China', pts: [[75,40],[78,38],[80,35],[85,33],[90,28],[97,28],[100,22],[105,22],[108,20],[110,22],[115,25],[118,30],[121,31],[122,37],[124,40],[127,42],[130,43],[128,48],[120,50],[115,48],[110,45],[105,42],[100,40],[95,42],[90,45],[85,48],[80,45],[75,40]] },
  JP: { name: 'Japan', pts: [[130,31],[131,33],[132,34],[131,35],[134,36],[136,37],[137,38],[140,40],[141,42],[141,44],[140,43],[138,38],[136,35],[134,34],[132,33],[130,31]] },
  KR: { name: 'South Korea', pts: [[126,34],[127,35],[128,36],[129,38],[128,38],[127,37],[126,36],[126,34]] },
  IN: { name: 'India', pts: [[68,24],[70,28],[72,32],[73,34],[75,35],[78,35],[80,33],[85,28],[88,22],[88,20],[85,16],[82,14],[80,10],[78,8],[77,12],[76,15],[75,17],[73,20],[72,22],[70,24],[68,24]] },
  PK: { name: 'Pakistan', pts: [[62,25],[63,27],[65,30],[67,32],[68,34],[70,36],[72,35],[75,35],[73,34],[72,32],[70,28],[68,24],[66,24],[63,25],[62,25]] },
  SA: { name: 'Saudi Arabia', pts: [[36,28],[38,27],[40,26],[42,24],[44,22],[46,20],[48,18],[50,17],[52,18],[55,22],[56,24],[54,25],[50,27],[48,28],[46,29],[44,29],[42,28],[39,29],[36,28]] },
  IR: { name: 'Iran', pts: [[44,39],[46,38],[48,38],[50,37],[52,36],[54,33],[56,28],[58,25],[60,25],[62,25],[62,28],[60,30],[58,33],[56,35],[53,37],[50,38],[48,38],[46,39],[44,39]] },
  TR: { name: 'Turkey', pts: [[26,41],[28,42],[30,42],[33,42],[36,42],[38,41],[40,40],[42,40],[44,39],[44,38],[42,37],[40,37],[38,36],[36,36],[33,37],[30,37],[28,37],[26,38],[26,41]] },
  EG: { name: 'Egypt', pts: [[25,31],[28,31],[30,31],[32,31],[34,30],[35,28],[36,24],[35,22],[33,22],[31,24],[30,26],[28,28],[25,31]] },
  NG: { name: 'Nigeria', pts: [[3,6],[4,8],[5,10],[7,12],[9,13],[11,13],[13,12],[14,10],[13,8],[11,6],[9,5],[7,5],[5,5],[3,6]] },
  ZA: { name: 'South Africa', pts: [[17,-29],[19,-28],[22,-27],[26,-27],[28,-28],[30,-27],[32,-28],[33,-29],[31,-30],[30,-32],[28,-33],[26,-34],[22,-34],[19,-33],[18,-32],[17,-31],[17,-29]] },
  KE: { name: 'Kenya', pts: [[34,4],[35,2],[36,0],[37,-1],[38,-2],[40,-2],[41,-1],[41,1],[40,3],[38,4],[36,4],[34,4]] },
  ET: { name: 'Ethiopia', pts: [[33,13],[35,15],[38,15],[41,14],[43,12],[45,11],[47,8],[46,5],[44,4],[42,3],[40,4],[38,5],[36,6],[35,8],[33,10],[33,13]] },
  MA: { name: 'Morocco', pts: [[-13,36],[-10,36],[-6,36],[-5,35],[-2,35],[-1,33],[-2,30],[-5,30],[-8,29],[-10,30],[-13,32],[-13,36]] },
  DZ: { name: 'Algeria', pts: [[-2,35],[0,36],[3,37],[6,37],[8,37],[9,35],[8,33],[9,30],[8,25],[6,20],[3,19],[0,22],[-2,24],[-3,27],[-2,30],[-1,33],[-2,35]] },
  IQ: { name: 'Iraq', pts: [[39,37],[41,37],[43,37],[44,36],[45,35],[47,33],[48,30],[46,29],[44,29],[42,32],[40,33],[39,35],[39,37]] },
  AF: { name: 'Afghanistan', pts: [[61,36],[63,37],[65,38],[67,37],[69,37],[71,36],[70,34],[68,34],[66,32],[64,30],[62,30],[61,32],[61,36]] },
  KZ: { name: 'Kazakhstan', pts: [[50,52],[55,54],[60,55],[65,54],[70,52],[75,50],[80,48],[80,45],[75,42],[70,40],[65,42],[60,44],[55,44],[52,46],[50,48],[50,52]] },
  MN: { name: 'Mongolia', pts: [[88,50],[92,50],[96,50],[100,48],[105,48],[110,48],[115,48],[118,48],[115,46],[110,45],[105,42],[100,42],[95,44],[90,45],[88,48],[88,50]] },
  MM: { name: 'Myanmar', pts: [[92,28],[94,27],[96,25],[98,24],[99,22],[99,19],[98,16],[97,15],[96,16],[95,18],[94,20],[93,22],[92,24],[92,28]] },
  TH: { name: 'Thailand', pts: [[98,20],[100,19],[102,18],[103,16],[104,14],[103,12],[101,10],[100,8],[99,7],[99,10],[100,12],[100,14],[99,16],[98,18],[98,20]] },
  VN: { name: 'Vietnam', pts: [[103,22],[104,21],[106,20],[107,18],[108,16],[108,14],[107,12],[106,10],[106,8],[105,10],[105,12],[106,14],[106,16],[105,18],[104,20],[103,22]] },
  MY: { name: 'Malaysia', pts: [[100,6],[101,5],[102,4],[103,2],[104,1],[105,2],[106,3],[107,3],[105,5],[103,5],[101,6],[100,6]] },
  PH: { name: 'Philippines', pts: [[117,18],[118,17],[119,16],[120,14],[121,12],[122,10],[122,12],[121,14],[120,16],[119,18],[118,19],[117,18]] },
  ID: { name: 'Indonesia', pts: [[95,5],[98,4],[100,2],[103,0],[105,-2],[107,-4],[110,-6],[112,-7],[115,-8],[118,-8],[120,-8],[122,-6],[125,-5],[128,-4],[130,-3],[135,-4],[140,-5],[140,-3],[135,-2],[130,-1],[125,-2],[120,-5],[115,-6],[110,-5],[107,-2],[105,0],[102,1],[100,2],[97,3],[95,5]] },
  AU: { name: 'Australia', pts: [[115,-14],[118,-15],[120,-18],[122,-20],[124,-22],[127,-25],[130,-28],[132,-30],[135,-33],[137,-35],[140,-37],[143,-38],[147,-39],[150,-38],[152,-36],[153,-33],[153,-28],[150,-24],[148,-20],[146,-18],[144,-14],[142,-12],[140,-12],[138,-14],[135,-15],[132,-14],[130,-12],[128,-14],[125,-13],[120,-13],[115,-14]] },
  NZ: { name: 'New Zealand', pts: [[166,-35],[168,-37],[170,-39],[172,-41],[174,-43],[175,-45],[174,-46],[172,-45],[170,-43],[168,-42],[167,-40],[166,-38],[166,-35]] },
};

// Convert raw polygon data to SVG path strings
const COUNTRIES = {};
Object.keys(RAW).forEach((code) => {
  COUNTRIES[code] = {
    name: RAW[code].name,
    d: polyToPath(RAW[code].pts),
  };
});

/* ---------- region viewBox presets ---------- */
const REGIONS = {
  europe:   { minLon: -25, maxLon: 45, minLat: 35, maxLat: 72 },
  asia:     { minLon: 25,  maxLon: 150, minLat: 0,  maxLat: 55 },
  americas: { minLon: -170, maxLon: -30, minLat: -60, maxLat: 75 },
  africa:   { minLon: -20, maxLon: 55,  minLat: -40, maxLat: 40 },
  oceania:  { minLon: 100, maxLon: 180, minLat: -50, maxLat: 20 },
};

/* ---------- MapTooltip ---------- */
function MapTooltip({ x, y, label, visible }) {
  if (!visible || !label) return null;
  return React.createElement(
    'div',
    {
      style: {
        position: 'absolute',
        left: x + 12,
        top: y - 8,
        background: '#1e293b',
        color: '#f8fafc',
        padding: '4px 10px',
        borderRadius: 9999,
        fontSize: 12,
        fontWeight: 500,
        lineHeight: '18px',
        pointerEvents: 'none',
        whiteSpace: 'nowrap',
        zIndex: 50,
        boxShadow: '0 2px 6px rgba(0,0,0,.25)',
        transition: 'opacity 120ms ease-out',
        opacity: visible ? 1 : 0,
      },
      role: 'tooltip',
      'aria-hidden': !visible,
    },
    label
  );
}

/* ---------- WorldMap ---------- */
function WorldMap(props) {
  const {
    width = 800,
    height = 400,
    fills = {},
    highlights = [],
    defaultFill = '#e2e8f0',
    strokeColor = '#fff',
    strokeWidth = 0.5,
    onCountryClick,
    onCountryHover,
    showTooltip: showTip = true,
    background = '#f0f4ff',
    viewBox: customViewBox,
    children,
  } = props;

  const [tooltip, setTooltip] = React.useState({ visible: false, x: 0, y: 0, label: '' });

  const handleMouseEnter = (code, name, e) => {
    if (onCountryHover) onCountryHover(code, name);
    if (showTip) {
      const rect = e.currentTarget.closest('svg').getBoundingClientRect();
      setTooltip({ visible: true, x: e.clientX - rect.left, y: e.clientY - rect.top, label: name });
    }
  };

  const handleMouseMove = (e) => {
    if (!tooltip.visible) return;
    const rect = e.currentTarget.getBoundingClientRect();
    setTooltip((t) => ({ ...t, x: e.clientX - rect.left, y: e.clientY - rect.top }));
  };

  const handleMouseLeave = () => {
    if (onCountryHover) onCountryHover(null);
    setTooltip({ visible: false, x: 0, y: 0, label: '' });
  };

  const vb = customViewBox || `0 0 ${width} ${height}`;
  const highlightSet = new Set(highlights);

  return React.createElement(
    'div',
    { style: { position: 'relative', display: 'inline-block', width, maxWidth: '100%' } },
    React.createElement(
      'svg',
      {
        xmlns: 'http://www.w3.org/2000/svg',
        viewBox: vb,
        width: '100%',
        height: 'auto',
        style: { background, borderRadius: 8, display: 'block' },
        onMouseMove: handleMouseMove,
        role: 'img',
        'aria-label': 'World map',
      },
      /* country paths */
      Object.entries(COUNTRIES).map(([code, { name, d }]) =>
        React.createElement('path', {
          key: code,
          d,
          fill: fills[code] || defaultFill,
          stroke: highlightSet.has(code) ? '#f59e0b' : strokeColor,
          strokeWidth: highlightSet.has(code) ? 2 : strokeWidth,
          strokeLinejoin: 'round',
          style: { cursor: onCountryClick ? 'pointer' : 'default', transition: 'fill 150ms ease' },
          onMouseEnter: (e) => handleMouseEnter(code, name, e),
          onMouseLeave: handleMouseLeave,
          onClick: onCountryClick ? () => onCountryClick(code, name) : undefined,
          'aria-label': name,
          role: onCountryClick ? 'button' : undefined,
          tabIndex: onCountryClick ? 0 : undefined,
          onKeyDown: onCountryClick
            ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onCountryClick(code, name); } }
            : undefined,
        })
      ),
      children
    ),
    showTip && React.createElement(MapTooltip, tooltip)
  );
}

/* ---------- RegionMap ---------- */
function RegionMap(props) {
  const { region = 'europe', width = 800, height = 400, ...rest } = props;
  const r = REGIONS[region] || REGIONS.europe;
  const x = toX(r.minLon);
  const y = toY(r.maxLat);
  const w = toX(r.maxLon) - x;
  const h = toY(r.minLat) - y;
  return React.createElement(WorldMap, {
    width,
    height,
    viewBox: `${x.toFixed(1)} ${y.toFixed(1)} ${w.toFixed(1)} ${h.toFixed(1)}`,
    ...rest,
  });
}

/* ---------- DotMap ---------- */
function DotMap(props) {
  const {
    dots = [],
    onDotClick,
    onDotHover,
    width = 800,
    height = 400,
    ...mapProps
  } = props;

  const [tipDot, setTipDot] = React.useState(null);
  const [tipPos, setTipPos] = React.useState({ x: 0, y: 0 });

  const handleDotEnter = (dot, e) => {
    if (onDotHover) onDotHover(dot);
    const rect = e.currentTarget.closest('svg').getBoundingClientRect();
    setTipDot(dot);
    setTipPos({ x: e.clientX - rect.left, y: e.clientY - rect.top });
  };

  const handleDotLeave = () => {
    if (onDotHover) onDotHover(null);
    setTipDot(null);
  };

  const vbWidth = mapProps.viewBox ? parseFloat(mapProps.viewBox.split(' ')[2]) : width;

  const dotEls = dots.map((dot, i) => {
    const cx = toX(dot.lon, 800);
    const cy = toY(dot.lat, 400);
    const r = dot.size || 4;
    return React.createElement('circle', {
      key: dot.id || `dot-${i}`,
      cx,
      cy,
      r,
      fill: dot.color || '#ef4444',
      stroke: '#fff',
      strokeWidth: 1,
      style: { cursor: onDotClick ? 'pointer' : 'default', transition: 'r 150ms ease' },
      onMouseEnter: (e) => handleDotEnter(dot, e),
      onMouseLeave: handleDotLeave,
      onClick: onDotClick ? () => onDotClick(dot) : undefined,
      'aria-label': dot.label || `Dot at ${dot.lat}, ${dot.lon}`,
      role: onDotClick ? 'button' : undefined,
      tabIndex: onDotClick ? 0 : undefined,
      onKeyDown: onDotClick
        ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onDotClick(dot); } }
        : undefined,
    });
  });

  return React.createElement(
    'div',
    { style: { position: 'relative', display: 'inline-block', width, maxWidth: '100%' } },
    React.createElement(WorldMap, {
      width,
      height,
      showTooltip: !tipDot,
      ...mapProps,
      children: dotEls,
    }),
    tipDot &&
      React.createElement(MapTooltip, {
        x: tipPos.x,
        y: tipPos.y,
        label: tipDot.label || `${tipDot.lat.toFixed(1)}, ${tipDot.lon.toFixed(1)}`,
        visible: true,
      })
  );
}

/* ---------- exports ---------- */
if (typeof module !== 'undefined' && module.exports) {
  module.exports = { WorldMap, RegionMap, DotMap, MapTooltip, COUNTRIES, REGIONS };
}
