// device-frames.jsx — iOS / Android / Desktop device frame components
// Loaded via <script type="text/babel" src="device-frames.jsx">
// React is global. No imports. All icons inline SVG.

const { useState, useRef } = React;

// ---------------------------------------------------------------------------
// SVG icon helpers (monochrome, stroke-based)
// ---------------------------------------------------------------------------

function SignalIcon({ color }) {
  return (
    <svg width="17" height="12" viewBox="0 0 17 12" fill={color} style={{ display: 'block' }}>
      <rect x="0" y="9" width="3" height="3" rx="0.5" />
      <rect x="4.5" y="6" width="3" height="6" rx="0.5" />
      <rect x="9" y="3" width="3" height="9" rx="0.5" />
      <rect x="13.5" y="0" width="3" height="12" rx="0.5" />
    </svg>
  );
}

function WifiIcon({ color }) {
  return (
    <svg width="16" height="12" viewBox="0 0 16 12" fill="none" stroke={color} strokeWidth="1.5" strokeLinecap="round" style={{ display: 'block' }}>
      <path d="M1 4.5C3.8 1.8 12.2 1.8 15 4.5" />
      <path d="M3.2 6.8C5 5.1 11 5.1 12.8 6.8" />
      <path d="M5.6 9.1C6.7 8.1 9.3 8.1 10.4 9.1" />
      <circle cx="8" cy="11.5" r="0.8" fill={color} stroke="none" />
    </svg>
  );
}

function BatteryIcon({ color }) {
  return (
    <svg width="25" height="12" viewBox="0 0 25 12" fill="none" style={{ display: 'block' }}>
      <rect x="0.5" y="0.5" width="21" height="11" rx="3.5" stroke={color} strokeWidth="1" />
      <rect x="22.5" y="3.5" width="2" height="5" rx="1" fill={color} />
      <rect x="2" y="2" width="16" height="8" rx="2" fill={color} />
    </svg>
  );
}

function ChevronIcon({ color }) {
  return (
    <svg width="8" height="13" viewBox="0 0 8 13" fill="none" stroke={color} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ display: 'block' }}>
      <path d="M6 1L1 6.5L6 12" />
    </svg>
  );
}

function EllipsisIcon({ color }) {
  return (
    <svg width="20" height="5" viewBox="0 0 20 5" fill={color} style={{ display: 'block' }}>
      <circle cx="2.5" cy="2.5" r="2.5" />
      <circle cx="10" cy="2.5" r="2.5" />
      <circle cx="17.5" cy="2.5" r="2.5" />
    </svg>
  );
}

// ---------------------------------------------------------------------------
// IOSStatusBar
// ---------------------------------------------------------------------------

function IOSStatusBar({ dark = false, time = '9:41' }) {
  const color = dark ? '#fff' : '#000';
  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '14px 24px 0',
      height: 54,
      boxSizing: 'border-box',
      position: 'relative',
      zIndex: 10,
    }}>
      <span style={{
        fontFamily: '-apple-system, "SF Pro Display", BlinkMacSystemFont, sans-serif',
        fontSize: 17,
        fontWeight: 590,
        letterSpacing: '-0.2px',
        color,
      }}>{time}</span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 7, paddingTop: 1 }}>
        <SignalIcon color={color} />
        <WifiIcon color={color} />
        <BatteryIcon color={color} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// IOSGlassPill
// ---------------------------------------------------------------------------

