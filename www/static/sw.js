/* VirtBBS Web — minimal offline shell (static assets only). */
var CACHE = 'virtbbs-web-v1';
var ASSETS = ['/static/bootstrap.min.css', '/static/style.css', '/static/jquery.min.js', '/static/bootstrap.bundle.min.js', '/static/nav.js', '/static/notify.js', '/static/icon.svg'];

self.addEventListener('install', function (e) {
  e.waitUntil(
    caches.open(CACHE).then(function (cache) {
      return cache.addAll(ASSETS);
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', function (e) {
  e.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', function (e) {
  if (e.request.method !== 'GET') return;
  var url = new URL(e.request.url);
  if (url.pathname.indexOf('/static/') === 0) {
    e.respondWith(
      caches.match(e.request).then(function (cached) {
        return cached || fetch(e.request);
      })
    );
  }
});
