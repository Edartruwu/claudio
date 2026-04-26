// ComandCenter Service Worker — push notifications + offline support.
'use strict';

var CACHE_NAME = 'cc-static-v3';
var PRECACHE_URLS = [
  '/static/vendor/htmx.min.js',
  '/static/app.js',
  '/static/vendor/tailwind.min.css',
  '/manifest.json'
];

// Install: precache static assets
self.addEventListener('install', function (e) {
  e.waitUntil(
    caches.open(CACHE_NAME).then(function (cache) {
      return cache.addAll(PRECACHE_URLS);
    }).then(function () {
      return self.skipWaiting();
    })
  );
});

// Activate: clean old caches
self.addEventListener('activate', function (e) {
  e.waitUntil(
    caches.keys().then(function (names) {
      return Promise.all(
        names.filter(function (n) { return n !== CACHE_NAME; })
             .map(function (n) { return caches.delete(n); })
      );
    }).then(function () {
      return self.clients.claim();
    })
  );
});

// Fetch: cache-first for static assets, network-first for navigation
self.addEventListener('fetch', function (e) {
  var url = new URL(e.request.url);

  // Static assets: cache-first
  if (url.pathname.startsWith('/static/') || url.pathname === '/manifest.json') {
    e.respondWith(
      caches.match(e.request).then(function (cached) {
        return cached || fetch(e.request).then(function (res) {
          if (res.ok) {
            var clone = res.clone();
            caches.open(CACHE_NAME).then(function (cache) { cache.put(e.request, clone); });
          }
          return res;
        });
      })
    );
    return;
  }

  // Navigation: network-first, offline fallback
  if (e.request.mode === 'navigate') {
    e.respondWith(
      fetch(e.request).catch(function () {
        return caches.match('/static/offline.html');
      })
    );
    return;
  }
});

// Push notifications
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
