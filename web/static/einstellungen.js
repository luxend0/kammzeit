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

// Vorschau der Buchungsseite (#einst-mini): Akzentfarbe, Design, Begrüßungstext und
// gewählte Bilder. Bilder als data:-URL (die CSP erlaubt kein blob:).
const mini = document.getElementById("einst-mini");
if (mini) {
  document.getElementById("einst-accent")?.addEventListener("input", (e) => {
    mini.style.setProperty("--accent", e.target.value);
    mini.style.setProperty("--on-accent", onAccent(e.target.value));
  });
  const theme = () => {
    const val = (name) => document.querySelector(`input[name=${name}]:checked`)?.value;
    mini.classList.remove("font-modern", "font-klassisch", "round", "dark");
    if (!mini.dataset.pro) return; // ohne Pro gilt das Standard-Design
    const font = val("theme_font");
    if (font && font !== "schlicht") mini.classList.add("font-" + font);
    if (val("theme_radius") === "round") mini.classList.add("round");
    if (val("theme_mode") === "dark") mini.classList.add("dark");
  };
  document.querySelectorAll("input[data-theme]").forEach((el) => el.addEventListener("change", theme));
  const welcome = document.getElementById("einst-welcome");
  welcome?.addEventListener("input", () => {
    if (mini.dataset.pro) document.getElementById("einst-mini-welcome").textContent = welcome.value.trim();
  });
}
document.querySelectorAll("input[type=file][data-prev]").forEach((input) => {
  input.addEventListener("change", () => {
    const file = input.files && input.files[0];
    if (!file || !/^image\/(png|jpeg|webp)$/.test(file.type)) return;
    const reader = new FileReader();
    reader.addEventListener("load", () => {
      for (const id of [input.dataset.prev, input.dataset.mini]) {
        const img = document.getElementById(id);
        if (!img || (id === "einst-mini-cover" && !mini?.dataset.pro)) continue;
        img.src = reader.result;
        img.hidden = false;
      }
      document.getElementById(input.dataset.prev + "-none")?.remove();
    });
    reader.readAsDataURL(file);
  });
});
