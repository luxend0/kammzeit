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
	Loc                                    *time.Location
}

// Pro ist der bezahlte Plan: unbegrenzt Mitarbeiter, ohne Branding, ohne Gebühr.
func (s *salon) Pro() bool { return s.Plan == "pro" }

const salonCols = `SELECT id, slug, name, email, accent, plan, timezone, address, phone, website, imprint_url, about,
	min_notice_min, cancel_notice_min, booking_horizon_days, stripe_customer_id, stripe_subscription_id FROM salons `

func scanSalon(row interface{ Scan(...any) error }) (*salon, error) {
	var s salon
	err := row.Scan(&s.ID, &s.Slug, &s.Name, &s.Email, &s.Accent, &s.Plan, &s.TZ, &s.Address, &s.Phone, &s.Website,
		&s.ImprintURL, &s.About, &s.MinNoticeMin, &s.CancelNoticeMin, &s.HorizonDays, &s.StripeCustomerID, &s.StripeSubscriptionID)
	if err != nil {
		return nil, err
	}
	s.Loc = location(s.TZ)
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
var migrations = []string{}

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
// sonst die Gegenstelle. Von außen gesetzte Header werden so ignoriert.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return host
}

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
	if now.Sub(l.swept) > l.per || len(l.hits) > 10000 {
		l.swept = now
		for k, ts := range l.hits {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > l.per {
				delete(l.hits, k)
			}
		}
	}
	ts := l.hits[key]
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
