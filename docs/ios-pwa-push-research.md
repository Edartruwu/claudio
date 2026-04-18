# iOS Safari PWA Web Push — Technical Research

_April 2026. Focuses on what actually works. Includes specific iOS version numbers and known bugs._

---

## Summary

iOS Web Push works via **APNS as the transport**. The service worker does NOT need to stay alive for push delivery — APNS wakes Safari, which wakes the SW briefly to show the notification. The "stops working when backgrounded" symptom is almost always a **silent push penalty** (missing `event.waitUntil`) or **ITP wiping the SW registration**. The durable fix is **Declarative Web Push** (iOS 18.4+), which decouples subscriptions from SW registrations entirely.

---

## Q1. Why does iOS kill/suspend service workers when backgrounded? What are the exact limits?

**Architecture first:** iOS Web Push does NOT rely on the SW staying alive in the background. The delivery path is:

```
Your server → push.apple.com (Web Push endpoint) → APNS → iOS device → 
  Safari (wakes briefly) → Service Worker wakes for ~30s → showNotification()
```

So a sleeping/backgrounded SW is **not the root cause of missed notifications**. The SW is event-driven — iOS wakes it on demand when a push arrives via APNS.

**The actual SW lifetime limits:**

- SW runs in background ~30 seconds after the triggering event (standard Web Worker timeout).
- iOS does **not** support Background Sync API or Periodic Background Sync — no way to schedule SW wake-ups.
- Safari can terminate the SW earlier if the device is under memory pressure.
- If the phone is in Low Power Mode, APNS delivery may be deferred even for `high` urgency.

**ITP (Intelligent Tracking Prevention) — the real killer:**

- If the user has not visited/opened the PWA in ~7 days (varies), ITP **deletes all website data** for that origin, including the SW registration.
- The push subscription references that SW registration. If the SW registration is gone, iOS cannot route incoming APNS messages to a SW → **push silently fails** with no error to the server.
- `pushsubscriptionchange` event is **NOT supported on iOS Safari** (as of iOS 17/18). The device cannot notify your server that the subscription broke.
- You get a 410 HTTP response from `push.apple.com` when trying to send to a stale token — this is the server-side signal to delete the subscription.

---

## Q2. Is there a way to keep the SW alive on iOS?

**No.** Not in any meaningful sense.

| API | iOS Safari support |
|-----|-------------------|
| Background Sync API | ❌ Not supported |
| Periodic Background Sync | ❌ Not supported |
| Push (APNS-backed) | ✅ Works — but wakes SW on demand, not continuously |
| Background Fetch API | ❌ Not supported |

There is no mechanism to keep the SW running continuously. Design around event-driven wake-ups only.

---

## Q3. Does push delivery work when the PWA is fully closed?

**YES — this is the intended and working behavior.** When a push arrives via APNS and the Home Screen web app is fully closed:

1. iOS receives the APNS message (device-level, not app-level)
2. iOS wakes Safari's SW runner process
3. SW handles the push event and calls `showNotification()`
4. Notification appears in Notification Center

**Required iOS version:** iOS 16.4 (released March 2023) — first iOS version supporting Web Push for Home Screen web apps.

- iOS 16.0–16.3: **No Web Push for PWAs** at all.
- iOS 16.4+: Web Push works for Home Screen apps only (not Safari browser tabs).
- iOS 17.x: Continued support, some reliability improvements.
- iOS 18.4 (March 2025): Declarative Web Push introduced.

