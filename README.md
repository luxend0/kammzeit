# Kammzeit

Online-Terminbuchung für Friseure und Salons. Kunden wählen Leistung, Mitarbeiter und Uhrzeit, bekommen eine Bestätigung per Mail, eine Erinnerung 24 Stunden vorher und einen Link zum Absagen.

Eine einzige Go-Binary, SQLite als Datenbank, keine weiteren Dienste nötig. Die gehostete Version gibt es auf [kammzeit.de](https://kammzeit.de).

> Stand: frühes Gerüst. Buchen, Bestätigen, Erinnern und Absagen funktionieren, der Admin-Bereich (Kalender, Kundenkartei pflegen) fehlt noch.

## Selbst hosten

Binary:

```sh
CGO_ENABLED=0 go build -o kammzeit .
KAMMZEIT_SINGLE_TENANT=1 ./kammzeit demo   # legt einen Beispiel-Salon an
KAMMZEIT_SINGLE_TENANT=1 ./kammzeit        # http://localhost:8080
```

Docker:

```sh
docker build -t kammzeit .
docker run -d -p 8080:8080 -v kammzeit-data:/data kammzeit
docker run --rm -v kammzeit-data:/data kammzeit demo
```

Für HTTPS einen Reverse-Proxy (z. B. Caddy) davor setzen.

## Konfiguration

Alles über Umgebungsvariablen:

| Variable | Standard | Bedeutung |
|---|---|---|
| `PORT` | `8080` | HTTP-Port |
| `DB_PATH` | `kammzeit.db` | SQLite-Datei |
| `KAMMZEIT_SINGLE_TENANT` | leer | `1` = genau ein Salon, keine Subdomains |
| `BASE_DOMAIN` | `localhost` | Mehrere Salons laufen unter `<salon>.<BASE_DOMAIN>` |
| `PUBLIC_URL` | `https://{slug}.<BASE_DOMAIN>` bzw. `http://localhost:<PORT>` | Basis für Links in Mails, `{slug}` wird ersetzt |
| `SMTP_HOST` | leer | Mailserver, ohne ihn werden Mails nur geloggt |
| `SMTP_PORT` | `587` | |
| `SMTP_USER`, `SMTP_PASS` | leer | Anmeldung am Mailserver |
| `SMTP_FROM` | `Kammzeit <noreply@localhost>` | Absender |

Lokal mit mehreren Salons: `./kammzeit demo && ./kammzeit`, dann `http://demo.localhost:8080` öffnen.

## Lizenz

[GNU AGPL-3.0](LICENSE). Wer Kammzeit verändert und als Dienst anbietet, muss den geänderten Quellcode den Nutzern zugänglich machen.
