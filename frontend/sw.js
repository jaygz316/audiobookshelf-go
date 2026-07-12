const CACHE_NAME = 'audiobookshelf-v3';
const ASSETS = [
  '/',
  '/index.html',
  '/css/index.css',
  '/js/app.js',
  '/js/api.js',
  '/js/auth.js',
  '/js/dashboard.js',
  '/js/library.js',
  '/assets/favicon.ico',
  '/assets/images/logo.png',
  '/assets/images/icon.svg'
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(ASSETS).catch(err => {
        console.warn('Failed to pre-cache some assets during service worker install:', err);
      });
    })
  );
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(
        keys.map((key) => {
          if (key !== CACHE_NAME) {
            return caches.delete(key);
          }
        })
      );
    })
  );
  self.clients.claim();
});

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  // Skip dynamic API, HLS streaming, feed, and auth endpoints
  if (
    url.pathname.startsWith('/api/') ||
    url.pathname.startsWith('/auth/') ||
    url.pathname.startsWith('/hls/') ||
    url.pathname.startsWith('/feed/') ||
    url.pathname.startsWith('/opds/') ||
    url.pathname.endsWith('/status') ||
    url.pathname.endsWith('/login') ||
    url.pathname.endsWith('/logout') ||
    url.pathname.endsWith('/init') ||
    event.request.method !== 'GET'
  ) {
    return;
  }

  event.respondWith(
    caches.match(event.request).then((cachedResponse) => {
      if (cachedResponse) {
        return cachedResponse;
      }
      return fetch(event.request).then((networkResponse) => {
        if (
          networkResponse &&
          networkResponse.status === 200 &&
          networkResponse.type === 'basic'
        ) {
          const responseToCache = networkResponse.clone();
          caches.open(CACHE_NAME).then((cache) => {
            cache.put(event.request, responseToCache);
          });
        }
        return networkResponse;
      }).catch(() => {
        // Simple silent fetch error fallback
      });
    })
  );
});
