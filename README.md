# Kammzeit

Online-Terminbuchung für Friseure und Salons. Kunden wählen Leistung, Mitarbeiter und Uhrzeit, der Salon verwaltet Termine, Kunden und Team im Dashboard.

Gehostet auf [kammzeit.de](https://kammzeit.de) oder selbst betrieben auf dem eigenen Server. Eine einzige Go-Binary, SQLite als Datenbank, keine weiteren Dienste außer einem Mailserver.

## Funktionen

- Eigene Buchungsseite je Salon (`<salon>.kammzeit.de`, selbst gehostet unter der eigenen Domain)
- Bestätigung per Mail mit Link zum Absagen, Erinnerung 24 Stunden vor dem Termin
- Kalender mit allen Terminen
- Kundenkartei
- Leistungen mit Dauer und Preis, Team mit Arbeitszeiten und Zuordnung zu Leistungen
- Login per Magic-Link (Einmal-Link per Mail, kein Passwort)
- Pro-Abo über Stripe (nur für die gehostete Version relevant)

## Schnellstart

Voraussetzung: Go in der Version aus [go.mod](go.mod) oder neuer.

```sh
go run . demo
SITE_URL=http://localhost:8080 PUBLIC_URL='http://{slug}.localhost:8080' go run .
```

`demo` legt den Beispiel-Salon "demo" mit Leistungen, zwei Mitarbeitern und dem Konto `demo@example.com` an. Die Datenbank landet in `kammzeit.db` im aktuellen Verzeichnis.

- Startseite: <http://localhost:8080>
- Buchungsseite des Demo-Salons: <http://demo.localhost:8080>
- Dashboard: <http://localhost:8080/login> mit `demo@example.com`. Ohne `SMTP_HOST` werden Mails nur ins Log geschrieben, dort steht auch der Login-Link.

`SITE_URL` und `PUBLIC_URL` sorgen nur dafür, dass Links in Mails lokal stimmen. Ohne sie läuft der Server genauso, die Links zeigen dann auf `https://localhost`.

## Konfiguration

Alles über Umgebungsvariablen. Für einen einzelnen Salon reichen `KAMMZEIT_SINGLE_TENANT=1`, `PUBLIC_URL` und die SMTP-Werte.

| Variable | Standard | Bedeutung |
|---|---|---|
| `PORT` | `8080` | Port, wenn `LISTEN` nicht gesetzt ist |
| `LISTEN` | `:<PORT>` | Adresse, auf der der Server lauscht. Hinter einem Reverse-Proxy: `127.0.0.1:8080` |
| `BASE_DOMAIN` | `localhost` | Hauptdomain. Salons laufen unter `<slug>.<BASE_DOMAIN>` |
| `SITE_URL` | `https://<BASE_DOMAIN>`, im Einzelbetrieb wie `PUBLIC_URL` | Adresse von Startseite, Login und Dashboard, Basis für Login-Links |
| `PUBLIC_URL` | `https://{slug}.<BASE_DOMAIN>`, im Einzelbetrieb `http://localhost:<PORT>` | Adresse der Buchungsseiten, Basis für Links in Buchungsmails. `{slug}` wird durch den Salon ersetzt |
| `DB_PATH` | `kammzeit.db` | SQLite-Datei |
| `KAMMZEIT_SINGLE_TENANT` | leer | `1` = Einzelbetrieb: genau ein Salon, alles unter einer Domain, keine Subdomains |
| `SMTP_HOST` | leer | Mailserver. Leer: Mails werden nur ins Log geschrieben |
| `SMTP_PORT` | `587` | Port des Mailservers (STARTTLS, Port 465 wird nicht unterstützt) |
| `SMTP_USER` | leer | Benutzername am Mailserver, leer = ohne Anmeldung |
| `SMTP_PASS` | leer | Passwort am Mailserver |
| `SMTP_FROM` | `Kammzeit <noreply@localhost>` | Absender aller Mails |
| `STRIPE_SECRET_KEY` | leer | Geheimer API-Schlüssel von Stripe (`sk_...`), nur gehostete Version |
| `STRIPE_WEBHOOK_SECRET` | leer | Signing-Secret des Webhooks `/stripe/webhook` (`whsec_...`) |
| `STRIPE_PRICE_MONTHLY` | leer | Stripe-Preis-ID (`price_...`) für Pro monatlich |
| `STRIPE_PRICE_YEARLY` | leer | Stripe-Preis-ID (`price_...`) für Pro jährlich |

## Befehle

Die Binary liest dieselben Umgebungsvariablen, vor allem `DB_PATH`.

| Befehl | Wirkung |
|---|---|
| `kammzeit` | Startet den Server |
| `kammzeit demo` | Legt den Beispiel-Salon "demo" an (Login `demo@example.com`, Plan Pro). Mehrfach aufrufbar, legt nichts doppelt an |
| `kammzeit session <email>` | Legt eine Session für das Konto an und gibt `kz_session=...` aus. Zum Testen mit curl oder als Notzugang, wenn keine Mails ankommen |

## Selbst hosten

Anleitung mit systemd, Caddy, SMTP, Backups und Updates: [docs/selbsthosten.md](docs/selbsthosten.md).

Kurz mit Docker:

```sh
docker build -t kammzeit .
docker run -d --name kammzeit -p 127.0.0.1:8080:8080 -v kammzeit-data:/data \
  -e PUBLIC_URL=https://termine.example.de kammzeit
```

Das Image startet im Einzelbetrieb, die Datenbank liegt im Volume unter `/data/kammzeit.db`.

## Entwicklung

```sh
go test ./...
go vet ./...
gofmt -l .   # muss leer bleiben
```

Ein Go-Paket, Templates und statische Dateien unter `web/` (in die Binary eingebettet), Schema in [schema.sql](schema.sql). Änderungen an bestehenden Tabellen nur als neuer Eintrag in `migrations` in [model.go](model.go).

Rauchtest gegen einen laufenden Server, Salon über den Host-Header:

```sh
BASE_DOMAIN=kammzeit.test go run .
curl -H 'Host: demo.kammzeit.test' http://localhost:8080/
curl -H 'Host: kammzeit.test' -H "Cookie: $(go run . session demo@example.com)" http://localhost:8080/dashboard
```

## Lizenz

[GNU AGPL-3.0](LICENSE) oder später. Wer Kammzeit verändert und als Netzdienst betreibt, muss den Nutzern den geänderten Quellcode zugänglich machen (§ 13 AGPL). Das gilt auch für eine selbst gehostete Installation mit eigenen Änderungen.