function IOSGlassPill({ children, dark = false, style = {} }) {
  const bg = dark
    ? 'rgba(255,255,255,0.12)'
    : 'rgba(255,255,255,0.72)';
  const border = dark
    ? '0.5px solid rgba(255,255,255,0.18)'
    : '0.5px solid rgba(0,0,0,0.1)';
  const shine = dark
    ? 'inset 0 1px 0 rgba(255,255,255,0.15)'
    : 'inset 0 1px 0 rgba(255,255,255,0.9)';

  return (
    <div style={{
      backdropFilter: 'blur(12px) saturate(180%)',
      WebkitBackdropFilter: 'blur(12px) saturate(180%)',
      background: bg,
      border,
      boxShadow: shine,
      borderRadius: 20,
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: '6px 12px',
      gap: 4,
      cursor: 'pointer',
      userSelect: 'none',
      ...style,
    }}>
      {children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// IOSNavBar
// ---------------------------------------------------------------------------

function IOSNavBar({ title = '', dark = false, trailingIcon = true }) {
  const textColor = dark ? '#fff' : '#000';
  const pillTextColor = dark ? 'rgba(255,255,255,0.9)' : '#007AFF';

  return (
    <div style={{
      padding: '8px 16px 4px',
      boxSizing: 'border-box',
    }}>
      {/* pill row */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 6 }}>
        <IOSGlassPill dark={dark} style={{ padding: '6px 14px 6px 10px' }}>
          <ChevronIcon color={pillTextColor} />
          <span style={{
            fontFamily: '-apple-system, "SF Pro Text", BlinkMacSystemFont, sans-serif',
            fontSize: 17,
            fontWeight: 400,
            color: pillTextColor,
            lineHeight: 1,
          }}>Back</span>
        </IOSGlassPill>

        {trailingIcon && (
          <IOSGlassPill dark={dark} style={{ padding: '8px 12px' }}>
            <EllipsisIcon color={pillTextColor} />
          </IOSGlassPill>
        )}
      </div>

      {/* large title */}
      {title && (
        <h1 style={{
          margin: '8px 4px 12px',
          fontFamily: '-apple-system, "SF Pro Display", BlinkMacSystemFont, sans-serif',
          fontSize: 34,
          fontWeight: 700,
          letterSpacing: '-0.5px',
          color: textColor,
          lineHeight: 1.1,
        }}>{title}</h1>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// IOSHomeIndicator
// ---------------------------------------------------------------------------

function IOSHomeIndicator({ dark = false }) {
  return (
    <div style={{
      position: 'absolute',
      bottom: 8,
      left: 0,
      right: 0,
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      height: 34,
      zIndex: 60,
    }}>
      <div style={{
        width: 134,
        height: 5,
        borderRadius: 3,
        background: dark ? 'rgba(255,255,255,0.5)' : 'rgba(0,0,0,0.2)',
      }} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// IOSKeyboard (basic QWERTY rows)
// ---------------------------------------------------------------------------

function IOSKeyboard({ dark = false }) {
  const bg = dark ? '#1c1c1e' : '#d1d5db';
  const keyBg = dark ? '#3a3a3c' : '#fff';
  const keyColor = dark ? '#fff' : '#000';
  const rows = ['QWERTYUIOP', 'ASDFGHJKL', 'ZXCVBNM'];

  return (
    <div style={{
      background: bg,
      padding: '8px 4px 4px',
      boxSizing: 'border-box',
    }}>
      {rows.map((row, ri) => (
        <div key={ri} style={{ display: 'flex', justifyContent: 'center', gap: 5, marginBottom: 8 }}>
          {row.split('').map(k => (
            <div key={k} style={{
              background: keyBg,
              color: keyColor,
              borderRadius: 5,
              height: 42,
              minWidth: 32,
              flex: 1,
              maxWidth: 36,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontFamily: '-apple-system, sans-serif',
              fontSize: 17,
              fontWeight: 400,
              boxShadow: '0 1px 0 rgba(0,0,0,0.3)',
            }}>{k}</div>
          ))}
        </div>
      ))}
      {/* space bar row */}
      <div style={{ display: 'flex', gap: 5, marginBottom: 6 }}>
        {['123', '    space    ', 'return'].map((label, i) => (
          <div key={i} style={{
            background: i === 1 ? keyBg : (dark ? '#636366' : '#adb5bd'),
            color: keyColor,
            borderRadius: 5,
            height: 42,
            flex: i === 1 ? 3 : 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontFamily: '-apple-system, sans-serif',
            fontSize: 15,
            boxShadow: '0 1px 0 rgba(0,0,0,0.3)',
          }}>{label.trim()}</div>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// IOSDevice
// ---------------------------------------------------------------------------

function IOSDevice({
  width = 390,
  height = 844,
  dark = false,
  time = '9:41',
  title = '',
  keyboard = false,
  children,
}) {
  const bg = dark ? '#000' : '#fff';
  const frame = dark ? '#1c1c1e' : '#e5e5ea';

  return (
    <div style={{
      position: 'relative',
      width,
      height,
      borderRadius: 48,
      background: frame,
      boxShadow: '0 40px 80px rgba(0,0,0,0.18), 0 0 0 1px rgba(0,0,0,0.12)',
      overflow: 'hidden',
      flexShrink: 0,
    }}>
      {/* screen */}
      <div style={{
        position: 'absolute',
        inset: 0,
        background: bg,
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
      }}>
        {/* Dynamic Island */}
        <div style={{
          position: 'absolute',
          top: 11,
          left: '50%',
          transform: 'translateX(-50%)',
          width: 126,
          height: 37,
          borderRadius: 24,
          background: '#000',
          zIndex: 50,
        }} />

        {/* Status bar */}
        <IOSStatusBar dark={dark} time={time} />

        {/* Nav bar */}
        {title && <IOSNavBar title={title} dark={dark} />}

        {/* Content area */}
        <div style={{
          flex: 1,
          overflow: 'hidden',
          position: 'relative',
        }}>
          {children}
        </div>

        {/* Keyboard */}
        {keyboard && <IOSKeyboard dark={dark} />}
      </div>

      {/* Home indicator */}
      <IOSHomeIndicator dark={dark} />
    </div>
  );
}

// ---------------------------------------------------------------------------
// AndroidStatusBar
// ---------------------------------------------------------------------------

function AndroidStatusBar({ dark = false, time = '9:41' }) {
  const color = dark ? '#fff' : '#000';
  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      padding: '10px 16px 4px',
      height: 40,
      boxSizing: 'border-box',
    }}>
      <span style={{
        fontFamily: 'Google Sans, Roboto, sans-serif',
        fontSize: 14,
        fontWeight: 500,
        color,
      }}>{time}</span>
      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
        <SignalIcon color={color} />
        <WifiIcon color={color} />
        <BatteryIcon color={color} />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// AndroidDevice
// ---------------------------------------------------------------------------

function AndroidDevice({
  width = 360,
  height = 800,
  dark = false,
  time = '9:41',
  children,
}) {
  const bg = dark ? '#121212' : '#fff';
  const frame = dark ? '#1c1c1e' : '#e0e0e0';

  return (
    <div style={{
      position: 'relative',
      width,
      height,
      borderRadius: 32,
      background: frame,
      boxShadow: '0 30px 60px rgba(0,0,0,0.15), 0 0 0 1px rgba(0,0,0,0.1)',
      overflow: 'hidden',
      flexShrink: 0,
    }}>
      {/* screen */}
      <div style={{
        position: 'absolute',
        inset: 0,
        background: bg,
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
      }}>
        {/* Punch-hole camera */}
        <div style={{
          position: 'absolute',
          top: 12,
          left: '50%',
          transform: 'translateX(-50%)',
          width: 12,
          height: 12,
          borderRadius: '50%',
          background: '#000',
          zIndex: 50,
        }} />

        <AndroidStatusBar dark={dark} time={time} />

        <div style={{ flex: 1, overflow: 'hidden', position: 'relative' }}>
          {children}
        </div>

        {/* Gesture bar */}
        <div style={{
          height: 28,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}>
          <div style={{
            width: 100,
            height: 4,
            borderRadius: 2,
            background: dark ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.15)',
          }} />
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// DesktopBrowser
// ---------------------------------------------------------------------------

function DesktopBrowser({
  width = 1280,
  height = 800,
  dark = false,
  url = 'https://example.com',
  title = 'Example',
  children,
}) {
  const chromeBg = dark ? '#2d2d2f' : '#f0f0f0';
  const urlBg = dark ? '#3a3a3c' : '#fff';
  const urlColor = dark ? 'rgba(255,255,255,0.7)' : 'rgba(0,0,0,0.5)';
  const dotColors = ['#ff5f57', '#ffbd2e', '#28c840'];
  const tabColor = dark ? '#1c1c1e' : '#fff';
  const tabText = dark ? 'rgba(255,255,255,0.85)' : 'rgba(0,0,0,0.8)';

  return (
    <div style={{
      width,
      height,
      borderRadius: 10,
      overflow: 'hidden',
      boxShadow: '0 20px 60px rgba(0,0,0,0.15), 0 0 0 1px rgba(0,0,0,0.1)',
      display: 'flex',
      flexDirection: 'column',
      flexShrink: 0,
    }}>
      {/* Tab bar */}
      <div style={{
        background: chromeBg,
        padding: '8px 12px 0',
        display: 'flex',
        alignItems: 'flex-end',
        gap: 2,
        borderBottom: dark ? '1px solid rgba(255,255,255,0.06)' : '1px solid rgba(0,0,0,0.1)',
      }}>
        {/* Traffic lights */}
        <div style={{ display: 'flex', gap: 6, alignItems: 'center', paddingBottom: 8, paddingRight: 16 }}>
          {dotColors.map((c, i) => (
            <div key={i} style={{ width: 12, height: 12, borderRadius: '50%', background: c }} />
          ))}
        </div>
        {/* Active tab */}
        <div style={{
          background: tabColor,
          borderRadius: '6px 6px 0 0',
          padding: '6px 16px',
          minWidth: 160,
          maxWidth: 220,
          display: 'flex',
          alignItems: 'center',
          gap: 8,
        }}>
          <div style={{ width: 14, height: 14, borderRadius: 3, background: '#4f46e5', flexShrink: 0 }} />
          <span style={{
            fontFamily: '-apple-system, sans-serif',
            fontSize: 12,
            color: tabText,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            flex: 1,
          }}>{title}</span>
          <span style={{ color: urlColor, fontSize: 13, cursor: 'pointer', lineHeight: 1 }}>×</span>
        </div>
      </div>

      {/* Toolbar */}
      <div style={{
        background: chromeBg,
        padding: '8px 12px',
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        borderBottom: dark ? '1px solid rgba(255,255,255,0.06)' : '1px solid rgba(0,0,0,0.1)',
      }}>
        {/* Nav arrows */}
        {['‹', '›'].map((a, i) => (
          <span key={i} style={{
            fontFamily: 'sans-serif',
            fontSize: 20,
            color: urlColor,
            cursor: 'pointer',
            lineHeight: 1,
            userSelect: 'none',
            padding: '0 2px',
          }}>{a}</span>
        ))}
        <span style={{ color: urlColor, fontSize: 16, cursor: 'pointer', userSelect: 'none' }}>↻</span>

        {/* URL bar */}
        <div style={{
          flex: 1,
          background: urlBg,
          borderRadius: 8,
          padding: '5px 12px',
          display: 'flex',
          alignItems: 'center',
          gap: 6,
          border: dark ? '1px solid rgba(255,255,255,0.1)' : '1px solid rgba(0,0,0,0.12)',
        }}>
          <span style={{ fontSize: 12, color: urlColor }}>🔒</span>
          <span style={{
            fontFamily: '-apple-system, sans-serif',
            fontSize: 13,
            color: urlColor,
            flex: 1,
          }}>{url}</span>
        </div>
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflow: 'hidden', position: 'relative', background: dark ? '#1c1c1e' : '#fff' }}>
        {children}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// MobileBrowser (iOS Safari style — URL bar at bottom)
// ---------------------------------------------------------------------------

function MobileBrowser({ width = 390, dark = false, url = 'https://example.com', children }) {
  const toolbarBg = dark ? 'rgba(28,28,30,0.94)' : 'rgba(242,242,247,0.94)';
  const urlBg = dark ? '#3a3a3c' : '#fff';
  const urlColor = dark ? 'rgba(255,255,255,0.6)' : 'rgba(0,0,0,0.45)';
  const iconColor = dark ? 'rgba(255,255,255,0.8)' : '#007AFF';

  const bottomBar = (
    <div style={{
      background: toolbarBg,
      backdropFilter: 'blur(20px) saturate(180%)',
      WebkitBackdropFilter: 'blur(20px) saturate(180%)',
      borderTop: dark ? '0.5px solid rgba(255,255,255,0.12)' : '0.5px solid rgba(0,0,0,0.15)',
      padding: '8px 12px 4px',
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      zIndex: 20,
    }}>
      {/* URL bar */}
      <div style={{
        flex: 1,
        background: urlBg,
        borderRadius: 10,
        padding: '7px 12px',
        display: 'flex',
        alignItems: 'center',
        gap: 6,
        boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
      }}>
        <span style={{ fontSize: 11, color: urlColor }}>🔒</span>
        <span style={{ fontFamily: '-apple-system, sans-serif', fontSize: 14, color: urlColor, flex: 1 }}>{url}</span>
      </div>
      {/* Action icons */}
      <span style={{ color: iconColor, fontSize: 20, cursor: 'pointer', userSelect: 'none' }}>⬆</span>
      <span style={{ color: iconColor, fontSize: 18, cursor: 'pointer', userSelect: 'none', fontFamily: '-apple-system, sans-serif' }}>⧉</span>
    </div>
  );

  return (
    <IOSDevice width={width} height={844} dark={dark} time="9:41">
      <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <div style={{ flex: 1, overflow: 'hidden', position: 'relative' }}>
          {children}
        </div>
        {bottomBar}
      </div>
    </IOSDevice>
  );
}

// ---------------------------------------------------------------------------
// Exports — attach to window for global access
// ---------------------------------------------------------------------------

Object.assign(window, {
  IOSDevice,
  IOSStatusBar,
  IOSNavBar,
  IOSGlassPill,
  IOSHomeIndicator,
  AndroidDevice,
  AndroidStatusBar,
  DesktopBrowser,
  MobileBrowser,
});
