<div align="center">

<img src="docs/readme/kammzeit.png" width="400" alt="Kammzeit">

**Online-Terminbuchung für Friseure, Barbershops und Beauty-Salons.**

[![Lizenz: AGPL-3.0](https://img.shields.io/badge/Lizenz-AGPL--3.0-1E6BFF)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-13294B?logo=go&logoColor=white)](go.mod)

[kammzeit.de](https://kammzeit.de) · [kammzeit.com](https://kammzeit.com) · [Demo](https://demo.kammzeit.de) · [Demo (English)](https://demo-en.kammzeit.com) · [Selbst hosten](docs/selbsthosten.md) · [In English](#in-english)

<img src="docs/readme/banner.png" width="100%" alt="Illustration: Eine Friseurin schneidet einer Kundin im Salon die Haare, vorne zeigt Igel Kammi auf dem Handy eine bestätigte Buchung">

</div>

## Worum es geht

<img src="docs/readme/kammi-winkt.png" align="right" width="146" alt="Kammi, das Maskottchen von Kammzeit: ein Igel, dessen Stacheln an Kammzähne erinnern, winkt">

Jeder Salon bekommt eine eigene Buchungsseite. Kunden wählen dort Leistung, Mitarbeiter und Uhrzeit und buchen rund um die Uhr selbst, ohne Konto und ohne App. Bestätigung und Erinnerung kommen per Mail.

Der Salon sieht alle Termine im Kalender, führt seine Kundenkartei und pflegt Leistungen, Team und Arbeitszeiten im Dashboard. Angemeldet wird per Einmal-Link, ein Passwort gibt es nicht. Mit Pro kommen eine Warteschlange für Laufkundschaft und eigene Logins für Mitarbeiter dazu.

Gehostet auf [kammzeit.de](https://kammzeit.de) (Deutsch) und [kammzeit.com](https://kammzeit.com) (Englisch) oder selbst betrieben: eine einzige Go-Binary, SQLite als Datenbank, keine weiteren Dienste außer einem Mailserver.

## So sieht es aus

<table>
  <tr>
    <td colspan="2" align="center"><img src="docs/readme/kalender.png" width="764" alt="Kalender im Dashboard: Tagesansicht mit drei Mitarbeitern, jeder in eigener Farbe"><br><sub><b>Kalender</b> · Tag oder Woche, eine Farbe je Mitarbeiter</sub></td>
  </tr>
  <tr>
    <td align="center" valign="top"><img src="docs/readme/buchungsseite.png" width="230" alt="Buchungsseite eines Salons am Handy: Salon-Infos und Auswahl der Leistung"><br><sub><b>Buchungsseite</b> am Handy</sub></td>
    <td align="center" valign="top"><img src="docs/readme/kundenkarte.png" width="508" alt="Kundenkarte mit Notizen, Kontakt, Besuchen und kommenden Terminen"><br><sub><b>Kundenkarte</b> · Notizen und alle Termine</sub></td>
  </tr>
</table>

Buchungsseite zum Ausprobieren: [demo.kammzeit.de](https://demo.kammzeit.de) auf Deutsch, [demo-en.kammzeit.com](https://demo-en.kammzeit.com) auf Englisch.

## Funktionen

<table>
  <tr>
    <td width="33%" valign="top"><img src="docs/readme/icons/buchung.svg" width="56" height="56" alt=""><br><b>Eigene Buchungsseite</b><br>Unter <code>&lt;salon&gt;.kammzeit.de</code> oder der eigenen Domain, in der Akzentfarbe des Salons. Kunden buchen ohne Konto.</td>
    <td width="33%" valign="top"><img src="docs/readme/icons/erinnerung.svg" width="56" height="56" alt=""><br><b>Bestätigung und Erinnerung</b><br>Mail mit Kalendereintrag und Link zum Absagen, Erinnerung 24 Stunden vor dem Termin.</td>
    <td width="33%" valign="top"><img src="docs/readme/icons/kalender.svg" width="56" height="56" alt=""><br><b>Kalender</b><br>Tages- und Wochenansicht. Termine eintragen, verschieben, absagen oder als nicht erschienen markieren.</td>
  </tr>
  <tr>
    <td valign="top"><img src="docs/readme/icons/kartei.svg" width="56" height="56" alt=""><br><b>Kundenkartei</b><br>Kontakt, Notizen wie die Farbrezeptur und alle kommenden und vergangenen Termine auf einer Karte.</td>
    <td valign="top"><img src="docs/readme/icons/schere.svg" width="56" height="56" alt=""><br><b>Leistungen</b><br>Mit Dauer und Preis. Je Leistung festlegen, wer sie anbietet.</td>
    <td valign="top"><img src="docs/readme/icons/team.svg" width="56" height="56" alt=""><br><b>Team</b><br>Arbeitszeiten je Mitarbeiter und Wochentag, auch mit Mittagspause.</td>
  </tr>
  <tr>
    <td valign="top"><img src="docs/readme/icons/warteschlange.svg" width="56" height="56" alt=""><br><b>Warteschlange (Pro)</b><br>Für Laufkundschaft: QR-Code im Laden scannen, Vornamen eintragen und live sehen, wann man dran ist. Auf Wunsch Mail kurz vorher, dazu eine Anzeige für den Bildschirm im Laden. Der Salon schaltet sie an und aus.</td>
    <td valign="top"><img src="docs/readme/icons/mitarbeiter.svg" width="56" height="56" alt=""><br><b>Mitarbeiter-Logins (Pro)</b><br>Eigener Zugang per Einladung. Mitarbeiter sehen Kalender, Kunden und Notizen und pflegen ihre eigenen Arbeitszeiten. Abo, Einstellungen, Leistungen und Export bleiben beim Inhaber.</td>
    <td valign="top"><img src="docs/readme/icons/anpassung.svg" width="56" height="56" alt=""><br><b>Salon-Anpassung</b><br>Logo, Links zu Instagram, TikTok, Facebook, Google und WhatsApp, Box mit den Öffnungszeiten. Mit Pro dazu Titelbild, Schrift, Ecken, hell oder dunkel, Begrüßungstext und eine Galerie mit bis zu 12 Fotos.</td>
  </tr>
  <tr>
    <td valign="top"><img src="docs/readme/icons/uhr.svg" width="56" height="56" alt=""><br><b>Buchungsregeln</b><br>Mindestvorlauf, Absagefrist und wie weit im Voraus gebucht werden kann.</td>
    <td valign="top"><img src="docs/readme/icons/schloss.svg" width="56" height="56" alt=""><br><b>Login ohne Passwort</b><br>Einmal-Link per Mail. Kein Passwort, das verloren gehen kann.</td>
    <td valign="top"><img src="docs/readme/icons/app.svg" width="56" height="56" alt=""><br><b>Als App installierbar</b><br>Das Dashboard lässt sich auf Handy und PC installieren, mit Kurzbefehlen und Offline-Seite. Kundendaten bleiben auf dem Server, nicht im Gerätespeicher.</td>
  </tr>
  <tr>
    <td valign="top"><img src="docs/readme/icons/tour.svg" width="56" height="56" alt=""><br><b>Geführte Tour</b><br>Beim ersten Login zeigt eine kurze Tour alle Bereiche des Dashboards. Neu starten über das Fragezeichen oben oder die Hilfe.</td>
    <td valign="top"><img src="docs/readme/icons/eu.svg" width="56" height="56" alt=""><br><b>Deutsch und Englisch</b><br>kammzeit.de auf Deutsch, kammzeit.com auf Englisch, Buchungsseite und Mails in der Sprache des Salons. Zeitzonen aller EU-Länder, gehostet in Eygelshoven (NL).</td>
    <td valign="top"><img src="docs/readme/icons/news.svg" width="56" height="56" alt=""><br><b>Kammzeit-News</b><br>Neuigkeiten für Salon-Inhaber per Mail, nur nach Bestätigung (Double-Opt-in) und jederzeit mit einem Klick abbestellbar.</td>
  </tr>
  <tr>
    <td valign="top"><img src="docs/readme/icons/export.svg" width="56" height="56" alt=""><br><b>Eigene Daten</b><br>Export aller Salondaten als JSON, Konto jederzeit selbst löschen.</td>
    <td valign="top"><img src="docs/readme/icons/offen.svg" width="56" height="56" alt=""><br><b>Open Source</b><br>AGPL-3.0. Selbst gehostet im Einzelbetrieb ohne Plan-Grenzen: beliebig viele Mitarbeiter, Warteschlange, Mitarbeiter-Logins, Titelbild, Design und Galerie, ohne Kammzeit-Hinweis.</td>
    <td valign="top"><img src="docs/readme/icons/gratis.svg" width="56" height="56" alt=""><br><b>Gratis starten</b><br>Gehostet gratis mit einem Mitarbeiter. Pro mit beliebig vielen Mitarbeitern, Mitarbeiter-Logins, Warteschlange und mehr Gestaltung: 14,90 € im Monat oder 149 € im Jahr.</td>
  </tr>
</table>

**Geplant** (ohne festen Termin, Details auf [kammzeit.de/geplant](https://kammzeit.de/geplant)): eigene Domain für die Buchungsseite, Abgleich mit Google Kalender, Urlaub und Abwesenheiten je Mitarbeiter, weitere Sprachen, Kunden per CSV übernehmen. Wünsche gern als Issue.

## Schnellstart

Voraussetzung: Go in der Version aus [go.mod](go.mod) oder neuer.

```sh
go run . demo
SITE_URL=http://localhost:8080 PUBLIC_URL='http://{slug}.localhost:8080' go run .
```

`demo` legt zwei Beispiel-Salons mit Leistungen und zwei Mitarbeitern an: "demo" auf Deutsch (Konto `demo@example.com`) und "demo-en" auf Englisch (Konto `demo-en@example.com`). Die Datenbank landet in `kammzeit.db` im aktuellen Verzeichnis.

| Was | Adresse |
|---|---|
| Startseite | <http://localhost:8080> |
| Buchungsseite des Demo-Salons | <http://demo.localhost:8080> |
| Englische Buchungsseite | <http://demo-en.localhost:8080> |
| Registrierung | <http://localhost:8080/registrieren> |
| Dashboard | <http://localhost:8080/login> mit `demo@example.com` |

Ohne `SMTP_HOST` werden Mails nur ins Log geschrieben, dort steht auch der Login-Link. `SITE_URL` und `PUBLIC_URL` sorgen nur dafür, dass Links in Mails lokal stimmen. Ohne sie läuft der Server genauso, die Links zeigen dann auf `https://localhost`.

## Konfiguration

Alles über Umgebungsvariablen. Für einen einzelnen Salon reichen `KAMMZEIT_SINGLE_TENANT=1`, `PUBLIC_URL` und die SMTP-Werte.

### Server und Domains

| Variable | Standard | Bedeutung |
|---|---|---|
| `PORT` | `8080` | Port, wenn `LISTEN` nicht gesetzt ist |
| `LISTEN` | `:<PORT>` | Adresse, auf der der Server lauscht. Hinter einem Reverse-Proxy: `127.0.0.1:8080` |
| `BASE_DOMAIN` | `localhost` | Hauptdomain. Salons laufen unter `<slug>.<BASE_DOMAIN>` |
| `BASE_DOMAINS` | leer | Mehrere Hauptdomains mit Sprache (`de` oder `en`), z. B. `kammzeit.de=de,kammzeit.com=en`. Die erste ist die Standarddomain und nutzt `SITE_URL` und `PUBLIC_URL`, jede weitere läuft unter `https://<domain>` und `https://<slug>.<domain>`. `BASE_DOMAIN` auf die erste setzen. Die Registrierung liegt auf englischen Domains unter `/signup`, auf deutschen unter `/registrieren`. Leer: nur `BASE_DOMAIN` auf Deutsch |
| `SITE_URL` | `https://<BASE_DOMAIN>`, im Einzelbetrieb wie `PUBLIC_URL` | Adresse von Startseite, Login und Dashboard, Basis für Login-Links |
| `PUBLIC_URL` | `https://{slug}.<BASE_DOMAIN>`, im Einzelbetrieb `http://localhost:<PORT>` | Adresse der Buchungsseiten, Basis für Links in Buchungsmails. `{slug}` wird durch den Salon ersetzt |
| `DB_PATH` | `kammzeit.db` | SQLite-Datei |
| `KAMMZEIT_SINGLE_TENANT` | leer | `1` = Einzelbetrieb: genau ein Salon, alles unter einer Domain, keine Subdomains |

### Mail

| Variable | Standard | Bedeutung |
|---|---|---|
| `SMTP_HOST` | leer | Mailserver. Leer: Mails werden nur ins Log geschrieben |
| `SMTP_PORT` | `587` | Port des Mailservers (STARTTLS, Port 465 wird nicht unterstützt) |
| `SMTP_USER` | leer | Benutzername am Mailserver, leer = ohne Anmeldung |
| `SMTP_PASS` | leer | Passwort am Mailserver |
| `SMTP_FROM` | `Kammzeit <noreply@localhost>` | Absender aller Mails |
| `MARKETING_SECRET` | leer | Geheimer Schlüssel (mindestens 16 Zeichen) für die Abmeldelinks der Kammzeit-News: der Link in jeder Marketing-Mail meldet ohne Anmeldung ab (HMAC über die Konto-ID). Leer: der Link führt in die Einstellungen, Abmelden nur angemeldet. Die Bestätigungslinks (Double-Opt-in) brauchen ihn nicht. Langen Zufallswert setzen, z. B. aus `openssl rand -hex 32`, und nicht mehr ändern (alte Abmeldelinks werden sonst ungültig) |

### Abo über Stripe (nur gehostete Version)

| Variable | Standard | Bedeutung |
|---|---|---|
| `STRIPE_SECRET_KEY` | leer | Geheimer API-Schlüssel von Stripe (`sk_...`) |
| `STRIPE_WEBHOOK_SECRET` | leer | Signing-Secret des Webhooks `/stripe/webhook` (`whsec_...`) |
| `STRIPE_PRICE_MONTHLY` | leer | Stripe-Preis-ID (`price_...`) für Pro monatlich |
| `STRIPE_PRICE_YEARLY` | leer | Stripe-Preis-ID (`price_...`) für Pro jährlich |

## Befehle

Die Binary liest dieselben Umgebungsvariablen, vor allem `DB_PATH`.

| Befehl | Wirkung |
|---|---|
| `kammzeit` | Startet den Server |
| `kammzeit demo` | Legt die Beispiel-Salons "demo" (Deutsch, Login `demo@example.com`) und "demo-en" (Englisch, Login `demo-en@example.com`) an, beide im Plan Pro. Mehrfach aufrufbar, legt nichts doppelt an |
| `kammzeit session <email>` | Legt eine Session für das Konto an und gibt `kz_session=...` aus. Zum Testen mit curl oder als Notzugang, wenn keine Mails ankommen |

## Selbst hosten

Anleitung mit systemd, Caddy, SMTP, Backups und Updates: **[docs/selbsthosten.md](docs/selbsthosten.md)** (auf Englisch: [docs/self-hosting.md](docs/self-hosting.md)).

Kurz mit Docker:

```sh
docker build -t kammzeit .
docker run -d --name kammzeit -p 127.0.0.1:8080:8080 -v kammzeit-data:/data \
  -e PUBLIC_URL=https://termine.example.de kammzeit
```

Das Image startet im Einzelbetrieb, die Datenbank liegt im Volume unter `/data/kammzeit.db`. Im Einzelbetrieb gibt es keine Plan-Grenzen: beliebig viele Mitarbeiter, Warteschlange, Mitarbeiter-Logins, Titelbild, Design und Galerie ohne Pro-Abo und ohne Kammzeit-Hinweis.

## Technik

- **Go** mit der Standardbibliothek: `net/http` (Routing mit Go-1.22-Mustern), `html/template`, `embed`. Templates und statische Dateien unter `web/` stecken in der Binary.
- **SQLite** über [modernc.org/sqlite](https://modernc.org/sqlite), reines Go, kein CGO. Schema in [schema.sql](schema.sql).
- **htmx 2** für die wenigen dynamischen Stellen, kein JS-Framework. Schriften und Skripte kommen vom eigenen Server, keine Tracker.
- Ein einziges Go-Paket, zwei direkte Abhängigkeiten in [go.mod](go.mod) (SQLite-Treiber, `golang.org/x/image` für Bild-Uploads).

## Entwicklung

```sh
go test ./...
go vet ./...
gofmt -l .   # muss leer bleiben
```

Änderungen an bestehenden Tabellen nur als neuer Eintrag in `migrations` in [model.go](model.go).

Mails ansehen: `KAMMZEIT_MAIL_DUMP=/tmp/mails go test -run TestMailsRender .` schreibt jede Mail auf Deutsch und Englisch als `.html` und `.txt` in den Ordner (der muss schon existieren).

Rauchtest gegen einen laufenden Server, Salon über den Host-Header:

```sh
BASE_DOMAIN=kammzeit.test go run .
curl -H 'Host: demo.kammzeit.test' http://localhost:8080/
curl -H 'Host: kammzeit.test' -H "Cookie: $(go run . session demo@example.com)" http://localhost:8080/dashboard
```

## Lizenz

[GNU AGPL-3.0](LICENSE) oder später. Wer Kammzeit verändert und als Netzdienst betreibt, muss den Nutzern den geänderten Quellcode zugänglich machen (§ 13 AGPL). Das gilt auch für eine selbst gehostete Installation mit eigenen Änderungen.

---

## In English

Kammzeit is open-source online booking for hairdressers, barbershops and beauty salons. Every salon gets its own booking page where customers choose a service, a staff member and a time, without creating an account. Confirmation and reminder arrive by email. The salon manages appointments, customer records, services and working hours in the dashboard and logs in with a one-time link, no password.

Use the hosted version at [kammzeit.com](https://kammzeit.com) (English, sign up at [kammzeit.com/signup](https://kammzeit.com/signup)) or [kammzeit.de](https://kammzeit.de) (German), try the demo booking pages [demo-en.kammzeit.com](https://demo-en.kammzeit.com) (English) and [demo.kammzeit.de](https://demo.kammzeit.de) (German), or run it yourself: a single Go binary with SQLite, nothing else but a mail server.

**Features**

- Booking page at `<salon>.kammzeit.com` in the salon's accent colour, booking without an account
- Confirmation with calendar entry and cancellation link, reminder 24 hours before the appointment
- Calendar (day and week), customer records with notes, services with duration and price, working hours with breaks
- Booking rules: minimum notice, cancellation deadline, booking window
- Walk-in queue (Pro): customers scan a QR code, enter their first name and see live when it is their turn, optional email shortly before, a display for a screen in the salon
- Staff logins (Pro): invited staff see the calendar, customers and notes and manage their own working hours; plan, settings, services and export stay with the owner
- Salon branding: logo, social links (Instagram, TikTok, Facebook, Google, WhatsApp) and an opening hours box; with Pro also a cover image, font, corners, light or dark theme, a welcome text and a gallery of up to 12 photos
- Installable app: add the dashboard to your phone or computer, with shortcuts and an offline page; no customer data is stored on the device
- Guided tour on first login, restart it from the question mark at the top or from the help pages
- English and German: kammzeit.com in English, kammzeit.de in German, booking page and emails in the salon's language, time zones of all EU countries, hosted in Eygelshoven (NL)
- Kammzeit news by email for salon owners: only after confirmation (double opt-in), unsubscribe with one click
- JSON export of all salon data, delete your account yourself
- Free with one staff member; Pro with unlimited staff, staff logins, the queue and more branding for €14.90 per month or €149 per year. Self-hosted in single-salon mode without staff limit, with queue and staff logins and without the Kammzeit notice

**Planned** (no fixed dates, see [kammzeit.com/roadmap](https://kammzeit.com/roadmap)): custom domain for the booking page, Google Calendar sync, holidays and time off per staff member, more languages, importing customers from CSV.

**Quick start** (Go version from [go.mod](go.mod) or newer):

```sh
go run . demo
SITE_URL=http://localhost:8080 PUBLIC_URL='http://{slug}.localhost:8080' go run .
```

`demo` creates two sample salons: "demo" in German (login `demo@example.com`) and "demo-en" in English (login `demo-en@example.com`). Booking pages: <http://demo.localhost:8080> and <http://demo-en.localhost:8080>. Dashboard: <http://localhost:8080/login>. Without `SMTP_HOST`, emails (including the login link) are only written to the log.

Configuration works through environment variables. The tables under [Konfiguration](#konfiguration) list all of them in German; the English self-hosting guide [docs/self-hosting.md](docs/self-hosting.md) describes them as well and covers systemd, Caddy, SMTP, backups, updates and running two languages like kammzeit.de and kammzeit.com (`BASE_DOMAINS`).

Licence: [GNU AGPL-3.0](LICENSE) or later. If you modify Kammzeit and run it as a network service, you must offer your users the modified source code.
