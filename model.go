package main

// Gemeinsame Bausteine: Salon-Modell, Migrationen, Client-IP, Rate-Limit.

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type salon struct {
	ID                                     int64
	Slug, Name, Email, Accent, Plan, TZ    string
	Address, Phone, Website, ImprintURL    string
	About                                  string
	MinNoticeMin, CancelNoticeMin          int
	HorizonDays                            int
	StripeCustomerID, StripeSubscriptionID string
	Lang                                   string // Sprache der Buchungsseite: de | en
	// Anpassung der Buchungsseite (custom.go): Social-Links (geprüft gespeichert),
	// Öffnungszeiten-Box, Design (nur Pro), Logo/Titelbild = Hash in salon_media.
	Instagram, TikTok, Facebook, Google, WhatsApp string
	ShowHours                                     bool
	ThemeFont, ThemeRadius, ThemeMode, Welcome    string
	Logo, Cover                                   string
	Loc                                           *time.Location
}

// Pro ist der bezahlte Plan: unbegrenzt Mitarbeiter, ohne Branding, ohne Gebühr.
func (s *salon) Pro() bool { return s.Plan == "pro" }

const salonCols = `SELECT id, slug, name, email, accent, plan, timezone, address, phone, website, imprint_url, about,
	min_notice_min, cancel_notice_min, booking_horizon_days, stripe_customer_id, stripe_subscription_id, lang,
	instagram, tiktok, facebook, google, whatsapp, show_hours, theme_font, theme_radius, theme_mode, welcome_text,
	COALESCE((SELECT hash FROM salon_media m WHERE m.salon_id = salons.id AND m.kind = 'logo'), ''),
	COALESCE((SELECT hash FROM salon_media m WHERE m.salon_id = salons.id AND m.kind = 'cover'), '') FROM salons `

func scanSalon(row interface{ Scan(...any) error }) (*salon, error) {
	var s salon
	err := row.Scan(&s.ID, &s.Slug, &s.Name, &s.Email, &s.Accent, &s.Plan, &s.TZ, &s.Address, &s.Phone, &s.Website,
		&s.ImprintURL, &s.About, &s.MinNoticeMin, &s.CancelNoticeMin, &s.HorizonDays, &s.StripeCustomerID, &s.StripeSubscriptionID, &s.Lang,
		&s.Instagram, &s.TikTok, &s.Facebook, &s.Google, &s.WhatsApp, &s.ShowHours, &s.ThemeFont, &s.ThemeRadius, &s.ThemeMode, &s.Welcome,
		&s.Logo, &s.Cover)
	if err != nil {
		return nil, err
	}
	s.Loc = location(s.TZ)
	if s.Lang != "en" {
		s.Lang = "de"
	}
	if !hexColor.MatchString(s.Accent) {
		s.Accent = "#3b82f6"
	}
	return &s, nil
}

func (a *app) loadSalon(id int64) (*salon, error) {
	return scanSalon(a.db.QueryRow(salonCols+`WHERE id = ?`, id))
}

// Free-Plan: höchstens so viele aktive Mitarbeiter.
const freeStaffLimit = 1

