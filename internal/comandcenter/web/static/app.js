(function () {
  var sessionId = document.body.dataset.sessionId;
  if (!sessionId) return;

  var msgs = document.getElementById('messages');
  var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  var reconnectTimer = null;
  var lastMsgDate = null;

  // --- Helpers ---

  function showBanner() {
    var b = document.getElementById('ws-banner');
    if (b) b.classList.remove('hidden');
  }

  function hideBanner() {
    var b = document.getElementById('ws-banner');
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
    // Move typing bubble to end of messages so it's always last.
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

  function appendMessage(html) {
    if (!msgs) return;
    maybeInsertDateDivider();
    // Insert before the typing bubble so it stays at the bottom.
    var bubble = document.getElementById('typing-bubble');
    if (bubble && !bubble.classList.contains('hidden')) {
      bubble.insertAdjacentHTML('beforebegin', html);
    } else {
      msgs.insertAdjacentHTML('beforeend', html);
    }
    msgs.scrollTop = msgs.scrollHeight;
  }

  // Insert initial date divider if messages exist on page load.
  (function () {
    if (msgs && msgs.children.length > 0) {
      // Find or create a sentinel before first message.
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
        // Re-append typing bubble (it lives outside the messages list).
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
      // Reload messages to catch anything missed while disconnected.
      if (wasDisconnected) reloadMessages();
    };

    ws.onmessage = function (e) {
      try {
        var data = JSON.parse(e.data);
        var type = data.type;

        if (type === 'message.assistant') {
          // Remove typing bubble, then append assistant response.
          removeTypingBubble();
          setTyping('● online');
          if (data.html) appendMessage(data.html);

        } else if (type === 'typing') {
          // Show transient typing indicator bubble + update header.
          showTypingBubble(data.tool, data.agentName);
          setTyping((data.agentName || 'Agent') + ' is working...');

        } else if (type === 'message.user') {
          // User's own message pushed back from server.
          if (data.html) appendMessage(data.html);

        } else if (type === 'message.tool_use') {
          // Permanent tool-use bubble in chat history.
          if (data.html) appendMessage(data.html);

        } else if (type === 'new_message' && data.html) {
          // Backward-compat path (legacy event type).
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
