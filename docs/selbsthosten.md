# Kammzeit selbst hosten

Kammzeit kennt zwei Betriebsarten:

- **Einzelbetrieb** (`KAMMZEIT_SINGLE_TENANT=1`): genau ein Salon, alles unter einer Domain. Die Buchungsseite liegt unter `/`, das Dashboard unter `/dashboard`. Das ist der normale Weg für einen Salon auf dem eigenen Server.
- **Mehrmandanten-Betrieb**: wie kammzeit.de, beliebig viele Salons unter `<slug>.<BASE_DOMAIN>`. Siehe [unten](#mehrmandanten-betrieb).

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

`SITE_URL` muss im Einzelbetrieb nicht gesetzt werden, es übernimmt `PUBLIC_URL`. Die Stripe-Variablen bleiben leer: im Einzelbetrieb gibt es keine Plan-Grenzen und keinen Kammzeit-Hinweis auf der Buchungsseite.

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

Kammzeit verschickt Login-Links, Buchungsbestätigungen, Absagen und die Erinnerung 24 Stunden vor dem Termin. Die Erinnerungen prüft der Server selbst jede Minute, ein Cronjob ist nicht nötig. Ohne `SMTP_HOST` landen alle Mails nur im Log (`journalctl -u kammzeit`), das reicht zum Ausprobieren, aber nicht für den Betrieb.

- Port 587 mit STARTTLS verwenden. Port 465 (TLS ab Verbindungsbeginn) wird nicht unterstützt.
- `SMTP_HOST` muss zum Zertifikat des Mailservers passen, das Zertifikat wird bei STARTTLS geprüft.
- Mit `SMTP_USER` wird das Passwort nur über eine verschlüsselte Verbindung gesendet. Bietet der Server kein STARTTLS an, schlägt der Versand fehl (Ausnahme: Mailserver auf localhost).
- Die Adresse in `SMTP_FROM` muss der Mailserver als Absender erlauben. SPF und DKIM für die Absender-Domain einrichten, sonst landen die Mails im Spam.

Fehler beim Versand stehen im Log. Erinnerungen werden bei einem Fehler jede Minute erneut versucht.

### 6. Erste Einrichtung

`https://termine.salon-muster.de/registrieren` aufrufen, Salonname und Mailadresse eintragen, den Login-Link aus der Mail öffnen. Im Einzelbetrieb geht die Registrierung nur, solange noch kein Salon existiert. Danach im Dashboard Leistungen, Team mit Arbeitszeiten und die Einstellungen (Adresse, Akzentfarbe) pflegen. Die Buchungsseite ist `https://termine.salon-muster.de/`.

Im Einzelbetrieb nicht `kammzeit demo` ausführen: der Demo-Salon wäre dann der eine Salon und die Registrierung gesperrt.

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

Für einen Dienst wie kammzeit.de mit vielen Salons unter eigenen Subdomains.

- DNS: A- bzw. AAAA-Eintrag für `kammzeit.example` und als Wildcard für `*.kammzeit.example` auf den Server.
- `/etc/kammzeit.env`: `BASE_DOMAIN=kammzeit.example`, `LISTEN=127.0.0.1:8080`, `DB_PATH`, SMTP-Werte. Ohne `KAMMZEIT_SINGLE_TENANT`. `SITE_URL` (`https://kammzeit.example`) und `PUBLIC_URL` (`https://{slug}.kammzeit.example`) ergeben sich aus `BASE_DOMAIN`.
- Startseite, Registrierung, Login und Dashboard laufen auf der Hauptdomain, jede Buchungsseite unter `<slug>.kammzeit.example`.
- Caddy wie in [deploy/Caddyfile](../deploy/Caddyfile): Subdomains bekommen ihr Zertifikat beim ersten Aufruf (On-Demand-TLS). Vorher fragt Caddy `http://127.0.0.1:8080/_tls/allow?domain=...`, Kammzeit antwortet nur für die Hauptdomain, `www` und existierende Salons mit 200. So kann niemand mit erfundenen Subdomains Zertifikate anfordern und die Let's-Encrypt-Limits aufbrauchen.

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
- Pro-Abo: `STRIPE_SECRET_KEY`, `STRIPE_PRICE_MONTHLY`, `STRIPE_PRICE_YEARLY` setzen und in Stripe einen Webhook auf `https://kammzeit.example/stripe/webhook` anlegen, dessen Signing-Secret nach `STRIPE_WEBHOOK_SECRET`. Welche Ereignisse ausgewertet werden, steht in [billing.go](../billing.go).
- Rechtstexte: neben Impressum und Datenschutz auch AGB und AVV in [web/legal.html](../web/legal.html) für den eigenen Dienst anpassen.
