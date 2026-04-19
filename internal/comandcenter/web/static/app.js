(function () {
  var sessionId = document.body.dataset.sessionId;
  if (!sessionId) return;

  var msgs = document.getElementById('messages');
  var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  var reconnectTimer = null;
  var lastMsgDate = null;

  // --- Helpers ---

  function isNearBottom() {
    if (!msgs) return true;
    return msgs.scrollTop >= msgs.scrollHeight - msgs.clientHeight - 100;
  }

  function scrollToBottom() {
    if (!msgs) return;
    if (isNearBottom()) msgs.scrollTop = msgs.scrollHeight;
  }

  function showBanner() {
    var b = document.getElementById('ws-banner') || document.getElementById('reconnect-banner');
    if (b) b.classList.remove('hidden');
  }

  function hideBanner() {
    var b = document.getElementById('ws-banner') || document.getElementById('reconnect-banner');
    if (b) b.classList.add('hidden');
  }

  function setTyping(text) {
    var t = document.getElementById('typing-indicator');
    if (t) t.textContent = text;
  }

  function showTypingBubble(tool, agentName) {
    var bubble = document.getElementById('typing-bubble');
    if (!bubble || !msgs) return;
    var label = (agentName || 'Agent') + ' is running ' + (tool || 'tool') + '...';
    var textEl = bubble.querySelector('.typing-text');
    if (textEl) textEl.textContent = label;
    bubble.classList.remove('hidden');
    msgs.appendChild(bubble);
    msgs.scrollTop = msgs.scrollHeight;
  }

  function removeTypingBubble() {
    var bubble = document.getElementById('typing-bubble');
    if (bubble) bubble.classList.add('hidden');
  }

  function todayLabel() {
    var d = new Date();
    var opts = { month: 'short', day: 'numeric' };
    return d.toLocaleDateString(undefined, opts);
  }

  function insertDateDivider(label) {
    if (!msgs) return;
    msgs.insertAdjacentHTML('beforeend',
      '<div class="flex justify-center my-2">' +
      '<span class="bg-black/20 text-white text-xs px-3 py-1 rounded-full">' + label + '</span>' +
      '</div>'
    );
  }

  function maybeInsertDateDivider() {
    var today = new Date().toDateString();
    if (lastMsgDate !== today) {
      lastMsgDate = today;
      insertDateDivider('Today');
    }
  }

  function showAgentToast(name, status) {
    var icons = { done: '✅', failed: '❌', working: '⚙️', waiting: '⏳' };
    var icon = icons[status] || '🤖';
    var label = name + ' — ' + status;
    var toast = document.createElement('div');
    toast.style.cssText = [
      'position:fixed', 'bottom:80px', 'left:50%', 'transform:translateX(-50%)',
      'z-index:600', 'background:#1C1C1E', 'border:1px solid #2C2C2E',
      'border-radius:12px', 'padding:10px 16px',
      'display:flex', 'align-items:center', 'gap:8px',
      'font-family:inherit', 'font-size:13px', 'color:#D4DDE8',
      'box-shadow:0 4px 20px rgba(0,0,0,0.6)', 'white-space:nowrap',
      'opacity:1', 'transition:opacity 0.4s'
    ].join(';');
    toast.innerHTML = '<span>' + icon + '</span><span>' + label + '</span>';
    document.body.appendChild(toast);
    setTimeout(function() {
      toast.style.opacity = '0';
      setTimeout(function() { toast.remove(); }, 400);
    }, 4000);
  }

  function appendMessage(html) {
    if (!msgs) return;
    maybeInsertDateDivider();
    var near = isNearBottom();
    var bubble = document.getElementById('typing-bubble');
    if (bubble && !bubble.classList.contains('hidden')) {
      bubble.insertAdjacentHTML('beforebegin', html);
    } else {
      msgs.insertAdjacentHTML('beforeend', html);
    }
    // Only auto-scroll if user was near bottom before the new message arrived
    if (near) msgs.scrollTop = msgs.scrollHeight;
  }

  // Insert initial date divider if messages exist on page load.
  (function () {
    if (msgs && msgs.children.length > 0) {
      var first = msgs.firstElementChild;
      if (first) {
        var divider = document.createElement('div');
        divider.className = 'flex justify-center my-2';
        divider.innerHTML = '<span class="bg-black/20 text-white text-xs px-3 py-1 rounded-full">Today</span>';
        msgs.insertBefore(divider, first);
      }
      lastMsgDate = new Date().toDateString();
    }
  })();

  // --- WebSocket ---

  var wsConnected = false;
  var wsInstance = null;

  function reloadMessages() {
    if (!msgs) return;
    fetch('/partials/messages/' + sessionId)
      .then(function(res) { return res.text(); })
      .then(function(html) {
        msgs.innerHTML = html;
        var bubble = document.getElementById('typing-bubble');
        if (bubble) msgs.appendChild(bubble);
        msgs.scrollTop = msgs.scrollHeight;
        lastMsgDate = new Date().toDateString();
      })
      .catch(function() {});
  }

  function initWS() {
    var ws = new WebSocket(proto + '//' + location.host + '/ws/ui?session_id=' + sessionId);
    wsInstance = ws;

    ws.onopen = function () {
      var wasDisconnected = !wsConnected;
      wsConnected = true;
      hideBanner();
      if (wasDisconnected) reloadMessages();
    };

    ws.onmessage = function (e) {
      try {
        var data = JSON.parse(e.data);
        var type = data.type;

        if (type === 'message.assistant') {
          var sb = document.getElementById('streaming-bubble');
          if (sb) sb.remove();
          removeTypingBubble();
          setTyping('● online');
          if (data.html) appendMessage(data.html);

        } else if (type === 'message.stream_delta') {
          var bubble = document.getElementById('streaming-bubble');
          if (!bubble) {
            bubble = document.createElement('div');
            bubble.id = 'streaming-bubble';
            bubble.className = 'msg-bubble msg-bubble-assistant';
            bubble.style.cssText = 'opacity:0.8; white-space: pre-wrap; font-family: inherit;';
            var msgList = document.getElementById('messages');
            if (msgList) {
              msgList.appendChild(bubble);
              scrollToBottom();
            }
          }
          bubble.textContent = data.accumulated;
          scrollToBottom();

        } else if (type === 'typing') {
          showTypingBubble(data.tool, data.agentName);
          setTyping((data.agentName || 'Agent') + ' is working...');

        } else if (type === 'message.user') {
          if (data.html) appendMessage(data.html);

        } else if (type === 'message.tool_use') {
          var sb = document.getElementById('streaming-bubble');
          if (sb) sb.remove();
          if (data.html) appendMessage(data.html);

        } else if (type === 'messages.cleared') {
          var msgsEl = document.getElementById('messages');
          if (msgsEl) msgsEl.innerHTML = '';

        } else if (type === 'messages.compacted') {
          var msgsEl = document.getElementById('messages');
          if (msgsEl && sessionId) {
            fetch('/partials/messages/' + sessionId, { credentials: 'include' })
              .then(function(r) { return r.text(); })
              .then(function(html) { msgsEl.innerHTML = html; msgsEl.scrollTop = msgsEl.scrollHeight; })
              .catch(function() {});
          }
          var loader = document.getElementById('compact-loading');
          if (loader) loader.style.display = 'none';

        } else if (type === 'agent_status') {
          showAgentToast(data.name, data.status);
          if (window.htmx) { htmx.trigger(document.body, 'refresh'); }
          if (msgs) {
            var agentIcon = data.status === 'complete' ? '✅' : data.status === 'failed' ? '❌' : data.status === 'working' ? '⚙️' : '⏳';
            var agentNotif = document.createElement('div');
            agentNotif.className = 'text-xs text-center py-1';
            agentNotif.style.color = '#8E8E93';
            agentNotif.textContent = agentIcon + ' ' + data.name + ' — ' + data.status;
            msgs.appendChild(agentNotif);
            msgs.scrollTop = msgs.scrollHeight;
          }

        } else if (type === 'new_message' && data.html) {
          var isToolUse  = data.html.indexOf('msg-bubble-tool') !== -1;
          var isAssistant = data.html.indexOf('msg-bubble-assistant') !== -1;
          if (isAssistant) {
            removeTypingBubble();
            setTyping('● online');
          } else if (isToolUse) {
            setTyping('typing...');
          }
          appendMessage(data.html);
        }
      } catch (_) {}
    };

    ws.onerror = function () {
      // Handled by onclose.
    };

    ws.onclose = function () {
      wsConnected = false;
      showBanner();
      if (reconnectTimer) clearTimeout(reconnectTimer);
      reconnectTimer = setTimeout(initWS, 3000);
    };
  }

  // htmx:wsClose / htmx:wsOpen — for htmx WS extension compatibility
  document.addEventListener('htmx:wsClose', function() { showBanner(); });
  document.addEventListener('htmx:wsOpen',  function() { hideBanner(); });

  // On app resume (phone unlock / tab focus), reconnect immediately.
  document.addEventListener('visibilitychange', function() {
    if (document.visibilityState === 'visible') {
      var dead = !wsInstance ||
                 wsInstance.readyState === WebSocket.CLOSED ||
                 wsInstance.readyState === WebSocket.CLOSING;
      if (dead) {
        if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
        initWS();
      }
    }
  });

  initWS();
})();

