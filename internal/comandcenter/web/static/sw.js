// ComandCenter Service Worker — handles push notifications.
'use strict';

self.addEventListener('push', function (e) {
  var data = {};
  try {
    data = e.data ? e.data.json() : {};
  } catch (_) {
    data = { title: 'ComandCenter', body: e.data ? e.data.text() : '' };
  }

  var title   = data.title  || 'ComandCenter';
  var options = {
    body   : data.body  || '',
    icon   : '/static/icon-192.png',
    badge  : '/static/icon-192.png',
    data   : { url: data.url || '/' },
    silent : false,
    vibrate: [200, 100, 200],
  };

  e.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', function (e) {
  e.notification.close();
  var url = (e.notification.data && e.notification.data.url) ? e.notification.data.url : '/';
  e.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true }).then(function (list) {
      for (var i = 0; i < list.length; i++) {
        if (list[i].url.indexOf(url) !== -1 && 'focus' in list[i]) {
          return list[i].focus();
        }
      }
      if (clients.openWindow) return clients.openWindow(url);
    })
  );
});
