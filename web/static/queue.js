// Warteschlange: Drucken (QR-Seite), Vollbild und wach bleiben (Anzeige im Laden).
// Eigene Datei, weil die CSP inline-Skripte blockiert.
document.addEventListener('click', (e) => {
  const b = e.target.closest('[data-queue]');
  if (!b) return;
  if (b.dataset.queue === 'print') window.print();
  if (b.dataset.queue === 'fullscreen' && document.documentElement.requestFullscreen) {
    document.documentElement.requestFullscreen().catch(() => {});
  }
});

// Bildschirm der Anzeige nicht ausgehen lassen, wo der Browser das kann.
if (document.body.classList.contains('queue-tv') && navigator.wakeLock) {
  const lock = () => navigator.wakeLock.request('screen').catch(() => {});
  lock();
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') lock();
  });
}
