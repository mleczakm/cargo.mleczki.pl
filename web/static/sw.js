// Service worker for cargo.mleczki.pl. Served from /sw.js so that it controls the whole site.
// It does two things: shows web push notifications and, when the network is gone,
// replaces a failed page load with a short offline notice. Pages themselves are never
// cached: they hold carts, sessions and orders.

const VERSION = 'v1';
const OFFLINE_CACHE = 'offline-' + VERSION;
const OFFLINE_URL = '/static/offline.html';
const ICON = '/static/icons/icon-192.png';
const BADGE = '/static/icons/badge-96.png';

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(OFFLINE_CACHE)
      .then((cache) => cache.add(new Request(OFFLINE_URL, { cache: 'reload' })))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== OFFLINE_CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.mode !== 'navigate') return;
  event.respondWith(
    fetch(req).catch(() => caches.match(OFFLINE_URL).then((res) => res || Response.error()))
  );
});

self.addEventListener('push', (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch (e) {
    data = { body: event.data ? event.data.text() : '' };
  }
  const title = data.title || 'cargo.mleczki.pl';
  const options = {
    body: data.body || '',
    icon: ICON,
    badge: BADGE,
    tag: data.tag || undefined,
    renotify: Boolean(data.tag),
    data: { url: (data.data && data.data.url) || data.url || '/' },
  };
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const target = new URL(event.notification.data && event.notification.data.url || '/', self.location.origin);
  if (target.origin !== self.location.origin) return;

  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windows) => {
      for (const client of windows) {
        if (new URL(client.url).origin === target.origin && 'focus' in client) {
          return client.focus().then((focused) => (focused && 'navigate' in focused ? focused.navigate(target.href) : focused));
        }
      }
      return self.clients.openWindow(target.href);
    })
  );
});

// The browser rotated the subscription: register the new one so notifications keep coming.
self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(
    fetch('/api/webpush/vapid-key', { credentials: 'same-origin' })
      .then((res) => res.json())
      .then(({ publicKey }) => self.registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: publicKey,
      }))
      .then((sub) => fetch('/api/webpush/subscribe', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(sub),
      }))
      .catch(() => {})
  );
});
