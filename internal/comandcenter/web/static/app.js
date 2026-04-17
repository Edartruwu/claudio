(function () {
  var sessionId = document.body.dataset.sessionId;
  if (!sessionId) return;

  var msgs = document.getElementById('messages');
  var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
  var ws = new WebSocket(proto + '//' + location.host + '/ws/ui?session_id=' + sessionId);

  ws.onmessage = function (e) {
    try {
      var data = JSON.parse(e.data);
      if (data.type === 'new_message' && msgs) {
        msgs.insertAdjacentHTML('beforeend', data.html);
        msgs.scrollTop = msgs.scrollHeight;
      }
    } catch (_) {}
  };

  ws.onerror = function () {};
  ws.onclose = function () {};
})();
