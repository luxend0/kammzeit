// Kammzeit als App: Service Worker registrieren und Installationshinweis
// (Karte "pwa-install-hint" aus web/pwa.html). Eigene Datei wegen CSP.
(() => {
  if ('serviceWorker' in navigator) navigator.serviceWorker.register('/sw.js', { scope: '/' }).catch(() => {});

  // Offline-Seite: sobald das Netz zurück ist, neu laden.
  if (document.body.hasAttribute('data-offline')) {
    addEventListener('online', () => location.reload());
    return;
  }

  const card = document.getElementById('pwa-hint');
  const KEY = 'kz-pwa-hint';
  const standalone = matchMedia('(display-mode: standalone)').matches || navigator.standalone === true;
  let closed = false;
  try { closed = localStorage.getItem(KEY) === '1'; } catch (e) { /* Speicher gesperrt: Hinweis zeigen */ }
  if (!card || standalone || closed) return;

  const part = (name) => card.querySelector('[data-pwa="' + name + '"]');
  const show = (...names) => { names.forEach((n) => { part(n).hidden = false; }); card.hidden = false; };
  const close = () => {
    card.hidden = true;
    try { localStorage.setItem(KEY, '1'); } catch (e) { /* dann eben nur für diese Seite */ }
  };
  part('close').addEventListener('click', close);
  addEventListener('appinstalled', close);

  // iPhone/iPad: kein Installationsdialog, nur der Weg über das Teilen-Menü. Nur in
  // Safari (Chrome, Firefox, Edge ... auf iOS melden sich mit eigenem Kürzel).
  const ua = navigator.userAgent;
  const ios = /iPhone|iPad|iPod/.test(ua) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
  if (ios) {
    if (/Safari/.test(ua) && !/CriOS|FxiOS|EdgiOS|OPiOS|GSA|DuckDuckGo/.test(ua)) show('ios');
    return;
  }

  // Android und Desktop (Chrome, Edge, Brave ...): eigener Knopf statt Mini-Leiste.
  let prompt = null;
  addEventListener('beforeinstallprompt', (e) => {
    e.preventDefault();
    prompt = e;
    show('prompt', 'install');
  });
  part('install').addEventListener('click', async () => {
    if (!prompt) return;
    prompt.prompt();
    await prompt.userChoice; // angenommen oder abgelehnt: nicht noch einmal fragen
    prompt = null;
    close();
  });
})();
