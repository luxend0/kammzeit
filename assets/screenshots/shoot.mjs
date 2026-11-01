// Steuert Brave headless über das DevTools-Protokoll und speichert PNGs.
// Wird von shoot.sh aufgerufen (dort stehen Server, Daten und Umwandlung in WebP).
// Umgebung: CDP_PORT, PORT, COOKIE (kz_session=...), OUT, DAY (YYYY-MM-DD, "heute").
// Node >= 22 (fetch und WebSocket eingebaut), keine Abhängigkeiten.
import { writeFileSync } from "node:fs";

const { CDP_PORT, PORT, COOKIE, OUT, DAY } = process.env;
const site = `http://kammzeit.test:${PORT}`;
const salon = `http://lindner.kammzeit.test:${PORT}`;

const target = await (await fetch(`http://127.0.0.1:${CDP_PORT}/json/new?about:blank`, { method: "PUT" })).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((ok, fail) => { ws.onopen = ok; ws.onerror = fail; });

let seq = 0;
const pending = new Map();
const waiters = [];
ws.onmessage = (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.id && pending.has(msg.id)) {
    const { ok, fail } = pending.get(msg.id);
    pending.delete(msg.id);
    msg.error ? fail(new Error(JSON.stringify(msg.error))) : ok(msg.result);
  } else if (msg.method) {
    for (const w of waiters.filter((w) => w.method === msg.method)) {
      waiters.splice(waiters.indexOf(w), 1);
      w.ok(msg.params);
    }
  }
};
const send = (method, params = {}) => new Promise((ok, fail) => {
  const id = ++seq;
  pending.set(id, { ok, fail });
  ws.send(JSON.stringify({ id, method, params }));
});
const once = (method) => new Promise((ok) => waiters.push({ method, ok }));
const sleep = (ms) => new Promise((ok) => setTimeout(ok, ms));

async function js(expr) {
  const r = await send("Runtime.evaluate", { expression: expr, awaitPromise: true, returnByValue: true });
  if (r.exceptionDetails) throw new Error(`${expr}: ${r.exceptionDetails.exception?.description}`);
  return r.result.value;
}
async function until(expr, what) {
  for (let i = 0; i < 100; i++) {
    if (await js(expr)) return;
    await sleep(100);
  }
  throw new Error(`Zeitüberschreitung: ${what}`);
}
async function viewport(width) {
  await send("Emulation.setDeviceMetricsOverride", { width, height: 900, deviceScaleFactor: 2, mobile: width < 600 });
}
async function go(url) {
  const loaded = once("Page.loadEventFired");
  await send("Page.navigate", { url });
  await loaded;
  await js("document.fonts.ready.then(() => true)");
}
// edge liefert die Kante (top/bottom, Seitenkoordinaten) des letzten Treffers.
async function edge(selector, side) {
  return js(`(() => {
    const els = document.querySelectorAll(${JSON.stringify(selector)});
    if (!els.length) throw new Error(${JSON.stringify("kein Treffer für " + selector)});
    return Math.round(els[els.length - 1].getBoundingClientRect()[${JSON.stringify(side)}] + window.scrollY);
  })()`);
}

// shot speichert einen Ausschnitt der Seite als PNG. Ohne Angaben die ganze Seite;
// top/bottom = CSS-Selektoren (Oberkante minus 24 bzw. Unterkante plus `pad`),
// left/width = waagerechter Ausschnitt in CSS-Pixeln.
async function shot(name, { top, bottom, pad = 32, left = 0, width } = {}) {
  await js("window.scrollTo(0, 0), document.activeElement && document.activeElement.blur(), true");
  await sleep(150);
  const full = await js("Math.ceil(document.documentElement.scrollHeight)");
  width ??= await js("document.documentElement.clientWidth");
  const y = top ? Math.max(0, (await edge(top, "top")) - 24) : 0;
  const y2 = bottom ? Math.min(full, (await edge(bottom, "bottom")) + pad) : full;
  const r = await send("Page.captureScreenshot", {
    format: "png", captureBeyondViewport: true, clip: { x: left, y, width, height: y2 - y, scale: 1 },
  });
  writeFileSync(`${OUT}/${name}.png`, Buffer.from(r.data, "base64"));
  console.log(`${name}.png ${width}x${y2 - y} CSS-Pixel ab y=${y} (Seite ${full})`);
}

