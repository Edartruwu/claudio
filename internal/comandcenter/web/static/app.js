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

  function initWS() {
    var ws = new WebSocket(proto + '//' + location.host + '/ws/ui?session_id=' + sessionId);

    ws.onopen = function () {
      hideBanner();
    };

    ws.onmessage = function (e) {
      try {
        var data = JSON.parse(e.data);
        if (data.type === 'new_message' && msgs) {
          // Detect role from rendered HTML class names (server only sends html+type).
          var isToolUse = data.html && data.html.indexOf('msg-bubble-tool') !== -1;
          var isAssistant = data.html && data.html.indexOf('msg-bubble-assistant') !== -1;

          if (isToolUse) {
            setTyping('typing...');
          } else if (isAssistant) {
            setTyping('');
          }

          maybeInsertDateDivider();
          msgs.insertAdjacentHTML('beforeend', data.html);
          msgs.scrollTop = msgs.scrollHeight;
        }
      } catch (_) {}
    };

    ws.onerror = function () {
      // Handled by onclose.
    };

    ws.onclose = function () {
      showBanner();
      if (reconnectTimer) clearTimeout(reconnectTimer);
      reconnectTimer = setTimeout(initWS, 3000);
    };
  }

  initWS();
})();