// migrations ändern bestehende Tabellen. Nur anhängen, nie umsortieren oder
// ändern: Index+1 = PRAGMA user_version nach dem Lauf.
var migrations = []string{
	// 1: Nachweis, wann welche AGB/AVV-Fassung akzeptiert wurde.
	`ALTER TABLE users ADD COLUMN terms_accepted_at INTEGER;
	 ALTER TABLE users ADD COLUMN terms_version TEXT NOT NULL DEFAULT '';`,
	// 2: Sprache der Buchungsseite; Marketing-Einwilligung der Salon-Inhaber mit
	// Double-Opt-in (Nachweis: wann, über welchen Weg, welcher Text; Widerruf).
	`ALTER TABLE salons ADD COLUMN lang TEXT NOT NULL DEFAULT 'de';
	 ALTER TABLE users ADD COLUMN lang TEXT NOT NULL DEFAULT 'de';
	 ALTER TABLE users ADD COLUMN marketing_at INTEGER;
	 ALTER TABLE users ADD COLUMN marketing_source TEXT NOT NULL DEFAULT '';
	 ALTER TABLE users ADD COLUMN marketing_text TEXT NOT NULL DEFAULT '';
	 ALTER TABLE users ADD COLUMN marketing_withdrawn_at INTEGER;
	 ALTER TABLE users ADD COLUMN marketing_pending_hash TEXT NOT NULL DEFAULT '';
	 ALTER TABLE users ADD COLUMN marketing_pending_at INTEGER;`,
	// 3: Mitarbeiter-Logins (Rolle owner|staff, staff_id = zugehöriger Mitarbeiter)
	// und Warteschlange für Laufkundschaft (pro Salon schaltbar, nur Pro).
	`ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'owner' CHECK (role IN ('owner', 'staff'));
	 ALTER TABLE users ADD COLUMN staff_id INTEGER REFERENCES staff(id) ON DELETE SET NULL;
	 ALTER TABLE salons ADD COLUMN queue_enabled INTEGER NOT NULL DEFAULT 0;
	 ALTER TABLE salons ADD COLUMN queue_note TEXT NOT NULL DEFAULT '';
	 CREATE TABLE queue_entries (
	     id          INTEGER PRIMARY KEY,
	     salon_id    INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
	     number      INTEGER NOT NULL,              -- Nummer des Tages (1, 2, 3 ...)
	     day         TEXT NOT NULL,                 -- YYYY-MM-DD in Salon-Zeitzone
	     name        TEXT NOT NULL,
	     email       TEXT,                          -- optional, für "gleich dran"-Mail
	     service_id  INTEGER REFERENCES services(id) ON DELETE SET NULL,
	     staff_id    INTEGER REFERENCES staff(id) ON DELETE SET NULL,
	     status      TEXT NOT NULL DEFAULT 'waiting' CHECK (status IN ('waiting', 'called', 'serving', 'done', 'noshow', 'left')),
	     token       TEXT NOT NULL UNIQUE,          -- Statusseite des Kunden
	     created_at  INTEGER NOT NULL DEFAULT (unixepoch()),
	     called_at   INTEGER,
	     done_at     INTEGER,
	     notified_at INTEGER,
	     UNIQUE (salon_id, day, number)
	 );
	 CREATE INDEX queue_entries_salon_day ON queue_entries (salon_id, day, status);`,
	// 4: Anpassung der Buchungsseite: Bilder (Logo, Titelbild, Galerie) als BLOB,
	// damit das Backup eine Datei bleibt; Social-Links, Öffnungszeiten-Box, Design.
	`CREATE TABLE salon_media (
	     id         INTEGER PRIMARY KEY,
	     salon_id   INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
	     kind       TEXT NOT NULL CHECK (kind IN ('logo', 'cover', 'gallery')),
	     mime       TEXT NOT NULL CHECK (mime IN ('image/jpeg', 'image/png')), -- immer neu kodiert
	     data       BLOB NOT NULL,
	     width      INTEGER NOT NULL,
	     height     INTEGER NOT NULL,
	     hash       TEXT NOT NULL UNIQUE,          -- Adresse /media/{hash}, ändert sich mit dem Inhalt
	     sort       INTEGER NOT NULL DEFAULT 0,
	     alt        TEXT NOT NULL DEFAULT '',
	     created_at INTEGER NOT NULL DEFAULT (unixepoch())
	 );
	 CREATE INDEX salon_media_salon ON salon_media (salon_id, kind, sort);
	 CREATE UNIQUE INDEX salon_media_single ON salon_media (salon_id, kind) WHERE kind IN ('logo', 'cover');
	 ALTER TABLE salons ADD COLUMN instagram TEXT NOT NULL DEFAULT '';
	 ALTER TABLE salons ADD COLUMN tiktok TEXT NOT NULL DEFAULT '';
	 ALTER TABLE salons ADD COLUMN facebook TEXT NOT NULL DEFAULT '';
	 ALTER TABLE salons ADD COLUMN google TEXT NOT NULL DEFAULT '';         -- Maps/Bewertungen
	 ALTER TABLE salons ADD COLUMN whatsapp TEXT NOT NULL DEFAULT '';       -- nur Ziffern mit Ländervorwahl
	 ALTER TABLE salons ADD COLUMN show_hours INTEGER NOT NULL DEFAULT 1;
	 ALTER TABLE salons ADD COLUMN theme_font TEXT NOT NULL DEFAULT 'schlicht';
	 ALTER TABLE salons ADD COLUMN theme_radius TEXT NOT NULL DEFAULT 'square';
	 ALTER TABLE salons ADD COLUMN theme_mode TEXT NOT NULL DEFAULT 'light';
	 ALTER TABLE salons ADD COLUMN welcome_text TEXT NOT NULL DEFAULT '';`,
}

