(function () {
  var _gen = 0; // incremented on every startChat call; stale closures check this
  var _activeWS = null; // single active WS; closed before each new startChat

  function startChat(sessionId) {
    if (!sessionId) return;
    var _myGen = ++_gen;
    // Close previous WS immediately so its onmessage/appendMessage never fires again.
    if (_activeWS && _activeWS.readyState <= 1) {
      _activeWS.onmessage = null;
      _activeWS.onclose = null;
      _activeWS.close();
      _activeWS = null;
    }

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
    if (isNearBottom()) msgs.scrollTop = msgs.scrollHeight;
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
    if (_reloading) return; // reload in progress — message will be in the reloaded HTML
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
    // Always start at the bottom when entering a chat
    if (msgs) msgs.scrollTop = msgs.scrollHeight;
  })();

  // --- WebSocket ---

  var wsConnected = false;
  var wsInstance = null;
  var _hadConnected = false;   // true after first successful onopen in this startChat call
  var _streaming = false;      // true while stream_delta events are arriving
  var _reloading = false;      // true while reloadMessages() fetch is in flight
  var _reloadTimer = null;     // debounce handle for reconnect reloads

  function reloadMessages() {
    if (!msgs || _streaming) return;
    _reloading = true;
    fetch('/partials/messages/' + sessionId)
      .then(function(res) { return res.text(); })
      .then(function(html) {
        if (_streaming) { _reloading = false; return; } // streaming started during fetch — don't clobber
        var near = isNearBottom();
        msgs.innerHTML = html;
        _reloading = false;
        var bubble = document.getElementById('typing-bubble');
        if (bubble) msgs.appendChild(bubble);
        if (near) msgs.scrollTop = msgs.scrollHeight;
        lastMsgDate = new Date().toDateString();
      })
      .catch(function() { _reloading = false; });
  }

  function initWS() {
    var ws = new WebSocket(proto + '//' + location.host + '/ws/ui?session_id=' + sessionId);
    wsInstance = ws;
    _activeWS = ws;

    ws.onopen = function () {
      var isReconnect = _hadConnected && !wsConnected;
      _hadConnected = true;
      wsConnected = true;
      hideBanner();
      // Only reload on true reconnects (not initial page load — server already rendered).
      // Delay 600ms so any WS messages that arrive right after reconnect are processed first.
      if (isReconnect) {
        if (_reloadTimer) clearTimeout(_reloadTimer);
        _reloadTimer = setTimeout(reloadMessages, 600);
      }
    };

    ws.onmessage = function (e) {
      try {
        var data = JSON.parse(e.data);
        var type = data.type;

        if (type === 'ping') {
          ws.send(JSON.stringify({ type: 'pong' }));
          return;
        }

        if (type === 'message.assistant') {
          _streaming = false;
          if (_reloadTimer) { clearTimeout(_reloadTimer); _reloadTimer = null; }
          var sb = document.getElementById('streaming-bubble');
          if (sb) sb.remove();
          removeTypingBubble();
          setTyping('● online');
          if (data.html) appendMessage(data.html);

        } else if (type === 'message.stream_delta') {
          _streaming = true;
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

        } else if (type === 'message.tool_result') {
          // Find the tool_use bubble by toolUseID and inject the output section.
          var bubble = document.querySelector('[data-tool-use-id="' + data.toolUseID + '"]');
          if (bubble) {
            var outputSection = bubble.querySelector('.tool-output-section');
            if (!outputSection) {
              var container = bubble.querySelector('.tool-sections');
              if (container && data.output) {
                var section = document.createElement('div');
                section.className = 'tool-output-section';
                section.innerHTML = '<p class="text-xs font-semibold mb-1" style="color:var(--color-textMuted);">Output</p>' +
                  '<pre class="text-xs overflow-auto whitespace-pre-wrap rounded p-2" style="color:var(--color-textSecondary);background:var(--color-bg);max-height:200px;">' +
                  data.output.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;') + '</pre>';
                container.appendChild(section);
              }
            }
          }

        } else if (type === 'messages.cleared') {
          var msgsEl = document.getElementById('messages');
          if (msgsEl) msgsEl.innerHTML = '';

        } else if (type === 'task.created' || type === 'task.updated') {
          if (window.htmx) { htmx.trigger(document.body, 'refresh'); }

        } else if (type === 'messages.reload') {
          if (_streaming) return;
          var msgsEl = document.getElementById('messages');
          if (msgsEl && sessionId) {
            _reloading = true;
            fetch('/partials/messages/' + sessionId, { credentials: 'include' })
              .then(function(r) { return r.text(); })
              .then(function(html) { var nb = msgsEl.scrollTop >= msgsEl.scrollHeight - msgsEl.clientHeight - 100; msgsEl.innerHTML = html; _reloading = false; if (nb) msgsEl.scrollTop = msgsEl.scrollHeight; })
              .catch(function() { _reloading = false; });
          }

        } else if (type === 'messages.compacted') {
          if (_streaming) return;
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
            var notifId = 'agent-status-' + data.name.replace(/[^a-z0-9]/gi, '-');
            var agentNotif = document.getElementById(notifId);
            if (!agentNotif) {
              agentNotif = document.createElement('div');
              agentNotif.id = notifId;
              agentNotif.className = 'text-xs text-center py-1';
              agentNotif.style.color = '#8E8E93';
              msgs.appendChild(agentNotif);
            }
            agentNotif.textContent = agentIcon + ' ' + data.name + ' — ' + data.status;
            if (isNearBottom()) msgs.scrollTop = msgs.scrollHeight;
          }

        } else if (type === 'agent.log') {
          if (window._ccLogAgent && data.agent_name === window._ccLogAgent.name && data.session_id === window._ccLogAgent.sessionID) {
            fetch('/chat/' + data.session_id + '/agents/' + encodeURIComponent(data.agent_name) + '/logs', {credentials:'include'})
              .then(function(r){return r.text();})
              .then(function(html){
                var body = document.getElementById('agent-log-body');
                if (!body) return;
                var near = body.scrollTop >= body.scrollHeight - body.clientHeight - 50;
                body.innerHTML = html;
                if (near) body.scrollTop = body.scrollHeight;
              });
          }

        } else if (type === 'config.changed') {
          // Update model display if present on the page.
          var modelEl = document.getElementById('current-model');
          if (modelEl && data.model) modelEl.textContent = data.model;
          var permEl = document.getElementById('current-permission-mode');
          if (permEl && data.permission_mode) permEl.textContent = data.permission_mode;
          if (window.htmx) { htmx.trigger(document.body, 'refresh'); }

        } else if (type === 'agent.changed') {
          var agentEl = document.getElementById('current-agent');
          if (agentEl) agentEl.textContent = data.agent_type || 'default';
          if (window.htmx) { htmx.trigger(document.body, 'refresh'); }

        } else if (type === 'team.changed') {
          var teamEl = document.getElementById('current-team');
          if (teamEl) teamEl.textContent = data.team_template || 'none';
          if (window.htmx) { htmx.trigger(document.body, 'refresh'); }

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
      // Don't reconnect if a newer startChat call has taken over.
      if (_myGen !== _gen) return;
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
  } // end startChat

  // Initial call on page load.
  startChat((document.getElementById('chat-app') || document.body).dataset.sessionId);

  // Re-init when HTMX swaps in a new chat view (session navigation without full reload).
  // Only trigger when #main itself was swapped — NOT for sidebar/session-list polls.
  document.addEventListener('htmx:afterSwap', function(evt) {
    var target = evt.detail && evt.detail.target;
    var main = document.getElementById('main');
    // Ignore swaps that didn't touch #main (e.g. session-list 3s poll, info panel, etc.)
    if (!main || !target || (target !== main && !main.contains(target))) return;
    var el = document.getElementById('chat-app');
    if (el && el.dataset.sessionId) startChat(el.dataset.sessionId);
  });
})();

// --- Task detail toggle ---
window.toggleTaskDetail = function(el) {
  var detail = el.parentElement && el.parentElement.querySelector('.task-detail');
  if (!detail) return;
  var chevron = el.querySelector('.task-chevron');
  if (!detail.classList.contains('hidden')) {
    // Close
    detail.innerHTML = '';
    detail.classList.add('hidden');
    if (chevron) chevron.style.transform = '';
  } else {
    // Open — reveal first, then let htmx fire via custom event
    detail.classList.remove('hidden');
    if (chevron) chevron.style.transform = 'rotate(90deg)';
    htmx.trigger(el, 'open-detail');
  }
};

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

    // Show the templ-rendered prompt if present; otherwise no-op.
    var prompt = document.getElementById('push-prompt');
    if (prompt) prompt.style.display = 'flex';
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

  // Global handlers called by PushPrompt templ component buttons.
  window.ccSubscribePush = function() {
    var p = document.getElementById('push-prompt');
    if (p) p.style.display = 'none';
    subscribePush();
  };
  window.ccDismissPush = function() {
    localStorage.setItem('push-dismissed', '1');
    var p = document.getElementById('push-prompt');
    if (p) p.style.display = 'none';
  };
});
