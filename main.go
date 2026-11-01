// Kammzeit - Online-Terminbuchung für Friseure und Salons.
// Copyright (C) 2026 Ben Oliver Mallow. Lizenz: AGPL-3.0-or-later, siehe LICENSE.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // Zeitzonen in der Binary, Selbsthosten ohne tzdata-Paket

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

//go:embed web
var webFS embed.FS

type config struct {
	Port         string
	BaseDomain   string // Salons laufen unter <slug>.<BaseDomain>
	DBPath       string
	SingleTenant bool   // Selbsthosten: genau ein Salon, keine Subdomains
	PublicURL    string // für Links in Mails, "{slug}" wird ersetzt
}

type app struct {
	cfg  config
	db   *sql.DB
	tpl  *template.Template
	mail mailer
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadConfig() config {
	c := config{
		Port:         env("PORT", "8080"),
		BaseDomain:   strings.ToLower(env("BASE_DOMAIN", "localhost")),
		DBPath:       env("DB_PATH", "kammzeit.db"),
		SingleTenant: os.Getenv("KAMMZEIT_SINGLE_TENANT") == "1",
	}
	def := "https://{slug}." + c.BaseDomain
	if c.SingleTenant {
		def = "http://localhost:" + c.Port
	}
	c.PublicURL = strings.TrimRight(env("PUBLIC_URL", def), "/")
	return c
}

func main() {
	cfg := loadConfig()
	db, err := openDB(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 && os.Args[1] == "demo" {
		if err := seedDemo(db); err != nil {
			log.Fatal(err)
		}
		return
	}
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		log.Fatal(err)
	}
	a := &app{cfg: cfg, db: db, tpl: tpl, mail: mailerFromEnv()}
	go a.reminderLoop(context.Background())

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	log.Printf("Kammzeit auf :%s (Basis-Domain %s, Einzelbetrieb %v)", cfg.Port, cfg.BaseDomain, cfg.SingleTenant)
	log.Fatal(srv.ListenAndServe())
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// ponytail: eine Verbindung serialisiert alle Zugriffe, damit gibt es kein
	// Doppelbuchungs-Rennen zwischen Prüfen und Eintragen. Bei Last: Lese-Pool
	// plus eine Schreibverbindung mit BEGIN IMMEDIATE.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return db, nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// tenantSlug zerlegt den Host-Header. root = die Basis-Domain selbst (Landingpage).
func tenantSlug(host, base string) (slug string, root bool) {
	host = strings.ToLower(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == base {
		return "", true
	}
	slug, ok := strings.CutSuffix(host, "."+base)
	if !ok || !slugRe.MatchString(slug) {
		return "", false
	}
	return slug, false
}

// link baut eine absolute URL für Mails (ohne Request, z. B. aus dem Erinnerungs-Loop).
func (a *app) link(slug, path string) string {
	return strings.ReplaceAll(a.cfg.PublicURL, "{slug}", slug) + path
}

func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand fällt unter Linux nicht aus
	}
	return hex.EncodeToString(b)
}

// --- Mail ---

type mailer struct{ host, port, user, pass, from string }

func mailerFromEnv() mailer {
	return mailer{
		host: os.Getenv("SMTP_HOST"),
		port: env("SMTP_PORT", "587"),
		user: os.Getenv("SMTP_USER"),
		pass: os.Getenv("SMTP_PASS"),
		from: env("SMTP_FROM", "Kammzeit <noreply@localhost>"),
	}
}

// send schickt eine Textmail. Ohne SMTP_HOST wird nur geloggt (Entwicklung).
// `to` muss vorher durch mail.ParseAddress gegangen sein.
func (m mailer) send(to, subject, body string) error {
	if m.host == "" {
		log.Printf("Mail (kein SMTP_HOST) an %s: %s\n%s", to, subject, body)
		return nil
	}
	from, err := mail.ParseAddress(m.from)
	if err != nil {
		return fmt.Errorf("SMTP_FROM: %w", err)
	}
	domain := from.Address[strings.LastIndexByte(from.Address, '@')+1:]
	msg := "From: " + m.from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" + // kodiert auch CR/LF, kein Header-Einschleusen
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\n" +
		"Message-ID: <" + randomToken() + "@" + domain + ">\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		strings.ReplaceAll(body, "\n", "\r\n")
	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.pass, m.host)
	}
	return smtp.SendMail(net.JoinHostPort(m.host, m.port), auth, from.Address, []string{to}, []byte(msg))
}