await send("Page.enable");
await send("Runtime.enable");
await send("Emulation.setLocaleOverride", { locale: "de-DE" });
const [name, ...rest] = COOKIE.split("=");
await send("Network.setCookie", { name, value: rest.join("="), domain: "kammzeit.test", path: "/", httpOnly: true });

// Dashboard, 1280 breit
await viewport(1280);
await go(`${site}/dashboard`);
await shot("kalender", { bottom: ".kal-scroll" });
await go(`${site}/dashboard/kunden?q=Neumann`);
const card = await js(`document.querySelector('#kunden-tabelle a[href^="/dashboard/kunden/"]').href`);
await go(card);
await shot("kunde", { bottom: "main .table-wrap" });
await go(`${site}/dashboard/team`);
await shot("team", { bottom: "main .table-wrap" });
await go(`${site}/dashboard/leistungen`);
await shot("leistungen", { bottom: "main .table-wrap" });
await go(`${site}/dashboard/einstellungen`);
const form = 'form[action="/dashboard/einstellungen"]';
await shot("einstellungen", { bottom: `${form} .einst-section:nth-of-type(2)` }); // bis "Aussehen"
// Zusatz: nur die Buchungsregeln, Spalte des Formulars
const col = await js(`(() => { const r = document.querySelector('${form}').getBoundingClientRect(); return [r.left, r.width]; })()`);
await shot("einstellungen-regeln", {
  top: `${form} .einst-section:nth-of-type(3)`, bottom: `${form} > .actions`, left: col[0] - 40, width: col[1] + 80,
});

// Mobil, 390 breit
await viewport(390);
await go(`${site}/dashboard`);
await shot("kalender-mobil", { bottom: ".kal-scroll" });

// Buchungsseite: übermorgen (Donnerstag), eine Uhrzeit gewählt, Daten eingetragen. Danach buchen.
const book = new Date(`${DAY}T12:00:00Z`);
book.setUTCDate(book.getUTCDate() + 2);
await go(`${salon}/`);
await until("!!document.querySelector('#slots .slots, #slots p')", "Uhrzeiten");
// Zusatz: Kopf der Buchungsseite mit den ersten Leistungen
await shot("buchung-start-mobil", { bottom: "form fieldset:first-of-type label:nth-of-type(6)", pad: 0 });
await js(`(() => {
  document.getElementById('slots').innerHTML = '';
  const d = document.getElementById('date');
  d.value = '${book.toISOString().slice(0, 10)}';
  d.dispatchEvent(new Event('change', { bubbles: true }));
  return true;
})()`);
await until("!!document.querySelector('#slots .slots')", "Uhrzeiten am Donnerstag");
const time = process.env.SLOT || "11:30";
await js(`(() => {
  const l = [...document.querySelectorAll('#slots .slot')].find((l) => l.textContent.trim() === '${time}');
  if (!l) throw new Error('Uhrzeit ${time} nicht frei, frei sind: ' +
    [...document.querySelectorAll('#slots .slot')].map((l) => l.textContent.trim()).join(' '));
  l.querySelector('input').click();
  const f = document.querySelector('form[action="/book"]');
  f.querySelector('[name=name]').value = 'Lena Hoffmann';
  f.querySelector('[name=email]').value = 'lena.hoffmann@example.com';
  return true;
})()`);
// Schritt 3 und 4: Tag, gewählte Uhrzeit, Daten, Buchen-Knopf (etwa ein Handy-Bildschirm)
await shot("buchung-mobil", { top: "form fieldset:nth-of-type(3)", bottom: 'form[action="/book"] > p' });
const booked = once("Page.loadEventFired");
await js(`document.querySelector('form[action="/book"]').requestSubmit(), true`);
await booked;
await js("document.fonts.ready.then(() => true)");
await shot("buchung-bestaetigt-mobil", { bottom: "footer" });

ws.close();
