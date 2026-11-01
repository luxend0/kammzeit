// Kammzeit Service Worker. Quelle für /sw.js (Root-Scope): pwa.go setzt die
// Version ein (Hash über die eingebetteten Web-Dateien). Neue Version = neuer
// Cache, "activate" löscht die alten. Nicht unter /static/app/ registrieren.
//
// - /static/...: cache-first (Dateien ändern sich nur mit einer neuen Version).
// - Seiten: immer aus dem Netz und nie gespeichert. Dashboard-Seiten enthalten
//   Kundendaten, Login- und Session-Antworten erst recht; die gehören nicht in
//   den Gerätespeicher (Tresen-PC, geteilte Tablets). Ohne Netz: Offline-Seite.
// - POST, htmx-Anfragen und fremde Hosts: nicht anfassen.
const CACHE = 'kammzeit-__VERSION__';
const OFFLINE = '/offline';
// Alles, was die Offline-Seite braucht.
const PRECACHE = [OFFLINE, '/static/dash.css', '/static/fonts/display.woff2', '/static/img/kammzeit.svg',
  '/static/img/ill/kammi-schlaeft.webp', '/static/app/register.js'];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(PRECACHE)).then(() => self.skipWaiting()));
});

self.addEventListener('activate', (e) => {
  e.waitUntil(caches.keys()
    .then((keys) => Promise.all(keys.filter((k) => k.startsWith('kammzeit-') && k !== CACHE).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});

self.addEventListener('fetch', (e) => {
  const req = e.request;
  const url = new URL(req.url);
  if (req.method !== 'GET' || req.headers.has('HX-Request') || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/static/')) {
    e.respondWith(caches.open(CACHE).then((c) => c.match(req).then((hit) => hit || fetch(req).then((res) => {
      if (res.status === 200 && res.type === 'basic') c.put(req, res.clone());
      return res;
    }))));
    return;
  }
  if (req.mode === 'navigate') {
    e.respondWith(fetch(req).catch(() => caches.match(OFFLINE).then((res) => res || Response.error())));
  }
});
