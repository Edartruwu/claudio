# PWA Home Screen Icon Pixelation on iPhone — Research Summary

## Summary

**Root cause**: 180×180 IS the correct size for iPhone 14/15 Pro (3x retina). Pixelation is almost always either (a) a stale cached icon from a previous bad export, or (b) iOS falling back to a manifest icon instead of the `apple-touch-icon`. Fix: version the icon filename and re-add.

---

## Findings by Question

### 1. `apple-touch-icon` vs Manifest Icons — Which Wins?

**`apple-touch-icon` link tag always wins on iOS.** From firt.dev (Maximiliano Firtman, authoritative PWA compatibility tracker):
> "If you still have that element [apple-touch-icon link], it overrides your manifest icons declaration."

- iOS 15.4+ added support for manifest `icons` field
- But if `<link rel="apple-touch-icon">` exists in HTML, iOS uses it exclusively
- **Action**: If manifest icons are wrong/bad size, they won't matter — but if the `apple-touch-icon` link tag is missing or broken, iOS falls back to manifest icons (or a screenshot)

### 2. Correct Size for iPhone 14/15 Pro (3x Retina)

Home screen icons render at **60pt**. At 3× scale: `60 × 3 = 180px`.

**180×180 IS the correct target size.** No larger size is needed or used.

Full size matrix:
| Device | Scale | Required px |
|---|---|---|
| iPhone 14/15 Pro | 3× | **180×180** |
| iPhone SE / older | 2× | 120×120 |
| iPad Pro | 2× | 167×167 |
| iPad (non-Pro) | 2× | 152×152 |

Providing all four via `sizes` attribute is best practice, but 180×180 alone covers all modern iPhones.

### 3. iOS Icon Caching — Does Remove + Re-Add Work?

**No, not reliably.** iOS caches the icon image at add-to-homescreen time. HTTP `Cache-Control` headers are frequently ignored by Safari/WebKit for this asset. Removing the PWA and re-adding may serve the cached image from Safari's disk cache rather than re-fetching.

**Only reliable cache-bust: change the icon URL (version the filename).**

```html
<!-- Bad: iOS may serve cached old.png even after re-add -->
<link rel="apple-touch-icon" href="/apple-touch-icon.png">

<!-- Good: v2 URL forces fresh fetch -->
<link rel="apple-touch-icon" href="/apple-touch-icon-v2.png">
```

After updating the URL, user must remove + re-add the PWA.

### 4. `apple-touch-icon` vs `apple-touch-icon-precomposed`

**No functional difference since iOS 7 (2013).** The `precomposed` variant originally suppressed iOS's glossy shine overlay. Apple removed that effect in iOS 7. Both variants render identically today. Using `precomposed` does no harm; it's just unnecessary legacy naming.

### 5. iOS 17/18 Known Icon Bugs

No confirmed iOS 17/18 bug specifically causing icon resolution degradation. Known PWA regressions in iOS 17 were around:
- Storage/session data loss (fixed in 17.4)
- Icons not appearing at all when manifest was malformed

No Apple-acknowledged bug exists for 3× retina icon blurriness. Pixelation is almost always environmental (cache, wrong source, wrong file served).

### 6. Recommended Approach 2024/2025

```html
<!-- In <head> — apple-touch-icon takes priority over manifest on iOS -->
<link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon-180x180.png">
<link rel="apple-touch-icon" sizes="167x167" href="/apple-touch-icon-167x167.png">
<link rel="apple-touch-icon" sizes="152x152" href="/apple-touch-icon-152x152.png">
<link rel="apple-touch-icon" sizes="120x120" href="/apple-touch-icon-120x120.png">
```

And in `manifest.json` for Android/cross-platform:
```json
{
  "icons": [
    { "src": "/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icon-512.png", "sizes": "512x512", "type": "image/png" }
  ]
}
```

Export apple-touch-icon.png:
- PNG (lossless, not JPEG-in-PNG disguise)
- No transparency (iOS fills transparent areas with **black**)
- No pre-rounded corners (iOS applies its own mask — double-rounding looks bad)
- No pre-applied shadow
- Solid background color filling full canvas

### 7. Black Background + Rounded Corner Masking

A solid black background does **not** cause pixelation. However:
- Transparency in the icon → iOS fills with black → fine, matches intent
- iOS applies ~22% corner radius mask with anti-aliasing
- On dark wallpapers: black bg blends; on light wallpapers: hard edge visible but not pixelated
- Anti-aliasing on the mask edge can look soft — this is normal, not a bug

If the icon *looks* pixelated, the cause is the source file or cache, not the masking.

### 8. Cache Headers to Force Refresh

iOS does read HTTP cache headers for the initial fetch but frequently ignores them for cached web clip icons. Best practices:

```
Cache-Control: no-cache, must-revalidate
ETag: "your-hash-here"
```

These *may* help on re-fetch but are unreliable. **URL versioning is the only guaranteed method.**

```
/apple-touch-icon-v3.png
```

---

## Root Cause Checklist (Most Likely → Least Likely)

1. **Stale cache** — iOS showing old low-res icon. Fix: version filename, remove + re-add.
2. **Wrong file being served** — Verify `curl -I https://yourdomain.com/apple-touch-icon.png` returns 200 and correct `Content-Length`.
3. **Manifest icons used instead** — If link tag is missing/broken, iOS uses manifest. Check manifest has 192×192 and 512×512 PNG entries.
4. **PNG actually JPEG-compressed** — Open in hex editor; first bytes should be `89 50 4E 47` (PNG), not `FF D8 FF` (JPEG).
5. **Double-rounding** — Source image has pre-baked rounded corners → remove them, export as square.

---

## Exact Fix Steps

```bash
# 1. Export fresh 180x180 PNG from your 1024x1024 source
#    Use ImageMagick: sharp corners, lossless, no transparency
convert source-1024x1024.png -resize 180x180 apple-touch-icon-v2.png

# 2. Verify it's actually PNG
file apple-touch-icon-v2.png
# Expected: "PNG image data, 180 x 180, 8-bit/color RGB..."

# 3. Deploy to a versioned URL
# /apple-touch-icon-v2.png

# 4. Update HTML link tag
# <link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon-v2.png">

# 5. Set permissive cache headers on the icon
# Cache-Control: no-cache

# 6. On test device: delete PWA → clear Safari cache (Settings > Safari > Clear)
#    → re-add PWA from fresh Safari load
```

---

## Sources

- [firt.dev — iOS PWA Compatibility](https://firt.dev/notes/pwa-ios/) — authoritative iOS PWA support matrix
- [MDN — Define PWA App Icons](https://developer.mozilla.org/en-US/docs/Web/Progressive_web_apps/How_to/Define_app_icons)
- [naildrivin5.com — PWA on iOS braindump](https://naildrivin5.com/blog/2023/08/24/braindump-of-pwa-on-ios.html)
- [premiumfavicon.com — Apple Touch Icon Complete Guide](https://www.premiumfavicon.com/blog/apple-touch-icon-guide)
- [Apple Developer Forums — iOS 17 PWA issues](https://developer.apple.com/forums/thread/737827)