**Caveat:** Fully-closed delivery only works if:
1. The SW registration still exists (ITP hasn't deleted it)
2. The push subscription is still valid (not revoked by silent push violations)
3. `userVisibleOnly: true` was set at subscription time (required — Apple enforces this)

---

## Q4. Known bugs and workarounds for reliable push delivery

### Bug 1: Silent push penalty → subscription revoked after ~3 violations

**Cause:** Safari revokes the push subscription if the SW's push event handler fails to show a notification before the event terminates. This counts as a "silent push" violation. After ~3 violations, the subscription is permanently revoked.

**Root cause in most codebases:** Missing `event.waitUntil()`:

```js
// ❌ WRONG — event ends before showNotification resolves
self.addEventListener("push", function(e) {
  self.registration.showNotification(e.data.title, e.data);
});

// ❌ ALSO WRONG — async/await doesn't extend the event lifetime
self.addEventListener("push", async function(e) {
  await self.registration.showNotification(e.data.title, e.data);
});

// ✅ CORRECT — waitUntil extends the event until the promise resolves
self.addEventListener("push", function(e) {
  e.waitUntil(
    self.registration.showNotification(e.data.json().title, e.data.json())
  );
});
```

**Other causes:**
- SW throws an exception before calling `showNotification()`
- Payload parse error (bad JSON)
- `showNotification()` called with invalid options

### Bug 2: ITP removes SW registration → push breaks silently

**Symptom:** Push worked for weeks, then stopped. Server gets no error (send returns 201), but device gets nothing.

**Cause:** ITP deleted the SW registration. The APNS message arrives, Safari tries to wake the SW, finds no registration, silently drops the message.

**Mitigation:**
- Encourage users to open the app regularly (prompt/reminder when they do open it)
- When the app opens, call `navigator.serviceWorker.getRegistration()` and `PushManager.getSubscription()` — if subscription is null, re-subscribe
- On the server: watch for 410 responses from `push.apple.com` → mark subscription invalid → ask user to re-subscribe next time they open the app
- **Best fix:** Migrate to Declarative Web Push (iOS 18.4+) — subscription survives ITP SW deletion

### Bug 3: SW registration exists but subscription is null

Observed in the Apple Developer Forums (Feb 2024): After a push is received, `ServiceWorkerRegistration.pushManager.getSubscription()` returns null — the subscription was revoked mid-flight. This is a known iOS behavior, not fully explained by Apple. Treat null subscription as a trigger to re-subscribe on next user interaction.

---

## Q5. Server-side improvements for delivery rate

### TTL

Set `TTL` header. Apple stores up to the TTL value (max 30 days) if device is offline. `TTL: 0` = deliver now or drop.

```
TTL: 86400     # 24h — reasonable for most alerts
TTL: 604800    # 7 days — for non-time-sensitive
TTL: 0         # ephemeral — drop if not immediately deliverable
```

### Urgency header

Apple explicitly documents the mapping:

| `Urgency` value | APNS behavior |
|-----------------|---------------|
| `high` | Immediate delivery. Maps to `apns-priority: 10`. Wakes device. |
| `normal` (default) | Power-efficient delivery. Maps to `apns-priority: 5`. May be batched. |
| `low` | Deferred. Device may not receive until next natural wake. |
| `very-low` | Very low priority. |

**For sleeping devices: always send `Urgency: high`** — this tells APNS to use the high-priority path that wakes the device.

```
Urgency: high
TTL: 86400
```

### Topic header

Use `Topic` header for coalescing duplicate notifications. Max 32 chars, URL-safe base64 charset. If you send 5 alerts about the same entity while the device is offline, only the last one is delivered.

```
Topic: order-status-update
```

### Payload size

Limit: **4KB** (returns 413 otherwise).

### VAPID JWT

Do not regenerate VAPID JWT more than once per hour (Apple enforces this). Cache and reuse.

---

## Q6. Does `Urgency: high` affect delivery on sleeping device?

**YES, significantly.**

- `Urgency: high` → APNS treats as alert notification (`apns-push-type: alert`, `apns-priority: 10`) → device wakes immediately
- `Urgency: normal` → APNS batches with other notifications, may wait for natural device wake → significant delivery delay when device is sleeping
- Low Power Mode: even `high` urgency may be deferred. Apple does not guarantee immediate delivery in Low Power Mode.

Apple internally maps Web Push urgency to APNS headers. You do not control `apns-priority` directly — it's derived from your `Urgency` header.

---

## Q7. Push subscription expiry problem on iOS

**No `expirationTime`.** iOS push subscriptions return `null` for `expirationTime` — Apple does not pre-announce expiry.

**But subscriptions do expire/revoke in practice:**

| Cause | Signal |
|-------|--------|
| Silent push violation (×3) | 410 from push.apple.com |
| ITP deleted SW registration | 201 from server, but nothing shown on device |
| User removed notification permission | 410 from push.apple.com |
| User uninstalled the Home Screen app | 410 from push.apple.com |

**`pushsubscriptionchange` event:** NOT supported on iOS Safari. No automatic notification when subscription is invalidated. You must detect stale subscriptions by:
1. Server-side: watch for 410 HTTP responses → mark subscription invalid
2. Client-side: on app open, call `getSubscription()` and check for null → re-subscribe if needed

**ITP + subscription orphaning** is the sneakiest case: server gets 201 OK from APNS (the APNS subscription token is still valid), but the SW registration was deleted locally, so the notification never displays. Only way to detect: user reports not receiving notifications, then you call `getSubscription()` and it's null.

---

## Q8. Polling / SSE fallback vs WebSocket + Push

### Capabilities comparison for iOS PWA

| Method | Works in background | Works when closed | Notes |
|--------|--------------------|--------------------|-------|
| Web Push (APNS) | ✅ | ✅ | The only option for background/closed delivery |
| SSE | ❌ | ❌ | Requires open tab/foreground |
| WebSocket | ❌ | ❌ | Requires open tab/foreground |
| Polling (fetch) | ❌ | ❌ | Requires JS execution = foreground |

SSE and WebSocket are **NOT alternatives for push**. They only work while the PWA is open and in the foreground. iOS will terminate the connection when the PWA is backgrounded.

**Recommended hybrid strategy:**
1. **Push notifications** (APNS/Web Push): for background + closed-app delivery
2. **SSE** (not WebSocket — simpler, HTTP/2 compatible, auto-reconnects): for real-time in-app updates while app is foreground. SSE is simpler to implement in Go (`net/http` + `text/event-stream`) and works fine over Tailscale HTTPS.
3. On app foreground: sync missed events via REST API call, then connect SSE for live updates

**WebSocket vs SSE for iOS foreground:**
- Both work in iOS Safari PWA foreground
- SSE preferred for server→client only; auto-reconnects; works over HTTP/2 multiplexing
- WebSocket if bidirectional needed
- Neither has background delivery

---

## Q9. Declarative Web Push (iOS 18.4+) — the real fix

Released March 2025. This is the architectural solution Apple designed to address ITP + SW reliability.

**How it works:**
- Subscribe via `window.pushManager.subscribe()` instead of through a SW registration
- Push payload must match the declarative JSON format:

```json
{
  "web_push": 8030,
  "notification": {
    "title": "New message",
    "body": "You have a new message from Alice",
    "navigate": "https://yourapp.example.com/messages",
    "silent": false,
    "app_badge": "3"
  }
}
```

- The browser renders the notification **without needing a SW** — notification content is in the payload itself
- If a SW is installed, it still gets the `push` event and can modify the notification
- If the SW fails or is deleted by ITP, the browser shows the fallback notification from the payload
- **Subscription survives ITP SW deletion** — this is the key improvement

**Backwards compatibility:**
- Old iOS (16.4–18.3): ignores `web_push: 8030`, tries to dispatch to SW as before
- iOS 18.4+: handles declaratively
- Can send declarative format to both — old iOS falls back to SW handler, new iOS uses declarative

**Migration steps for Go server:**
1. Change push payload to the declarative JSON format
2. Update SW `push` event handler to check `event.notification` (the proposed notification from the payload) and optionally replace it
3. Subscribe via `window.pushManager.subscribe()` for new subscriptions (keep SW-based for existing)

---

## Recommendations (priority order)

1. **Fix `event.waitUntil()` immediately** — most common cause of subscription revocation
2. **Always send `Urgency: high` + `TTL: 86400`** for important notifications
3. **On app open: call `getSubscription()` and re-subscribe if null**
4. **Server: monitor 410 responses** from push.apple.com → delete stale subscriptions
5. **Migrate to Declarative Web Push payload format** — backwards compatible, fixes ITP orphaning for iOS 18.4+ users
6. **Use SSE for in-app real-time** (Go: easy with `text/event-stream`), not as push replacement
7. **Do not implement Background Sync or Periodic Background Sync** — not supported on iOS

---

## Quick reference: iOS version matrix

| iOS version | Web Push (PWA) | Declarative Web Push | `pushsubscriptionchange` |
|-------------|---------------|---------------------|--------------------------|
| < 16.4 | ❌ | ❌ | ❌ |
| 16.4 – 17.x | ✅ | ❌ | ❌ |
| 18.0 – 18.3 | ✅ | ❌ | ❌ |
| 18.4+ | ✅ | ✅ | ❌ |

`pushsubscriptionchange` remains unsupported as of iOS 18.4.

---

## Sources

- [WebKit Blog: Web Push for Web Apps on iOS and iPadOS](https://webkit.org/blog/13878/web-push-for-web-apps-on-ios-and-ipados/)
- [WebKit Blog: Meet Declarative Web Push (March 2025)](https://www.webkit.org/blog/16535/meet-declarative-web-push/)
- [Apple Docs: Sending Web Push Notifications in Web Apps and Browsers](https://docs.developer.apple.com/tutorials/data/documentation/usernotifications/sending-web-push-notifications-in-web-apps-and-browsers.md)
- [Apple Developer Forums: When do web push notification subscriptions expire on iOS?](https://developer.apple.com/forums/thread/727372)
- [WebKit Bug 268797: Push notifications not received when PWA is backgrounded](https://bugs.webkit.org/show_bug.cgi?id=268797)
- [DEV.to / Progressier: How to fix iOS push subscriptions being terminated after 3 notifications](https://dev.to/progressier/how-to-fix-ios-push-subscriptions-being-terminated-after-3-notifications-39a7)
- [RFC 8030: Generic Event Delivery Using HTTP Push](https://www.rfc-editor.org/rfc/rfc8030)
