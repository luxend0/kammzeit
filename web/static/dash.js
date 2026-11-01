// Dashboard-Layout: Auf schmalen Bildschirmen ist das Hauptmenü eine waagerecht
// scrollbare Zeile. Den aktiven Punkt ins Bild holen, sonst steht z. B. "Abo" rechts außerhalb.
(() => {
  const a = document.querySelector('.topnav [aria-current="page"]');
  const nav = a && a.parentElement;
  if (nav && nav.scrollWidth > nav.clientWidth) nav.scrollLeft = a.offsetLeft - 16;
})();
