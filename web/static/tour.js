// Geführte Tour durchs Dashboard. Schritte und übersetzte Texte stehen in web/tour.html (#kz-tour).
// Start: automatisch beim ersten Besuch je Konto und Browser (localStorage kz-tour-<userID>),
// über den Knopf [data-tour-start] in der Kopfzeile oder mit ?tour in der Adresse (Hilfe-Center).
// Modales <dialog>: Fokus bleibt in der Sprechblase, Seite dahinter inert, Esc beendet.
(() => {
  const box = document.getElementById("kz-tour");
  if (!box) return;
  const d = box.dataset;
  const all = [...box.querySelectorAll("template[data-tour-step]")];
  const narrow = matchMedia("(max-width: 35.99rem)"); // wie tour.css: Karte unten statt Sprechblase
  const calm = matchMedia("(prefers-reduced-motion: reduce)");
  let dlg, pop, spot, ill, count, body, dots, skip, back, next;
  let steps = [], i = 0, target = null, radius = 12, before = null, raf = 0, moving = 0;

  // Ohne localStorage (gesperrt) lieber nie automatisch starten als bei jedem Seitenaufruf.
  const seen = () => { try { return localStorage.getItem(d.key) !== null; } catch { return true; } };
  const markSeen = () => { try { localStorage.setItem(d.key, "done"); } catch {} };
  const find = (t) => {
    const el = t.dataset.target && document.querySelector(t.dataset.target);
    return el && el.getClientRects().length ? el : null;
  };
  const button = (cls, text) => {
    const b = document.createElement("button");
    b.type = "button";
    b.className = cls;
    b.textContent = text;
    return b;
  };

  function build() {
    dlg = document.createElement("dialog");
    dlg.className = "tour";
    dlg.setAttribute("aria-label", d.label);
    dlg.setAttribute("aria-describedby", "tour-live");
    dlg.innerHTML = '<div class="tour-spot"></div><div class="tour-pop"><img class="tour-ill" src="/static/img/ill/kammi-winkt.webp" width="96" height="96" alt="">' +
      '<div class="tour-live" id="tour-live" aria-live="polite" aria-atomic="true"><p class="vh"></p><div class="tour-body"></div></div>' +
      '<div class="tour-foot"><ol class="tour-dots" aria-hidden="true"></ol></div></div>';
    [spot, pop] = dlg.children;
    ill = pop.querySelector(".tour-ill");
    count = pop.querySelector(".vh");
    body = pop.querySelector(".tour-body");
    dots = pop.querySelector(".tour-dots");
    skip = button("link tour-skip", d.skip);
    back = button("btn btn-secondary btn-small", d.back);
    next = button("btn btn-small", d.next);
    pop.querySelector(".tour-foot").append(skip, back, next);
    skip.onclick = () => dlg.close();
    back.onclick = () => show(i - 1);
    next.onclick = () => (i === steps.length - 1 ? dlg.close() : show(i + 1));
    body.addEventListener("click", (e) => e.target.closest("a") && dlg.close()); // Link im letzten Schritt
    // Klicks neben die Knöpfe (Abdunklung, Text) nehmen den Fokus nicht aus der Sprechblase.
    dlg.addEventListener("mousedown", (e) => e.target.closest("button, a") || e.preventDefault());
    dlg.addEventListener("keydown", (e) => {
      if (e.key === "ArrowRight" && i < steps.length - 1) show(i + 1);
      else if (e.key === "ArrowLeft" && i > 0) show(i - 1);
      else return;
      e.preventDefault();
    });
    // Jedes Schließen (Fertig, Überspringen, Esc, Link) zählt als gesehen.
    dlg.addEventListener("close", () => {
      markSeen();
      removeEventListener("resize", queue);
      document.removeEventListener("scroll", queue, true);
      if (before && before.isConnected && before !== document.body) before.focus({ preventScroll: true });
    });
    document.body.append(dlg);
  }

  function start() {
    steps = all.filter((t) => !t.dataset.target || t.hasAttribute("data-soft") || find(t));
    if (!steps.length) return;
    if (!dlg) build();
    dots.replaceChildren(...steps.map(() => document.createElement("li")));
    before = document.activeElement;
    dlg.showModal();
    addEventListener("resize", queue);
    document.addEventListener("scroll", queue, true); // auch das waagerecht scrollende Menü am Handy
    show(0);
    next.focus(); // showModal fokussiert sonst „Überspringen“
  }

  function show(n) {
    i = n;
    const t = steps[i];
    target = find(t);
    if (target) radius = (parseFloat(getComputedStyle(target).borderTopLeftRadius) || 6) + 6;
    body.replaceChildren(t.content.cloneNode(true));
    ill.src = "/static/img/ill/" + (t.dataset.ill || "kammi-winkt") + ".webp";
    count.textContent = d.count.replace("%d", i + 1).replace("%d", steps.length);
    [...dots.children].forEach((li, k) => li.className = k === i ? "on" : k < i ? "done" : "");
    back.hidden = i === 0;
    skip.hidden = i === steps.length - 1;
    next.textContent = i === steps.length - 1 ? d.done : d.next;
    if (!pop.contains(document.activeElement) || document.activeElement.hidden) next.focus();
    dlg.classList.add("tour-move"); // weich gleiten nur beim Schrittwechsel, nicht beim Scrollen
    clearTimeout(moving);
    moving = setTimeout(() => dlg.classList.remove("tour-move"), 350);
    if (target) target.scrollIntoView({ block: "nearest", inline: "center", behavior: calm.matches ? "auto" : "smooth" });
    place();
  }

  function place() {
    raf = 0;
    if (!dlg || !dlg.open) return;
    const pad = 6, gap = 14, m = 12;
    const r = target && target.isConnected ? target.getBoundingClientRect() : null;
    const s = spot.style;
    if (r && r.width) {
      s.left = r.left - pad + "px"; s.top = r.top - pad + "px";
      s.width = r.width + 2 * pad + "px"; s.height = r.height + 2 * pad + "px";
      s.borderRadius = radius + "px";
    } else { // ohne Ziel: Loch der Größe 0 in der Mitte = alles abgedunkelt
      s.left = s.top = "50%"; s.width = s.height = "0";
    }
    spot.classList.toggle("tour-spot-none", !(r && r.width));
    pop.removeAttribute("data-side");
    if (narrow.matches) { pop.style.left = pop.style.top = ""; return; }
    const pw = pop.offsetWidth, ph = pop.offsetHeight;
    let top = (innerHeight - ph) / 2, left = (innerWidth - pw) / 2;
    if (r && r.width) {
      const below = r.bottom + pad + gap, above = r.top - pad - gap - ph;
      if (below + ph <= innerHeight - m) { top = below; pop.dataset.side = "below"; }
      else if (above >= m) { top = above; pop.dataset.side = "above"; }
      else top = innerHeight - ph - m; // Ziel zu groß: Sprechblase unten, ohne Spitze
      left = Math.min(Math.max(r.left + r.width / 2 - pw / 2, m), innerWidth - pw - m);
      pop.style.setProperty("--arrow", Math.min(Math.max(r.left + r.width / 2 - left, 28), pw - 28) + "px");
    }
    pop.style.left = Math.max(m, left) + "px";
    pop.style.top = Math.max(m, top) + "px";
  }
  const queue = () => { raf ||= requestAnimationFrame(place); };
  document.fonts?.ready.then(queue);

  document.addEventListener("click", (e) => {
    if (!e.target.closest("[data-tour-start]")) return;
    e.preventDefault(); // ohne JS führt der Knopf ins Hilfe-Center
    start();
  });
  const q = new URLSearchParams(location.search);
  if (q.has("tour")) {
    q.delete("tour");
    const rest = q.toString();
    history.replaceState(null, "", location.pathname + (rest ? "?" + rest : "") + location.hash);
    start();
  } else if (!seen()) start();
})();