// --- DOMContentLoaded: skeleton CSS + push-prompt banner ---
document.addEventListener('DOMContentLoaded', function() {

  // 1. Inject skeleton shimmer + streaming cursor CSS
  (function() {
    var style = document.createElement('style');
    style.textContent =
      '.skeleton{' +
        'background:linear-gradient(90deg,var(--color-surface) 25%,var(--color-surfaceHigh) 50%,var(--color-surface) 75%);' +
        'background-size:200% 100%;animation:shimmer 1.5s infinite;border-radius:4px;' +
      '}' +
      '@keyframes shimmer{0%{background-position:200% 0}100%{background-position:-200% 0}}' +
      '.streaming-cursor::after{content:"▋";animation:blink 1s step-end infinite;color:var(--color-ai);}' +
      '@keyframes blink{50%{opacity:0}}';
    document.head.appendChild(style);
  })();

  // 2. Push-prompt banner — show once if permission not yet decided and SW available
  (function() {
    if (typeof Notification === 'undefined') return;
    if (Notification.permission !== 'default') return;
    if (!('serviceWorker' in navigator)) return;
    if (localStorage.getItem('push-dismissed') !== null) return;
    // Only show in chat view (session page)
    if (!document.body.dataset.sessionId) return;

    // Guard: don't inject twice
    if (document.getElementById('push-prompt')) return;

    var banner = document.createElement('div');
    banner.id = 'push-prompt';
    banner.style.cssText = [
      'position:fixed',
      'bottom:80px',
      'left:12px',
      'right:12px',
      'z-index:500',
      'background:#1C1C1E',
      'border-radius:16px',
      'padding:16px',
      'box-shadow:0 4px 24px rgba(0,0,0,0.6)',
      'border:1px solid #2C2C2E',
      'font-family:inherit'
    ].join(';');

    banner.innerHTML =
      '<div style="display:flex;align-items:flex-start;gap:12px;">' +
        '<div style="width:40px;height:40px;border-radius:10px;background:#075E54;display:flex;align-items:center;justify-content:center;flex-shrink:0;font-size:20px;">🔔</div>' +
        '<div style="flex:1;min-width:0;">' +
          '<p style="color:#D4DDE8;font-weight:600;font-size:15px;margin:0 0 4px 0;">Enable notifications</p>' +
          '<p style="color:#8A9BA0;font-size:13px;margin:0 0 12px 0;">Get notified when agents finish tasks or need input.</p>' +
          '<div style="display:flex;gap:8px;">' +
            '<button id="push-dismiss-btn" style="flex:1;padding:10px;border-radius:10px;border:1px solid #3A3A3C;background:transparent;color:#8A9BA0;font-family:inherit;font-size:14px;cursor:pointer;">Not now</button>' +
            '<button id="push-enable-btn"  style="flex:2;padding:10px;border-radius:10px;border:none;background:#25D366;color:#000;font-family:inherit;font-size:14px;font-weight:600;cursor:pointer;">Enable</button>' +
          '</div>' +
        '</div>' +
      '</div>';

    document.body.appendChild(banner);

    document.getElementById('push-dismiss-btn').addEventListener('click', function() {
      localStorage.setItem('push-dismissed', '1');
      banner.remove();
    });

    document.getElementById('push-enable-btn').addEventListener('click', function() {
      banner.remove();
      subscribePush();
    });
  })();

  // --- Push subscription helper (used by prompt banner) ---
  function urlBase64ToUint8Array(base64String) {
    var padding = '='.repeat((4 - base64String.length % 4) % 4);
    var base64  = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
    var raw     = atob(base64);
    var out     = new Uint8Array(raw.length);
    for (var i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
    return out;
  }

  function subscribePush() {
    if (!('serviceWorker' in navigator)) return;
    Notification.requestPermission().then(function(perm) {
      if (perm !== 'granted') return;
      return Promise.all([
        navigator.serviceWorker.ready,
        fetch('/api/push/vapid-public-key').then(function(r) { return r.json(); })
      ]).then(function(results) {
        var reg  = results[0];
        var data = results[1];
        if (!data.publicKey) return;
        var appKey = urlBase64ToUint8Array(data.publicKey);
        return reg.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: appKey
        });
      }).then(function(sub) {
        if (!sub) return;
        var j     = sub.toJSON();
        var token = (window.CLAUDIO_TOKEN || '');
        var headers = { 'Content-Type': 'application/json' };
        if (token) headers['Authorization'] = 'Bearer ' + token;
        return fetch('/api/push/subscribe', {
          method: 'POST',
          headers: headers,
          body: JSON.stringify({
            endpoint: j.endpoint,
            keys: { p256dh: j.keys.p256dh, auth: j.keys.auth }
          })
        });
      });
    }).catch(function() {});
  }
});
