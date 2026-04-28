(function () {
  var _gen = 0; // incremented on every startChat call; stale closures check this
  var _activeWS = null; // single active WS; closed before each new startChat
  var wsConnected = false;  // hoisted — shared across startChat calls
  window._wsConnected = false; // exposed for htmx polling conditional
  var isConnecting = false; // guard: prevents concurrent initWS calls
  var reconnectTimer = null; // hoisted — cleared/set from any closure
  var _initWSFn = null;     // always points to current-session initWS
  var reconnectAttempt = 0; // exponential backoff counter; reset on successful connect

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
    // Reset shared state for new session
    wsConnected = false;
    window._wsConnected = false;
    isConnecting = false;
    if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }

  var msgs = document.getElementById('messages');
  var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
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

  function showThinkingBubble() {
    var bubble = document.getElementById('typing-indicator-dots');
    if (!bubble || !msgs) return;
    var textEl = bubble.querySelector('.typing-text');
    if (textEl) textEl.textContent = 'Thinking...';
    bubble.classList.remove('hidden');
    msgs.appendChild(bubble);
    if (isNearBottom()) msgs.scrollTop = msgs.scrollHeight;
  }

  function hideThinkingBubble() {
    var bubble = document.getElementById('typing-indicator-dots');
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

  // updateAgentCard — targeted in-place update for a single agent card.
  // Finds the card by data-agent-name and patches status dot, label, avatar pulse,
  // and tool chip without a server round-trip.
  function updateAgentCard(name, status, currentTool) {
    var card = document.querySelector('[data-agent-name="' + name + '"]');
    if (!card) return;
    // Ephemeral agents (Agent-tool spawns) signal completion with "removed" — delete the card.
    if (status === 'removed') { card.remove(); return; }

    // Avatar pulse ring (running → add class, else remove)
    var avatar = card.querySelector('.flex.items-center.justify-center');
    if (avatar) {
      if (status === 'running') {
        avatar.classList.add('agent-pulse');
      } else {
        avatar.classList.remove('agent-pulse');
      }
    }

    // Body is second child of card
    var bodyDiv = card.children[1];
    if (!bodyDiv) return;

    // Tool chip lives in name+chip row (first child of body)
    var nameRow = bodyDiv.children[0];
    if (nameRow) {
      var chipSpan = nameRow.querySelector('span');
      if (status === 'running' && currentTool) {
        if (!chipSpan) {
          chipSpan = document.createElement('span');
          chipSpan.style.cssText = 'background:var(--color-toolDim);color:var(--color-tool);font-size:11px;font-weight:600;letter-spacing:0.5px;border-radius:9999px;padding:2px 8px;white-space:nowrap;flex-shrink:0;';
          nameRow.appendChild(chipSpan);
        }
        chipSpan.textContent = currentTool;
      } else if (chipSpan) {
        chipSpan.remove();
      }
    }

    // Status dot + label row (second child of body)
    var statusRow = bodyDiv.children[1];
    if (!statusRow) return;
    var dot = statusRow.children[0];
    var label = statusRow.children[1];
    var color = (status === 'running') ? 'var(--color-brand)'
              : (status === 'done' || status === 'inactive') ? 'var(--color-textMuted)'
              : 'var(--color-tool)';
    if (dot) dot.style.background = color;
    if (label) { label.textContent = status; label.style.color = color; }
  }

  var _agentToast = null;
  function showAgentToast(name, status) {
    if (status !== 'complete' && status !== 'failed') return;
    if (_agentToast) { _agentToast.remove(); _agentToast = null; }
    var icon = status === 'complete' ? '✅' : '❌';
    var toast = document.createElement('div');
    toast.id = 'agent-toast';
    toast.style.cssText = [
      'position:fixed', 'bottom:calc(80px + env(safe-area-inset-bottom, 0px))', 'left:50%', 'transform:translateX(-50%)',
      'z-index:600', 'background:#1C1C1E', 'border:1px solid #2C2C2E',
      'border-radius:12px', 'padding:10px 16px',
      'display:flex', 'align-items:center', 'gap:8px',
      'font-family:inherit', 'font-size:13px', 'color:#D4DDE8',
      'box-shadow:0 4px 20px rgba(0,0,0,0.6)', 'white-space:nowrap',
      'opacity:1', 'transition:opacity 0.4s'
    ].join(';');
    var iconEl = document.createElement('span');
    iconEl.textContent = icon;
    var labelEl = document.createElement('span');
    labelEl.textContent = name + ' — ' + status;
    toast.appendChild(iconEl);
    toast.appendChild(labelEl);
    document.body.appendChild(toast);
    _agentToast = toast;
    setTimeout(function() {
      toast.style.opacity = '0';
      setTimeout(function() { if (_agentToast === toast) _agentToast = null; toast.remove(); }, 400);
    }, 3000);
  }

  function appendMessage(html) {
    if (!msgs) return;
    if (_reloading) return; // reload in progress — message will be in the reloaded HTML
    maybeInsertDateDivider();
    var near = isNearBottom();
    var bubble = document.getElementById('typing-bubble');
    var anchor = document.getElementById('chat-bottom');
    if (bubble && !bubble.classList.contains('hidden')) {
      bubble.insertAdjacentHTML('beforebegin', html);
    } else if (anchor) {
      anchor.insertAdjacentHTML('beforebegin', html); // keep anchor always last
    } else {
      msgs.insertAdjacentHTML('beforeend', html);
    }
    if (near) {
      var a = document.getElementById('chat-bottom');
      if (a) a.scrollIntoView(); else msgs.scrollTop = msgs.scrollHeight;
    }
  }

  // Init: set lastMsgDate so maybeInsertDateDivider doesn't re-insert on first WS message.
  // Server already renders the "Today" badge via MessagesContainer — do NOT insert one here.
  (function () {
    if (msgs && msgs.children.length > 0) {
      lastMsgDate = new Date().toDateString();
    }
    // Always start at the bottom. Use scrollIntoView on the anchor — more
    // reliable than scrollTop=scrollHeight because it runs after layout settles.
    function _scrollToBottom() {
      var anchor = document.getElementById('chat-bottom');
      if (anchor) { anchor.scrollIntoView(); return; }
      if (msgs) msgs.scrollTop = msgs.scrollHeight;
    }
    requestAnimationFrame(function() {
      requestAnimationFrame(function() {
        _scrollToBottom();
        setTimeout(_scrollToBottom, 150); // fallback for slow/large layouts
      });
    });
  })();

  // --- WebSocket ---

  var wsInstance = null;
  var _hadConnected = false;   // true after first successful onopen in this startChat call
  var _streaming = false;      // true while stream_delta events are arriving
  var _reloading = false;      // true while reloadMessages() fetch is in flight
  var _reloadTimer = null;     // debounce handle for reconnect reloads

  function reloadMessages() {
    if (!msgs || _streaming) return;
    _reloading = true;
    var near = isNearBottom();
    htmx.ajax('GET', '/partials/messages/' + sessionId, {
      target: '#messages', swap: 'innerHTML'
    }).then(function() {
      _reloading = false;
      if (_streaming) return;
      var bubble = document.getElementById('typing-bubble');
      if (bubble) msgs.appendChild(bubble);
      if (near) msgs.scrollTop = msgs.scrollHeight;
      lastMsgDate = new Date().toDateString();
    }).catch(function() { _reloading = false; });
  }

  function initWS() {
    if (wsConnected || isConnecting) return;
    isConnecting = true;
    var ws = new WebSocket(proto + '//' + location.host + '/ws/ui?session_id=' + sessionId);
    wsInstance = ws;
    _activeWS = ws;

    ws.onopen = function () {
      isConnecting = false;
      if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
      reconnectAttempt = 0;
      var isReconnect = _hadConnected && !wsConnected;
      _hadConnected = true;
      wsConnected = true;
      window._wsConnected = true;
      hideBanner();
      // Only reload on true reconnects (not initial page load — server already rendered).
      // Delay 600ms so any WS messages that arrive right after reconnect are processed first.
      if (isReconnect) {
        // Don't reload full history on reconnect — it re-injects old tool results as if they're new.
        // Only reload if the container is empty (e.g. navigated away and back).
        if (_reloadTimer) clearTimeout(_reloadTimer);
        if (msgs && msgs.children.length === 0) {
          _reloadTimer = setTimeout(reloadMessages, 600);
        }
        // Refresh team list only if team tab is currently active.
        var teamTab = document.getElementById('tab-team');
        teamTab = teamTab && teamTab.getAttribute('aria-selected') === 'true' ? teamTab : null;
        if (teamTab && window.htmx) {
          htmx.ajax('GET', '/api/sessions/' + sessionId + '/team', {
            target: '#team-members-list',
            swap: 'innerHTML'
          });
        }
      }
    };

    ws.onmessage = function (e) {
      try {
        var data = JSON.parse(e.data);
        if (!data || typeof data !== 'object' || !data.type) {
          console.warn('[claudio-ws] invalid payload shape:', e.data);
          return;
        }
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
          hideThinkingBubble();
          setTyping('● online');
          if (data.html) appendMessage(data.html);

        } else if (type === 'message.stream_delta') {
          if (data.agent_name) {
            // Sub-agent delta — route to agent log dialog if it's open for this agent.
            if (window._ccLogAgent && data.agent_name === window._ccLogAgent.name) {
              var logContent = document.getElementById('agent-log-content');
              if (logContent) {
                var agentBubble = logContent.querySelector('#agent-streaming-bubble');
                if (!agentBubble) {
                  var emptyEl = logContent.querySelector('#agent-log-empty');
                  if (emptyEl) emptyEl.remove();
                  agentBubble = document.createElement('div');
                  agentBubble.id = 'agent-streaming-bubble';
                  agentBubble.className = 'msg-bubble msg-bubble-assistant';
                  agentBubble.style.cssText = 'opacity:0.8; white-space: pre-wrap; font-family: inherit; margin-bottom: 8px;';
                  logContent.appendChild(agentBubble);
                }
                agentBubble.textContent = data.accumulated;
                logContent.scrollTop = logContent.scrollHeight;
              }
            }
            return; // Never show sub-agent deltas in main chat bubble.
          }
          hideThinkingBubble();
          // Drop stale delta if message.assistant already finalized this turn.
          if (!_streaming && !document.getElementById('streaming-bubble')) return;
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
          hideThinkingBubble();
          showTypingBubble(data.tool, data.agentName);
          setTyping((data.agentName || 'Agent') + ' is working...');

        } else if (type === 'message.user') {
          if (data.html) appendMessage(data.html);
          showThinkingBubble();

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
          showThinkingBubble();

        } else if (type === 'messages.cleared') {
          var msgsEl = document.getElementById('messages');
          if (msgsEl) msgsEl.innerHTML = '';

        } else if (type === 'message.deleted') {
          var del = document.querySelector('[data-msg-id="' + data.message_id + '"]');
          if (del) {
            del.style.transition = 'opacity .3s, max-height .3s';
            del.style.opacity = '0';
            del.style.maxHeight = '0';
            del.style.overflow = 'hidden';
            setTimeout(function() { del.remove(); }, 300);
          }

        } else if (type === 'task.created' || type === 'task.updated') {
          if (window.htmx) { htmx.trigger(document.body, 'refresh'); }

        } else if (type === 'messages.reload') {
          if (_streaming) return;
          if (sessionId) {
            _reloading = true;
            var msgsEl = document.getElementById('messages');
            var nb = msgsEl && msgsEl.scrollTop >= msgsEl.scrollHeight - msgsEl.clientHeight - 100;
            htmx.ajax('GET', '/partials/messages/' + sessionId, {
              target: '#messages', swap: 'innerHTML'
            }).then(function() { _reloading = false; if (nb) { var m = document.getElementById('messages'); if (m) m.scrollTop = m.scrollHeight; } })
              .catch(function() { _reloading = false; });
          }

        } else if (type === 'messages.compacted') {
          if (_streaming) return;
          if (sessionId) {
            htmx.ajax('GET', '/partials/messages/' + sessionId, {
              target: '#messages', swap: 'innerHTML'
            }).then(function() { var m = document.getElementById('messages'); if (m) m.scrollTop = m.scrollHeight; });
          }
          var loader = document.getElementById('compact-loading');
          if (loader) loader.style.display = 'none';

        } else if (type === 'agent_status') {
          // Targeted card update — always (live + replay).
          updateAgentCard(data.name, data.status, data.current_tool || '');
          // Toast + session refresh only for live events, not replays on reconnect.
          if (data.replay !== 'true') {
            showAgentToast(data.name, data.status);
            if (window.refreshSessionList) window.refreshSessionList();
          }

        } else if (type === 'agent.log') {
          if (window._ccLogAgent && data.agent_name === window._ccLogAgent.name && data.session_id === window._ccLogAgent.sessionID) {
            htmx.ajax('GET', '/chat/' + data.session_id + '/agents/' + encodeURIComponent(data.agent_name) + '/logs', {
              target: '#agent-log-content', swap: 'innerHTML'
            });
          }

        } else if (type === 'config.changed') {
          var modelEl = document.getElementById('current-model');
          if (modelEl && data.model) modelEl.textContent = data.model;
          var permEl = document.getElementById('current-permission-mode');
          if (permEl && data.permission_mode) permEl.textContent = data.permission_mode;

        } else if (type === 'agent.changed') {
          var agentEl = document.getElementById('current-agent');
          if (agentEl) agentEl.textContent = data.agent_type || 'default';

        } else if (type === 'team.changed') {
          var teamEl = document.getElementById('current-team');
          if (teamEl) teamEl.textContent = data.team_template || 'none';

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
          if (window.refreshSessionList) window.refreshSessionList();
        }
      } catch (err) {
        console.error('[claudio-ws] message parse error:', err, e.data);
      }
    };

    ws.onerror = function () {
      isConnecting = false;
      // Handled by onclose.
    };

    ws.onclose = function () {
      isConnecting = false;
      wsConnected = false;
      window._wsConnected = false;
      showBanner();
      if (reconnectTimer) clearTimeout(reconnectTimer);
      // Don't reconnect if a newer startChat call has taken over.
      if (_myGen !== _gen) return;
      var delay = Math.min(1000 * Math.pow(2, reconnectAttempt), 60000);
      reconnectAttempt++;
      reconnectTimer = setTimeout(initWS, delay);
    };
  }

  // Register current-session initWS so once-attached listeners call the right fn.
  _initWSFn = initWS;

  // htmx:wsClose / htmx:wsOpen — for htmx WS extension compatibility
  document.addEventListener('htmx:wsClose', function() { showBanner(); });
  document.addEventListener('htmx:wsOpen',  function() { hideBanner(); });

  // Attach reconnect listeners only once — each startChat() call must not pile up duplicates.
  if (!window._wsListenersAttached) {
    window._wsListenersAttached = true;

    // On app resume (phone unlock / tab focus), reconnect immediately.
    document.addEventListener('visibilitychange', function() {
      if (document.visibilityState === 'visible') {
        // On mobile PWA resume, close info panel so user sees chat, not stale panel.
        if (window.innerWidth < 1280 && typeof ccCloseInfoPanel === 'function') {
          ccCloseInfoPanel();
        }
        if (!wsConnected && !isConnecting) {
          if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
          reconnectAttempt = 0;
          if (_initWSFn) _initWSFn();
        }
      }
    });

    // iOS PWA bfcache restore — close info panel on mobile so user sees chat.
    window.addEventListener('pageshow', function(e) {
      if (e.persisted && window.innerWidth < 1280 && typeof ccCloseInfoPanel === 'function') {
        ccCloseInfoPanel();
      }
    });

    // Reconnect immediately when network comes back online.
    window.addEventListener('online', function() {
      if (!wsConnected && !isConnecting) {
        if (reconnectTimer) { clearTimeout(reconnectTimer); reconnectTimer = null; }
        reconnectAttempt = 0;
        if (_initWSFn) _initWSFn();
      }
    });
  }

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
    if (!main || !target || target !== main) return;
    var el = document.getElementById('chat-app');
    if (el && el.dataset.sessionId) startChat(el.dataset.sessionId);
  });
})();

