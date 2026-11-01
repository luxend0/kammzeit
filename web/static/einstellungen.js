// Einstellungen: Live-Vorschau der Akzentfarbe, Link der Buchungsseite kopieren und am Handy teilen.
// Texte (übersetzt) kommen aus data-copied und data-manual am Statusfeld.

// onAccent wie in handlers.go: Schwarz oder Weiß, je nachdem, was nach WCAG mehr Kontrast hat.
function onAccent(hex) {
  if (!/^#[0-9a-f]{6}$/i.test(hex)) return "#000";
  const lin = (i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  const l = 0.2126 * lin(1) + 0.7152 * lin(3) + 0.0722 * lin(5);
  return (l + 0.05) * (l + 0.05) > 1.05 * 0.05 ? "#000" : "#fff";
}

document.getElementById("einst-accent")?.addEventListener("input", (e) => {
  const box = document.getElementById("einst-preview-box");
  box.style.setProperty("--accent", e.target.value);
  box.style.setProperty("--on-accent", onAccent(e.target.value));
});

const url = document.getElementById("einst-url");
const copy = document.getElementById("einst-copy");
const status = document.getElementById("einst-copy-status");
url?.addEventListener("focus", () => url.select());
if (url && copy && status) {
  copy.hidden = false; // ohne JS bleibt der Knopf weg, die Adresse lässt sich dann von Hand kopieren
  copy.addEventListener("click", async () => {
    status.textContent = "";
    try {
      await navigator.clipboard.writeText(url.value);
      status.textContent = status.dataset.copied;
    } catch {
      url.focus(); // markiert die Adresse
      status.textContent = status.dataset.manual;
    }
    if (navigator.share && matchMedia("(pointer: coarse)").matches) {
      navigator.share({ url: url.value }).catch(() => {}); // abgebrochen: nichts zu tun
    }
  });
}