// termsVersion = Stand von AGB und AVV in web/legal.html. Bei inhaltlicher
// Änderung hochzählen.
const termsVersion = "2026-11-01"

func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// clientIP: hinter Caddy (Verbindung von localhost) zählt X-Forwarded-For,
// sonst die Gegenstelle. Von außen gesetzte Header werden so ignoriert. Der
// Proxy hängt die echte Gegenstelle hinten an: zählt nur der letzte Eintrag
// der letzten Headerzeile, alles davor kann der Client selbst geschickt haben.
// Kein gültiger Eintrag: die Gegenstelle.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
			parts := strings.Split(xff[len(xff)-1], ",")
			if fwd := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); fwd != nil {
				return fwd.String()
			}
		}
	}
	return host
}

// clientKey ist der Schlüssel für alle Rate-Limits pro Anschluss: die
// IPv4-Adresse, bei IPv6 das /64-Netz. Ein Anschluss bekommt meist ein ganzes
// /64 und könnte sonst für jede Anfrage eine neue Adresse nehmen.
// Überall verwenden, wo ein Limiter nach Client fragt, nicht clientIP.
func clientKey(r *http.Request) string {
	host := clientIP(r)
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() != nil {
		return host
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// limiterMaxKeys begrenzt den Speicher eines Limiters. Ist er voll, werden
// neue Schlüssel bis zum nächsten Aufräumen abgelehnt (bekannte zählen weiter).
const limiterMaxKeys = 50000

// limiter erlaubt n Vorgänge pro Schlüssel und Zeitfenster.
// ponytail: im Speicher, ein Prozess; reicht für eine Instanz.
type limiter struct {
	mu    sync.Mutex
	n     int
	per   time.Duration
	hits  map[string][]time.Time
	swept time.Time
}

func newLimiter(n int, per time.Duration) *limiter {
	return &limiter{n: n, per: per, hits: map[string][]time.Time{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// Abgelaufene Schlüssel (IP-Adressen, Mail-Adressen) spätestens nach einem
	// Zeitfenster wegwerfen, so steht es auch in der Datenschutzerklärung.
	// Nur zeitgesteuert (höchstens einmal pro Minute), damit viele frische
	// Schlüssel nicht jeden Aufruf zu einer Schleife über alle machen.
	if now.Sub(l.swept) > min(l.per, time.Minute) {
		l.swept = now
		for k, ts := range l.hits {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > l.per {
				delete(l.hits, k)
			}
		}
	}
	ts, known := l.hits[key]
	if !known && len(l.hits) >= limiterMaxKeys {
		return false
	}
	i := 0
	for i < len(ts) && now.Sub(ts[i]) > l.per {
		i++
	}
	ts = ts[i:]
	if len(ts) >= l.n {
		l.hits[key] = ts
		return false
	}
	l.hits[key] = append(ts, now)
	return true
}
