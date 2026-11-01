# Self-hosting Kammzeit

*Deutsche Fassung: [selbsthosten.md](selbsthosten.md)*

Kammzeit has two modes:

- **Single-salon mode** (`KAMMZEIT_SINGLE_TENANT=1`): exactly one salon, everything on one domain. The booking page is at `/`, the dashboard at `/dashboard`. This is the normal way to run Kammzeit for one salon on your own server. There are no plan limits: unlimited staff, the walk-in queue and staff logins without a Pro plan, and no Kammzeit notice on the booking page.
- **Multi-tenant mode**: like kammzeit.de and kammzeit.com, any number of salons under `<slug>.<BASE_DOMAIN>`, optionally with one German and one English main domain. See [below](#multi-tenant-mode).

## Configuration

Everything is configured through environment variables. The [table in the README](../README.md#konfiguration) has the same list in German.

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | Port, if `LISTEN` is not set |
| `LISTEN` | `:<PORT>` | Address the server listens on. Behind a reverse proxy: `127.0.0.1:8080` |
| `BASE_DOMAIN` | `localhost` | Main domain. Salons run under `<slug>.<BASE_DOMAIN>` |
| `BASE_DOMAINS` | empty | Several main domains with a language (`de` or `en`), e.g. `kammzeit.de=de,kammzeit.com=en`. The first one is the default domain and uses `SITE_URL` and `PUBLIC_URL`; every further domain runs under `https://<domain>` and `https://<slug>.<domain>`. Set `BASE_DOMAIN` to the first one. Sign-up is at `/signup` on English domains and `/registrieren` on German ones. Empty: only `BASE_DOMAIN`, in German |
| `SITE_URL` | `https://<BASE_DOMAIN>`, in single-salon mode same as `PUBLIC_URL` | Address of the start page, login and dashboard; base for login links |
| `PUBLIC_URL` | `https://{slug}.<BASE_DOMAIN>`, in single-salon mode `http://localhost:<PORT>` | Address of the booking pages; base for links in booking emails. `{slug}` is replaced with the salon |
| `DB_PATH` | `kammzeit.db` | SQLite file |
| `KAMMZEIT_SINGLE_TENANT` | empty | `1` = single-salon mode: exactly one salon, everything on one domain, no subdomains |
| `SMTP_HOST` | empty | Mail server. Empty: emails are only written to the log |
| `SMTP_PORT` | `587` | Mail server port (STARTTLS; port 465 is not supported) |
| `SMTP_USER` | empty | Mail server user name, empty = no authentication |
| `SMTP_PASS` | empty | Mail server password |
| `SMTP_FROM` | `Kammzeit <noreply@localhost>` | Sender of all emails |
| `MARKETING_SECRET` | empty | Secret key (at least 16 characters) for the unsubscribe links of Kammzeit news: the link in each marketing email unsubscribes without logging in (HMAC over the account ID). Empty: the link leads to the settings, unsubscribing only when logged in. Confirmation links (double opt-in) do not need it. Use a long random value, e.g. from `openssl rand -hex 32`, and never change it (old unsubscribe links would stop working) |
| `STRIPE_SECRET_KEY` | empty | Stripe secret API key (`sk_...`), hosted plans only |
| `STRIPE_WEBHOOK_SECRET` | empty | Signing secret of the webhook `/stripe/webhook` (`whsec_...`) |
| `STRIPE_PRICE_MONTHLY` | empty | Stripe price ID (`price_...`) for Pro monthly |
| `STRIPE_PRICE_YEARLY` | empty | Stripe price ID (`price_...`) for Pro yearly |

## Requirements

- Linux server with systemd, ports 80 and 443 reachable
- A domain (here `bookings.salon-example.com`) whose A or AAAA record points to the server
- Go in the version from [go.mod](../go.mod) or newer, to build (can be done on another machine)
- [Caddy](https://caddyserver.com) as reverse proxy for HTTPS
- An SMTP account for sending email
- `sqlite3` for backups

## Single-salon mode

### 1. Replace the legal pages

[web/legal.html](../web/legal.html) contains the imprint, privacy policy, terms and data processing agreement of kammzeit.de, in German, with the details of its operator. The pages are embedded in the binary and are reachable on your server too, at `/impressum`, `/datenschutz`, `/agb` and `/avv`; every page links the imprint and privacy policy in its footer.

Before building, replace the templates `impressum` and `datenschutz` with your own texts, in the language you need (the templates are only named in German, their content is up to you). `agb` and `avv` only concern the hosted service; a short note or a link to the imprint is enough there. Keep the template names (`{{define "..."}}`) as they are.

The privacy policy of kammzeit.de works as a starting point because it describes what the software processes: the login cookie `kz_session`; for the queue the first name, optionally the email address and the cookie `kz_queue` (valid for 12 hours, entries are deleted the following day); staff accounts with email address and last login; the installable app (the service worker only caches static files, no customer data); and the consent to Kammzeit news.

[web/legal_en.html](../web/legal_en.html) is the English translation (`en-imprint`, `en-privacy`, `en-terms`, `en-dpa`). It is only served in multi-tenant mode on an English main domain, at `/imprint`, `/privacy`, `/terms` and `/dpa`; single-salon mode only has the pages from `legal.html`. English booking pages (booking page language in the settings) currently link the privacy policy to `https://kammzeit.com/privacy`: if you run English booking pages yourself, change these links in [web/pages.html](../web/pages.html).

### 2. Build the binary

```sh
git clone https://github.com/luxend0/kammzeit.git
cd kammzeit
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kammzeit .
```

The binary is statically linked and contains the templates, schema and time zone data. To build for an ARM server on another machine, prefix the command with `GOOS=linux GOARCH=arm64`.

### 3. Install

```sh
sudo useradd --system --home-dir /var/lib/kammzeit --shell /usr/sbin/nologin kammzeit
sudo install -m 755 kammzeit /usr/local/bin/kammzeit
sudo install -m 644 deploy/kammzeit.service /etc/systemd/system/kammzeit.service
```

The unit [deploy/kammzeit.service](../deploy/kammzeit.service) runs as user `kammzeit`, creates `/var/lib/kammzeit` (`StateDirectory`) and reads its configuration from `/etc/kammzeit.env`. It does not set a working directory, so `DB_PATH` must be an absolute path.

`/etc/kammzeit.env`:

```sh
KAMMZEIT_SINGLE_TENANT=1
LISTEN=127.0.0.1:8080
PUBLIC_URL=https://bookings.salon-example.com
BASE_DOMAINS=bookings.salon-example.com=en
DB_PATH=/var/lib/kammzeit/kammzeit.db
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=bookings@salon-example.com
SMTP_PASS=secret
SMTP_FROM=Salon Example <bookings@salon-example.com>
```

`BASE_DOMAINS=<your domain>=en` switches the dashboard, login, sign-up and login emails to English and puts sign-up at `/signup`. Without it they are in German. The language of the booking page is separate: set it in the dashboard under Settings.

`SITE_URL` does not need to be set in single-salon mode, it follows `PUBLIC_URL`. Leave the Stripe variables empty: single-salon mode has no plan limits, so unlimited staff, the queue and staff logins without a subscription, and no Kammzeit notice on the booking page. `MARKETING_SECRET` is not needed either (only for unsubscribe links of Kammzeit news in multi-tenant mode).

```sh
sudo chmod 600 /etc/kammzeit.env
sudo systemctl daemon-reload
sudo systemctl enable --now kammzeit
journalctl -u kammzeit -f
```

### 4. Caddy in front

`/etc/caddy/Caddyfile`:

```
bookings.salon-example.com {
	reverse_proxy 127.0.0.1:8080
}
```

```sh
sudo systemctl reload caddy
```

Caddy fetches the certificate itself and redirects HTTP to HTTPS. On-demand TLS as in [deploy/Caddyfile](../deploy/Caddyfile) is only needed in multi-tenant mode.

Kammzeit only trusts `X-Forwarded-For` on connections from localhost. If the proxy runs on another machine, all visitors share the same rate limits for login and sign-up.

### 5. Set up SMTP

Kammzeit sends login links, booking confirmations, cancellations, the reminder 24 hours before the appointment, invitations for staff logins and, for the queue, an optional email shortly before it is someone's turn. All emails go out as HTML with a plain text version, in the language of the salon or of the account. The server checks for due reminders every minute by itself, no cron job needed. Without `SMTP_HOST` all emails only end up in the log (`journalctl -u kammzeit`), which is fine for trying it out but not for real use.

- Use port 587 with STARTTLS. Port 465 (TLS from the start of the connection) is not supported.
- `SMTP_HOST` must match the mail server's certificate; it is verified during STARTTLS.
- With `SMTP_USER` set, the password is only sent over an encrypted connection. If the server does not offer STARTTLS, sending fails (exception: a mail server on localhost).
- The mail server must allow the address in `SMTP_FROM` as sender. Set up SPF and DKIM for the sender domain, otherwise the emails end up in spam.

Sending errors are written to the log. Reminders are retried every minute after an error.

### 6. First setup

Open `https://bookings.salon-example.com/signup` (`/registrieren` on a German domain), enter the salon name, your first name and your email address, and open the login link from the email. In single-salon mode sign-up only works as long as no salon exists. On first login a short tour shows you around the dashboard; restart it any time from the question mark at the top.

Then set up in the dashboard:

- **Services**: the two sample services are switched off. Check them, adjust them and switch them on; only then can customers book them online.
- **Team**: staff members with working hours. A staff member gets their own login on their page under "Dashboard login", by invitation email. Staff see the calendar, customers and notes, run the queue and manage their working hours under "My hours". Plan, settings, services, team and export stay with the owner.
- **Settings**: address, accent colour, booking page language, time zone, booking rules.
- **Queue** (optional): switch it on, print the QR code and put it up in the salon. The display for a screen in the salon is on the same page.

The booking page is `https://bookings.salon-example.com/`. The dashboard can be installed like an app from the browser (Chrome, Edge, or Safari on iPhone: "Add to Home Screen").

Do not run `kammzeit demo` in single-salon mode: the demo salons would take the one slot and sign-up would be closed.

If no email arrives, there is an emergency login from the command line:

```sh
sudo -u kammzeit env DB_PATH=/var/lib/kammzeit/kammzeit.db /usr/local/bin/kammzeit session you@salon-example.com
```

Set the output `kz_session=...` as a cookie for the domain in your browser (developer tools, Storage or Application tab).

### 7. Backups

All data lives in one SQLite file (`DB_PATH`, plus `-wal` and `-shm` next to it). Copying it with `cp` while the server runs is not safe, `sqlite3 .backup` is.

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
Description=Kammzeit database backup

[Service]
Type=oneshot
User=kammzeit
ExecStart=/usr/local/bin/kammzeit-backup
```

`/etc/systemd/system/kammzeit-backup.timer`:

```ini
[Unit]
Description=Kammzeit database backup, daily

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

The backup runs as user `kammzeit` because sqlite3 needs write access to the `-shm` file in WAL mode. Backups on the same server do not protect against a disk failure: copy the files somewhere else as well (e.g. with restic or rsync).

Alternative: [Litestream](https://litestream.io) continuously replicates the database to S3-compatible storage.

```yaml
# /etc/litestream.yml
dbs:
  - path: /var/lib/kammzeit/kammzeit.db
    replicas:
      - url: s3://my-bucket/kammzeit
```

Litestream needs write access to `/var/lib/kammzeit`; the easiest way is to run the litestream service as user `kammzeit` too.

Restoring:

```sh
sudo systemctl stop kammzeit
sudo rm -f /var/lib/kammzeit/kammzeit.db-wal /var/lib/kammzeit/kammzeit.db-shm
sudo -u kammzeit cp /var/backups/kammzeit/kammzeit-2026-11-01.db /var/lib/kammzeit/kammzeit.db
sudo systemctl start kammzeit
```

Always delete the old `-wal` and `-shm` files first, otherwise SQLite mixes them into the restored file.

### 8. Updates

```sh
cd kammzeit
git pull
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kammzeit .
sudo systemctl start kammzeit-backup
sudo install -m 755 kammzeit /usr/local/bin/kammzeit
sudo systemctl restart kammzeit
```

Database schema changes run automatically on start and only forwards. Going back to an older version only works together with the backup from before the update. Commit your own changes (e.g. to `web/legal.html`), then `git pull` merges them with new versions.

## Docker

The [Dockerfile](../Dockerfile) builds an image without a shell that runs as non-root (UID 65532) in single-salon mode. The database lives in the volume at `/data/kammzeit.db`. Replace the legal pages as above before building.

```sh
docker build -t kammzeit .
docker run -d --name kammzeit --restart unless-stopped \
  --network host -e LISTEN=127.0.0.1:8080 \
  -v kammzeit-data:/data --env-file /etc/kammzeit.env \
  kammzeit
```

`/etc/kammzeit.env` as above, but without `DB_PATH` and `LISTEN` (set by the image and the command line). Caddy in front as in single-salon mode.

`--network host` makes Kammzeit see Caddy's connections as coming from localhost, so it takes the real visitor IP from `X-Forwarded-For`. It also works with `-p 127.0.0.1:8080:8080` instead of `--network host`, but then Kammzeit only sees the Docker bridge address and all visitors share the same rate limits for login and sign-up.

Commands in the container: `docker exec kammzeit /kammzeit session you@salon-example.com`. The image does not contain sqlite3, so run backups on the host with the script above, as root and with `kammzeit.db` in the directory from `docker volume inspect -f '{{.Mountpoint}}' kammzeit-data`.

## Multi-tenant mode

For a service like kammzeit.de and kammzeit.com with many salons on their own subdomains.

- DNS: A or AAAA record for `kammzeit.example` and a wildcard for `*.kammzeit.example` pointing to the server.
- `/etc/kammzeit.env`: `BASE_DOMAIN=kammzeit.example`, `LISTEN=127.0.0.1:8080`, `DB_PATH`, SMTP values. Without `KAMMZEIT_SINGLE_TENANT`. `SITE_URL` (`https://kammzeit.example`) and `PUBLIC_URL` (`https://{slug}.kammzeit.example`) follow from `BASE_DOMAIN`.
- Start page, sign-up, login and dashboard run on the main domain, each booking page under `<slug>.kammzeit.example`.
- Caddy as in [deploy/Caddyfile](../deploy/Caddyfile): subdomains get their certificate on first request (on-demand TLS). Before that, Caddy asks `http://127.0.0.1:8080/_tls/allow?domain=...`; Kammzeit only answers 200 for the main domains, their `www` and salons whose owner has logged in at least once. That way nobody can request certificates with made-up subdomains or sign-ups and use up the Let's Encrypt limits.

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

- Let's Encrypt issues at most 50 certificates per domain and week. If more new salons sign up per week, use a wildcard certificate via the DNS challenge (needs a Caddy build with your DNS provider's module).
- Pro plan: set `STRIPE_SECRET_KEY`, `STRIPE_PRICE_MONTHLY`, `STRIPE_PRICE_YEARLY` and create a webhook in Stripe to `https://kammzeit.example/stripe/webhook`, with its signing secret in `STRIPE_WEBHOOK_SECRET`. The events Kammzeit handles are listed in [billing.go](../billing.go). Without Pro: one active staff member, no queue, staff logins paused (nothing is deleted), Kammzeit notice on the booking page.
- Kammzeit news: Kammzeit stores the consents (double opt-in, with time, channel and wording) and sends the confirmation email, but not the newsletter itself. For unsubscribe links that work without logging in, set `MARKETING_SECRET` (at least 16 characters, e.g. `openssl rand -hex 32`) and never change it afterwards.
- Legal pages: besides imprint and privacy policy, adapt the terms and data processing agreement in [web/legal.html](../web/legal.html) for your own service, and with an English domain also [web/legal_en.html](../web/legal_en.html).

### Two languages (like kammzeit.de and kammzeit.com)

One instance can serve several main domains with one language each, sharing the database and accounts:

```sh
BASE_DOMAIN=kammzeit.example
BASE_DOMAINS=kammzeit.example=de,kammzeit-en.example=en
```

- The first domain is the default domain and uses `SITE_URL` and `PUBLIC_URL`; set `BASE_DOMAIN` to it. Every further domain runs at fixed addresses, `https://<domain>` and `https://<slug>.<domain>`.
- DNS: A or AAAA record and wildcard for both domains (`kammzeit-en.example` and `*.kammzeit-en.example`). In the Caddyfile, list both main domains in the block with `reverse_proxy` (like `kammzeit.de, kammzeit.com` in [deploy/Caddyfile](../deploy/Caddyfile)); salon subdomains of both domains go through the on-demand block.
- Website, login, sign-up and dashboard appear in the language of the domain, emails to salons in the account's language, emails to customers in the booking page language. Sign-up is at `/signup` on the English domain and `/registrieren` on the German one; the other path redirects permanently.
- One account works on both domains, sessions are per domain. Login links go to the domain of the account's language.
- Each salon chooses the language of its booking page. Links in booking emails point to `<slug>.<domain of that language>`.
- Visitors with a German browser language who open the English start page are sent to the German domain (start page only, not with `?lang=en`).
