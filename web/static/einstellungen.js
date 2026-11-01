// Einstellungen: Live-Vorschau der Akzentfarbe, Adresse der Buchungsseite beim Anklicken markieren.
document.getElementById("einst-accent")?.addEventListener("input", (e) => {
  document.getElementById("einst-preview-box").style.setProperty("--accent", e.target.value);
});
document.getElementById("einst-url")?.addEventListener("focus", (e) => e.target.select());
