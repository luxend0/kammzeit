# Kammzeit selbst hosten

*English version: [self-hosting.md](self-hosting.md)*

Kammzeit kennt zwei Betriebsarten:

- **Einzelbetrieb** (`KAMMZEIT_SINGLE_TENANT=1`): genau ein Salon, alles unter einer Domain. Die Buchungsseite liegt unter `/`, das Dashboard unter `/dashboard`. Das ist der normale Weg für einen Salon auf dem eigenen Server. Es gibt keine Plan-Grenzen: beliebig viele Mitarbeiter, Warteschlange für Laufkundschaft und Mitarbeiter-Logins ohne Pro-Abo, kein Kammzeit-Hinweis auf der Buchungsseite.
- **Mehrmandanten-Betrieb**: wie kammzeit.de und kammzeit.com, beliebig viele Salons unter `<slug>.<BASE_DOMAIN>`, auf Wunsch mit einer deutschen und einer englischen Hauptdomain. Siehe [unten](#mehrmandanten-betrieb).

Alle Umgebungsvariablen stehen in der [Konfigurationstabelle im README](../README.md#konfiguration).

## Voraussetzungen

- Linux-Server mit systemd, Ports 80 und 443 erreichbar
- Eine Domain (hier `termine.salon-muster.de`), deren A- bzw. AAAA-Eintrag auf den Server zeigt
- Go in der Version aus [go.mod](../go.mod) oder neuer, zum Bauen (geht auch auf einem anderen Rechner)
- [Caddy](https://caddyserver.com) als Reverse-Proxy für HTTPS
- Ein SMTP-Zugang zum Mailversand
- `sqlite3` für Backups

## Einzelbetrieb

### 1. Rechtstexte anpassen

[web/legal.html](../web/legal.html) enthält Impressum, Datenschutzerklärung, AGB und AVV von kammzeit.de mit den Angaben des dortigen Betreibers. Die Seiten werden in die Binary eingebettet und sind auch auf deinem Server unter `/impressum`, `/datenschutz`, `/agb` und `/avv` erreichbar, jede Seite verlinkt Impressum und Datenschutz im Fuß.

Vor dem Bauen die Templates `impressum` und `datenschutz` durch deine eigenen Texte ersetzen. `agb` und `avv` betreffen nur die gehostete Version, dort reicht ein kurzer Hinweis oder ein Verweis auf das Impressum. Die Template-Namen (`{{define "..."}}`) bleiben gleich.

Die Datenschutzerklärung von kammzeit.de taugt als Vorlage, weil sie beschreibt, was die Software verarbeitet: das Login-Cookie `kz_session`, bei der Warteschlange Vorname, freiwillig die Mail-Adresse und das Cookie `kz_queue` (12 Stunden gültig, Einträge werden am Folgetag gelöscht), Mitarbeiter-Konten mit Mail-Adresse und letzter Anmeldung, die installierbare App (der Service Worker speichert nur statische Dateien, keine Kundendaten) und die Einwilligung in Kammzeit-News.

[web/legal_en.html](../web/legal_en.html) ist die englische Fassung (`en-imprint`, `en-privacy`, `en-terms`, `en-dpa`). Sie läuft nur im Mehrmandanten-Betrieb auf einer englischen Hauptdomain unter `/imprint`, `/privacy`, `/terms` und `/dpa`, im Einzelbetrieb gibt es nur die Seiten aus `legal.html`. Englische Buchungsseiten (Sprache der Buchungsseite in den Einstellungen) verlinken die Datenschutzerklärung derzeit fest auf `https://kammzeit.com/privacy`: wer englische Buchungsseiten selbst betreibt, ändert diese Links in [web/pages.html](../web/pages.html).

### 2. Binary bauen

```sh
git clone https://github.com/luxend0/kammzeit.git
cd kammzeit
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kammzeit .
```

Die Binary ist statisch gelinkt und enthält Templates, Schema und Zeitzonendaten. Für einen ARM-Server auf einem anderen Rechner bauen mit `GOOS=linux GOARCH=arm64` davor.

### 3. Installieren

```sh
sudo useradd --system --home-dir /var/lib/kammzeit --shell /usr/sbin/nologin kammzeit
sudo install -m 755 kammzeit /usr/local/bin/kammzeit
sudo install -m 644 deploy/kammzeit.service /etc/systemd/system/kammzeit.service
```

Die Unit [deploy/kammzeit.service](../deploy/kammzeit.service) läuft als Benutzer `kammzeit`, legt `/var/lib/kammzeit` an (`StateDirectory`) und liest die Konfiguration aus `/etc/kammzeit.env`. Sie setzt kein Arbeitsverzeichnis, `DB_PATH` muss deshalb ein absoluter Pfad sein.

`/etc/kammzeit.env`:

```sh
KAMMZEIT_SINGLE_TENANT=1
LISTEN=127.0.0.1:8080
PUBLIC_URL=https://termine.salon-muster.de
DB_PATH=/var/lib/kammzeit/kammzeit.db
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=termine@salon-muster.de
SMTP_PASS=geheim
SMTP_FROM=Salon Muster <termine@salon-muster.de>
```

`SITE_URL` muss im Einzelbetrieb nicht gesetzt werden, es übernimmt `PUBLIC_URL`. Die Stripe-Variablen bleiben leer: im Einzelbetrieb gibt es keine Plan-Grenzen, also beliebig viele Mitarbeiter, Warteschlange und Mitarbeiter-Logins ohne Abo, und keinen Kammzeit-Hinweis auf der Buchungsseite. `MARKETING_SECRET` braucht es im Einzelbetrieb nicht (nur für Abmeldelinks der Kammzeit-News im Mehrmandanten-Betrieb).

Dashboard, Login und Registrierung sind auf Deutsch. Für ein englisches Dashboard die Domain als englisch eintragen: `BASE_DOMAINS=termine.salon-muster.de=en`. Die Registrierung liegt dann unter `/signup`, Login-Mails kommen auf Englisch. Die Sprache der Buchungsseite stellst du davon unabhängig im Dashboard unter Einstellungen ein.

```sh
sudo chmod 600 /etc/kammzeit.env
sudo systemctl daemon-reload
sudo systemctl enable --now kammzeit
journalctl -u kammzeit -f
```

### 4. Caddy davor

`/etc/caddy/Caddyfile`:

```
termine.salon-muster.de {
	reverse_proxy 127.0.0.1:8080
}
```

```sh
sudo systemctl reload caddy
```

Caddy holt das Zertifikat selbst und leitet HTTP auf HTTPS um. On-Demand-TLS wie in [deploy/Caddyfile](../deploy/Caddyfile) braucht es nur im Mehrmandanten-Betrieb.

Kammzeit wertet `X-Forwarded-For` nur bei Verbindungen von localhost aus. Läuft der Proxy auf einem anderen Rechner, teilen sich alle Besucher dieselben Rate-Limits für Login und Registrierung.

### 5. SMTP einrichten

Kammzeit verschickt Login-Links, Buchungsbestätigungen, Absagen, die Erinnerung 24 Stunden vor dem Termin, Einladungen für Mitarbeiter-Logins und bei der Warteschlange auf Wunsch eine Mail kurz bevor jemand dran ist. Alle Mails gehen als HTML mit Textfassung raus, in der Sprache des Salons bzw. des Kontos. Die Erinnerungen prüft der Server selbst jede Minute, ein Cronjob ist nicht nötig. Ohne `SMTP_HOST` landen alle Mails nur im Log (`journalctl -u kammzeit`), das reicht zum Ausprobieren, aber nicht für den Betrieb.

- Port 587 mit STARTTLS verwenden. Port 465 (TLS ab Verbindungsbeginn) wird nicht unterstützt.
- `SMTP_HOST` muss zum Zertifikat des Mailservers passen, das Zertifikat wird bei STARTTLS geprüft.
- Mit `SMTP_USER` wird das Passwort nur über eine verschlüsselte Verbindung gesendet. Bietet der Server kein STARTTLS an, schlägt der Versand fehl (Ausnahme: Mailserver auf localhost).
- Die Adresse in `SMTP_FROM` muss der Mailserver als Absender erlauben. SPF und DKIM für die Absender-Domain einrichten, sonst landen die Mails im Spam.

Fehler beim Versand stehen im Log. Erinnerungen werden bei einem Fehler jede Minute erneut versucht.

### 6. Erste Einrichtung

`https://termine.salon-muster.de/registrieren` aufrufen (mit englischer Domain `/signup`), Salonname, deinen Vornamen und deine Mailadresse eintragen, den Login-Link aus der Mail öffnen. Im Einzelbetrieb geht die Registrierung nur, solange noch kein Salon existiert. Beim ersten Login zeigt eine kurze Tour das Dashboard, neu starten lässt sie sich über das Fragezeichen oben.

Danach im Dashboard einrichten:

- **Leistungen**: die zwei Beispiel-Leistungen sind ausgeschaltet. Prüfen, anpassen und einschalten, erst dann sind sie online buchbar.
- **Team**: Mitarbeiter mit Arbeitszeiten. Einen eigenen Login bekommt ein Mitarbeiter auf seiner Seite unter "Login fürs Dashboard" per Einladungsmail. Mitarbeiter sehen Kalender, Kunden und Notizen, bedienen die Warteschlange und pflegen unter "Meine Zeiten" ihre Arbeitszeiten. Abo, Einstellungen, Leistungen, Team und Export bleiben beim Inhaber.
- **Einstellungen**: Adresse, Akzentfarbe, Sprache der Buchungsseite, Zeitzone, Buchungsregeln.
- **Warteschlange** (optional): einschalten, QR-Code ausdrucken und im Laden aufhängen. Die Anzeige für einen Bildschirm im Laden liegt auf derselben Seite.

Die Buchungsseite ist `https://termine.salon-muster.de/`. Das Dashboard lässt sich im Browser wie eine App installieren (Chrome, Edge, Safari auf dem iPhone: "Zum Home-Bildschirm").

Im Einzelbetrieb nicht `kammzeit demo` ausführen: die Demo-Salons belegen dann den einen Platz und die Registrierung ist gesperrt.

Kommt keine Mail an, gibt es einen Notzugang über die Kommandozeile:

```sh
sudo -u kammzeit env DB_PATH=/var/lib/kammzeit/kammzeit.db /usr/local/bin/kammzeit session du@salon-muster.de
```

Die Ausgabe `kz_session=...` im Browser als Cookie für die Domain setzen (Entwicklerwerkzeuge, Bereich Speicher bzw. Anwendung).

### 7. Backups

Alle Daten stecken in einer SQLite-Datei (`DB_PATH`, dazu `-wal` und `-shm` daneben). Eine Kopie mit `cp` im laufenden Betrieb ist nicht sicher, `sqlite3 .backup` schon.

`/usr/local/bin/kammzeit-backup`:

```sh
#!/bin/sh
set -eu
dir=/var/backups/kammzeit
sqlite3 -cmd ".timeout 5000" /var/lib/kammzeit/kammzeit.db ".backup $dir/kammzeit-$(date +%F).db"
find "$dir" -name 'kammzeit-*.db' -mtime +14 -delete
```

`/etc/systemd/system/kammzeit-backup.service`:

```ini
[Unit]
Description=Kammzeit Datenbank-Backup

[Service]
Type=oneshot
User=kammzeit
ExecStart=/usr/local/bin/kammzeit-backup
```

`/etc/systemd/system/kammzeit-backup.timer`:

```ini
[Unit]
Description=Kammzeit Datenbank-Backup täglich

[Timer]
OnCalendar=*-*-* 03:30
Persistent=true

[Install]
WantedBy=timers.target
```

```sh
sudo chmod 755 /usr/local/bin/kammzeit-backup
sudo install -d -o kammzeit -g kammzeit -m 700 /var/backups/kammzeit
sudo systemctl daemon-reload
sudo systemctl enable --now kammzeit-backup.timer
```

Das Backup läuft als Benutzer `kammzeit`, weil sqlite3 im WAL-Modus Schreibrechte auf die `-shm`-Datei braucht. Backups auf demselben Server schützen nicht vor einem Plattenausfall: die Dateien zusätzlich woanders hin kopieren (z. B. mit restic oder rsync).

Alternative: [Litestream](https://litestream.io) repliziert die Datenbank laufend auf S3-kompatiblen Speicher.

```yaml
# /etc/litestream.yml
dbs:
  - path: /var/lib/kammzeit/kammzeit.db
    replicas:
      - url: s3://mein-bucket/kammzeit
```

Litestream braucht Schreibzugriff auf `/var/lib/kammzeit`, am einfachsten läuft der litestream-Dienst ebenfalls als Benutzer `kammzeit`.

Zurückspielen:

```sh
sudo systemctl stop kammzeit
sudo rm -f /var/lib/kammzeit/kammzeit.db-wal /var/lib/kammzeit/kammzeit.db-shm
sudo -u kammzeit cp /var/backups/kammzeit/kammzeit-2026-11-01.db /var/lib/kammzeit/kammzeit.db
sudo systemctl start kammzeit
```

Die alten `-wal`- und `-shm`-Dateien unbedingt vorher löschen, sonst mischt SQLite sie in die zurückgespielte Datei.

### 8. Updates

```sh
cd kammzeit
git pull
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kammzeit .
sudo systemctl start kammzeit-backup
sudo install -m 755 kammzeit /usr/local/bin/kammzeit
sudo systemctl restart kammzeit
```

Änderungen am Datenbankschema laufen beim Start automatisch und nur vorwärts. Zurück auf eine ältere Version geht nur zusammen mit dem Backup von vor dem Update. Eigene Änderungen (z. B. an `web/legal.html`) committen, dann führt `git pull` sie mit den neuen Versionen zusammen.

## Docker

Das [Dockerfile](../Dockerfile) baut ein Image ohne Shell, das als nicht-root (UID 65532) im Einzelbetrieb läuft. Die Datenbank liegt im Volume unter `/data/kammzeit.db`. Rechtstexte wie oben vor dem Bauen anpassen.

```sh
docker build -t kammzeit .
docker run -d --name kammzeit --restart unless-stopped \
  --network host -e LISTEN=127.0.0.1:8080 \
  -v kammzeit-data:/data --env-file /etc/kammzeit.env \
  kammzeit
```

`/etc/kammzeit.env` wie oben, aber ohne `DB_PATH` und `LISTEN` (die setzt das Image bzw. die Kommandozeile). Caddy davor wie im Einzelbetrieb.

`--network host` sorgt dafür, dass Kammzeit die Verbindungen von Caddy als localhost sieht und die echte Besucher-IP aus `X-Forwarded-For` nimmt. Mit `-p 127.0.0.1:8080:8080` statt `--network host` läuft es auch, dann sieht Kammzeit nur die Adresse der Docker-Bridge und alle Besucher teilen sich dieselben Rate-Limits für Login und Registrierung.

Befehle im Container: `docker exec kammzeit /kammzeit session du@salon-muster.de`. Das Image enthält kein sqlite3. Backups deshalb auf dem Host mit dem Skript oben, als root und mit `kammzeit.db` im Verzeichnis aus `docker volume inspect -f '{{.Mountpoint}}' kammzeit-data`.

## Mehrmandanten-Betrieb

Für einen Dienst wie kammzeit.de und kammzeit.com mit vielen Salons unter eigenen Subdomains.

- DNS: A- bzw. AAAA-Eintrag für `kammzeit.example` und als Wildcard für `*.kammzeit.example` auf den Server.
- `/etc/kammzeit.env`: `BASE_DOMAIN=kammzeit.example`, `LISTEN=127.0.0.1:8080`, `DB_PATH`, SMTP-Werte. Ohne `KAMMZEIT_SINGLE_TENANT`. `SITE_URL` (`https://kammzeit.example`) und `PUBLIC_URL` (`https://{slug}.kammzeit.example`) ergeben sich aus `BASE_DOMAIN`.
- Startseite, Registrierung, Login und Dashboard laufen auf der Hauptdomain, jede Buchungsseite unter `<slug>.kammzeit.example`.
- Caddy wie in [deploy/Caddyfile](../deploy/Caddyfile): Subdomains bekommen ihr Zertifikat beim ersten Aufruf (On-Demand-TLS). Vorher fragt Caddy `http://127.0.0.1:8080/_tls/allow?domain=...`, Kammzeit antwortet nur für die Hauptdomains, deren `www` und Salons, deren Inhaber sich schon einmal angemeldet hat, mit 200. So kann niemand mit erfundenen Subdomains oder Registrierungen Zertifikate anfordern und die Let's-Encrypt-Limits aufbrauchen.

```
{
	on_demand_tls {
		ask http://127.0.0.1:8080/_tls/allow
	}
}

kammzeit.example {
	reverse_proxy 127.0.0.1:8080
}

https:// {
	tls {
		on_demand
	}
	reverse_proxy 127.0.0.1:8080
}
```

- Let's Encrypt stellt höchstens 50 Zertifikate pro Domain und Woche aus. Kommen mehr neue Salons pro Woche dazu, ein Wildcard-Zertifikat über die DNS-Challenge nehmen (braucht einen Caddy-Build mit dem Modul des DNS-Anbieters).
- Pro-Abo: `STRIPE_SECRET_KEY`, `STRIPE_PRICE_MONTHLY`, `STRIPE_PRICE_YEARLY` setzen und in Stripe einen Webhook auf `https://kammzeit.example/stripe/webhook` anlegen, dessen Signing-Secret nach `STRIPE_WEBHOOK_SECRET`. Welche Ereignisse ausgewertet werden, steht in [billing.go](../billing.go). Ohne Pro gilt: ein aktiver Mitarbeiter, keine Warteschlange, Mitarbeiter-Logins pausiert (gelöscht wird nichts), Kammzeit-Hinweis auf der Buchungsseite.
- Kammzeit-News: Kammzeit speichert die Einwilligungen (Double-Opt-in, mit Zeitpunkt, Weg und Text) und verschickt die Bestätigungsmail, den Newsletter selbst nicht. Für Abmeldelinks ohne Anmeldung `MARKETING_SECRET` setzen (mindestens 16 Zeichen, z. B. `openssl rand -hex 32`) und danach nicht mehr ändern.
- Rechtstexte: neben Impressum und Datenschutz auch AGB und AVV in [web/legal.html](../web/legal.html) für den eigenen Dienst anpassen, mit englischer Domain auch [web/legal_en.html](../web/legal_en.html).

### Zwei Sprachen (wie kammzeit.de und kammzeit.com)

Eine Instanz kann mehrere Hauptdomains mit je einer Sprache bedienen, gleiche Datenbank, gleiche Konten:

```sh
BASE_DOMAIN=kammzeit.example
BASE_DOMAINS=kammzeit.example=de,kammzeit-en.example=en
```

- Die erste Domain ist die Standarddomain und nutzt `SITE_URL` und `PUBLIC_URL`, `BASE_DOMAIN` auf sie setzen. Jede weitere läuft fest unter `https://<domain>` und `https://<slug>.<domain>`.
- DNS: A- bzw. AAAA-Eintrag und Wildcard für beide Domains (`kammzeit-en.example` und `*.kammzeit-en.example`). In der Caddyfile beide Hauptdomains in den Block mit `reverse_proxy` schreiben (wie `kammzeit.de, kammzeit.com` in [deploy/Caddyfile](../deploy/Caddyfile)), Salon-Subdomains beider Domains laufen über den On-Demand-Block.
- Website, Login, Registrierung und Dashboard erscheinen in der Sprache der Domain, Mails an Salons in der Sprache des Kontos, Mails an Kunden in der Sprache der Buchungsseite. Die Registrierung liegt auf der englischen Domain unter `/signup`, auf der deutschen unter `/registrieren`; der jeweils andere Pfad leitet dauerhaft um.
- Ein Konto gilt auf beiden Domains, angemeldet ist man je Domain. Login-Links gehen auf die Domain der Kontosprache.
- Jeder Salon wählt die Sprache seiner Buchungsseite. Links in Buchungsmails zeigen auf `<slug>.<Domain dieser Sprache>`.
- Wer mit deutscher Browsersprache die englische Startseite aufruft, wird auf die deutsche Domain geschickt (nur die Startseite, nicht mit `?lang=en`).