// --- Session list refresh (debounced, WS-gated) ---
var _sessionRefreshTimer = null;
window.refreshSessionList = function() {
  if (_sessionRefreshTimer) return; // already queued
  _sessionRefreshTimer = setTimeout(function() {
    _sessionRefreshTimer = null;
    var el = document.getElementById('session-list');
    if (el && window.htmx) htmx.trigger(el, 'refresh');
  }, 2000);
};

// --- Agent card click (delegated) ---
document.addEventListener('click', function(e) {
  var card = e.target.closest('[data-agent]');
  if (!card) return;
  var name = card.dataset.agent;
  var sid = card.dataset.session;
  if (name && sid && window.ccOpenAgentLogs) {
    window.ccOpenAgentLogs(name, sid);
  }
});

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
        fetch('/api/push/vapid-public-key', { credentials: 'include' }).then(function(r) { return r.json(); })
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
        var headers = { 'Content-Type': 'application/json' };
        return fetch('/api/push/subscribe', {
          method: 'POST',
          credentials: 'include',
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

// iOS keyboard height detection — reposition #plus-popover above keyboard
if (window.visualViewport) {
  window.visualViewport.addEventListener('resize', function () {
    var keyboardHeight = Math.max(0, window.innerHeight - window.visualViewport.height);
    var popover = document.getElementById('plus-popover');
    if (popover) {
      if (keyboardHeight > 100) {
        popover.style.bottom = (keyboardHeight + 68) + 'px';
      } else {
        popover.style.bottom = '68px';
      }
    }
  });
}

// Scroll message input into view on focus (iOS keyboard overlap fix)
document.addEventListener('focusin', function (e) {
  if (e.target && e.target.id === 'msg-input') {
    setTimeout(function () {
      e.target.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }, 300);
  }
});

// --- Message delete (WhatsApp-style) ---
(function() {
  var sheet = document.getElementById('msg-action-sheet');
  var overlay = document.getElementById('msg-action-overlay');
  var longPressTimer = null;
  var LONG_PRESS_MS = 500;

  function getRow(el) {
    return el ? el.closest('[data-msg-id]') : null;
  }

  function getSessionId() {
    var el = document.getElementById('chat-app');
    return el ? el.dataset.sessionId : null;
  }

  function showSheet(msgId) {
    if (!sheet || !overlay) return;
    sheet.dataset.msgId = msgId;
    var msgEl = document.querySelector('[data-msg-id="' + msgId + '"]');
    var textEl = msgEl ? msgEl.querySelector('.md-content, .msg-bubble-assistant, .msg-bubble-user') : null;
    window._sheetMsgText = textEl ? textEl.innerText : '';
    overlay.classList.remove('hidden');
    sheet.classList.remove('translate-y-full');
    document.body.style.overflow = 'hidden';
  }

  function hideSheet() {
    if (!sheet || !overlay) return;
    sheet.classList.add('translate-y-full');
    setTimeout(function() {
      overlay.classList.add('hidden');
      document.body.style.overflow = '';
    }, 200);
  }

  function deleteMsg(msgId) {
    var sid = getSessionId();
    if (!sid || !msgId) return;
    var csrfMeta = document.querySelector('meta[name="csrf-token"]');
    var headers = {};
    if (csrfMeta) headers['X-CSRF-Token'] = csrfMeta.content;
    fetch('/api/sessions/' + sid + '/messages/' + msgId, {
      method: 'DELETE',
      credentials: 'include',
      headers: headers
    });
  }

  // Touch: long-press opens action sheet
  document.addEventListener('touchstart', function(e) {
    var row = getRow(e.target);
    if (!row) return;
    longPressTimer = setTimeout(function() {
      longPressTimer = null;
      showSheet(row.dataset.msgId);
    }, LONG_PRESS_MS);
  }, { passive: true });

  document.addEventListener('touchend', function() {
    if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null; }
  });
  document.addEventListener('touchmove', function() {
    if (longPressTimer) { clearTimeout(longPressTimer); longPressTimer = null; }
  });

  // Pointer: hover shows delete button, click triggers delete
  document.addEventListener('mouseover', function(e) {
    var row = getRow(e.target);
    if (!row) return;
    var btn = row.querySelector('.msg-delete-btn');
    if (btn) btn.classList.remove('opacity-0', 'pointer-events-none');
  });
  document.addEventListener('mouseout', function(e) {
    var row = getRow(e.target);
    if (!row) return;
    var btn = row.querySelector('.msg-delete-btn');
    if (btn) btn.classList.add('opacity-0', 'pointer-events-none');
  });

  // Desktop hover-button click
  document.addEventListener('click', function(e) {
    var btn = e.target.closest('.msg-delete-btn');
    if (!btn) return;
    var row = getRow(btn);
    if (!row) return;
    deleteMsg(row.dataset.msgId);
  });

  // Action sheet buttons
  if (overlay) overlay.addEventListener('click', hideSheet);
  window.ccDeleteSheetMsg = function() {
    var msgId = sheet ? sheet.dataset.msgId : null;
    hideSheet();
    if (msgId) deleteMsg(msgId);
  };
  window.ccCopySheetMsg = function() {
    hideSheet();
    if (navigator.clipboard && window._sheetMsgText) {
      navigator.clipboard.writeText(window._sheetMsgText).catch(function() {});
    }
  };
  window.ccCancelSheet = hideSheet;
})();