// --- Erinnerungen ---

func (a *app) reminderLoop(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		a.sendReminders(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// sendReminders mailt einmalig 24 h vor jedem gebuchten Termin.
func (a *app) sendReminders(now time.Time) {
	type due struct {
		id                                int64
		start                             time.Time
		token, cust, email, svc, stf, sal string
		slug, tz                          string
	}
	rows, err := a.db.Query(`
		SELECT a.id, a.starts_at, a.cancel_token, c.name, c.email, sv.name, st.name, sa.name, sa.slug, sa.timezone
		FROM appointments a
		JOIN customers c ON c.id = a.customer_id
		JOIN services sv ON sv.id = a.service_id
		JOIN staff st ON st.id = a.staff_id
		JOIN salons sa ON sa.id = a.salon_id
		WHERE a.status = 'booked' AND a.reminder_sent_at IS NULL AND a.starts_at > ? AND a.starts_at <= ?`,
		now.Unix(), now.Add(24*time.Hour).Unix())
	if err != nil {
		log.Printf("Erinnerungen: %v", err)
		return
	}
	var list []due
	for rows.Next() {
		var d due
		var start int64
		if err := rows.Scan(&d.id, &start, &d.token, &d.cust, &d.email, &d.svc, &d.stf, &d.sal, &d.slug, &d.tz); err != nil {
			log.Printf("Erinnerungen: %v", err)
			continue
		}
		d.start = time.Unix(start, 0).In(location(d.tz))
		list = append(list, d)
	}
	rows.Close() // vor den Updates, es gibt nur eine Verbindung
	for _, d := range list {
		body := fmt.Sprintf("Hallo %s,\n\nwir erinnern an deinen Termin bei %s:\n\n%s\n%s bei %s\n\nAbsagen: %s\n",
			d.cust, d.sal, deDate(d.start), d.svc, d.stf, a.link(d.slug, "/storno/"+d.token))
		if err := a.mail.send(d.email, "Erinnerung: dein Termin bei "+d.sal, body); err != nil {
			log.Printf("Erinnerung %d: %v", d.id, err) // nächster Versuch in einer Minute
			continue
		}
		if _, err := a.db.Exec(`UPDATE appointments SET reminder_sent_at = ? WHERE id = ?`, now.Unix(), d.id); err != nil {
			log.Printf("Erinnerung %d: %v", d.id, err)
		}
	}
}

func location(tz string) *time.Location {
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	loc, _ := time.LoadLocation("Europe/Berlin")
	return loc
}

// --- Demo-Daten ---

// seedDemo legt den Salon "demo" an (Aufruf: kammzeit demo). Mehrfach aufrufbar.
// ponytail: bis zum Admin-Bereich ist das der Weg, einen Salon anzulegen.
func seedDemo(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM salons WHERE slug = 'demo'`).Scan(&n); err != nil || n > 0 {
		if err == nil {
			log.Print("Salon demo existiert schon")
		}
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO salons (slug, name) VALUES ('demo', 'Salon Demo')`)
	if err != nil {
		return err
	}
	salonID, _ := res.LastInsertId()
	for _, s := range []struct {
		name       string
		min, cents int
	}{{"Haarschnitt Damen", 60, 4500}, {"Haarschnitt Herren", 30, 2500}, {"Färben", 120, 7900}} {
		if _, err := tx.Exec(`INSERT INTO services (salon_id, name, duration_min, price_cents) VALUES (?, ?, ?, ?)`, salonID, s.name, s.min, s.cents); err != nil {
			return err
		}
	}
	for _, name := range []string{"Anna", "Ben"} {
		res, err := tx.Exec(`INSERT INTO staff (salon_id, name) VALUES (?, ?)`, salonID, name)
		if err != nil {
			return err
		}
		staffID, _ := res.LastInsertId()
		blocks := [][3]int{{6, 9 * 60, 14 * 60}} // Samstag
		for wd := 1; wd <= 5; wd++ {
			blocks = append(blocks, [3]int{wd, 9 * 60, 12 * 60}, [3]int{wd, 13 * 60, 18 * 60})
		}
		for _, b := range blocks {
			if _, err := tx.Exec(`INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, ?, ?)`, staffID, b[0], b[1], b[2]); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Print("Salon demo angelegt")
	return nil
}
