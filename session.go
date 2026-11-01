package main

// Sessions und der Rahmen für alle Dashboard-Seiten. Login selbst (Magic-Link)
// steht in auth.go.

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"
)

const (
	sessionCookie = "kz_session"
	sessionTTL    = 30 * 24 * time.Hour
)

type user struct {
	ID, SalonID int64
	Email       string
	Role        string // owner | staff (leer, z. B. in Tests, zählt als owner)
	StaffID     int64  // nur bei staff: der eigene Mitarbeiter (0 = gelöscht)
	StaffName   string // nur bei staff, lädt a.dash für den Kopf
}

// dctx = alles, was ein Dashboard-Handler braucht.
type dctx struct {
	User  *user
	Salon *salon
}

// Owner: Inhaber dürfen alles, Mitarbeiter-Logins (role staff, nur Pro) nur
// Kalender, Kunden (ohne Löschen und Export), eigene Arbeitszeiten und Warteschlange.
func (c *dctx) Owner() bool { return c.User.Role != "staff" }

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// siteURL ist die Adresse der Hauptseite (Landing, Login, Dashboard), ohne Slash am Ende.
func (a *app) siteURL() string { return a.cfg.SiteURL }

// onRootHost: Dashboard, Login und Registrierung laufen nur auf der Hauptdomain
// (im Einzelbetrieb überall).
func (a *app) onRootHost(r *http.Request) bool {
	if a.cfg.SingleTenant {
		return true
	}
	_, root, _ := a.tenant(r.Host)
	return root
}

func secureRequest(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// startSession meldet einen Nutzer an (nach eingelöstem Magic-Link).
func (a *app) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token := randomToken()
	exp := time.Now().Add(sessionTTL)
	if _, err := a.db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), userID, exp.Unix()); err != nil {
		return err
	}
	a.db.Exec(`UPDATE users SET last_login_at = unixepoch() WHERE id = ?`, userID)
	a.db.Exec(`DELETE FROM sessions WHERE expires_at < unixepoch()`)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: exp,
		HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (a *app) endSession(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
}

// currentUser liefert den angemeldeten Nutzer oder nil.
func (a *app) currentUser(r *http.Request) (*user, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, nil
	}
	var u user
	err = a.db.QueryRow(`SELECT u.id, u.salon_id, u.email, u.role, COALESCE(u.staff_id, 0) FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > unixepoch()`, hashToken(c.Value)).Scan(&u.ID, &u.SalonID, &u.Email, &u.Role, &u.StaffID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// sameOrigin schützt alle ändernden Dashboard-Anfragen gegen CSRF: Origin
// (oder ersatzweise Referer) muss der eigene Host sein. Zusätzlich SameSite=Lax.
func sameOrigin(r *http.Request) bool {
	src := r.Header.Get("Origin")
	if src == "" {
		src = r.Header.Get("Referer")
	}
	if src == "" {
		return false
	}
	u, err := url.Parse(src)
	return err == nil && u.Host == r.Host
}

// dash schützt einen Dashboard-Handler: nur Hauptdomain, nur angemeldet, bei
// POST/PUT/DELETE nur gleiche Herkunft. Lädt Nutzer und Salon.
func (a *app) dash(h func(http.ResponseWriter, *http.Request, *dctx)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.onRootHost(r) {
			http.Redirect(w, r, a.siteURLFor(r)+r.URL.RequestURI(), http.StatusFound)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOrigin(r) {
			http.Error(w, "Ungültige Herkunft der Anfrage.", http.StatusForbidden)
			return
		}
		u, err := a.currentUser(r)
		if err != nil {
			a.serverError(w, nil, err)
			return
		}
		if u == nil {
			if r.Header.Get("HX-Request") != "" {
				w.Header().Set("HX-Redirect", "/login")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login?weiter="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		s, err := a.loadSalon(u.SalonID)
		if err != nil {
			a.serverError(w, nil, err)
			return
		}
		if u.Role == "staff" {
			// Mitarbeiter-Logins nur mit Pro und solange der Mitarbeiter aktiv ist.
			// Sonst abmelden und erklären (staff.go), gelöscht wird nichts.
			var active bool
			err := a.db.QueryRow(`SELECT name, active FROM staff WHERE id = ? AND salon_id = ?`, u.StaffID, s.ID).Scan(&u.StaffName, &active)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				a.serverError(w, nil, err)
				return
			}
			if !active || !a.staffLogins(s) {
				a.endSession(w, r)
				if r.Header.Get("HX-Request") != "" {
					w.Header().Set("HX-Redirect", "/login?pausiert")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				http.Redirect(w, r, "/login?pausiert", http.StatusFound)
				return
			}
		}
		h(w, r, &dctx{User: u, Salon: s})
	}
}

// dashOwner ist a.dash nur für Inhaber: Mitarbeiter-Logins bekommen 403 "Nur für Inhaber".
// Alles, was ein Mitarbeiter darf (Kalender, Kunden ansehen und bearbeiten,
// Warteschlange), bleibt bei a.dash.
func (a *app) dashOwner(h func(http.ResponseWriter, *http.Request, *dctx)) http.HandlerFunc {
	return a.dash(func(w http.ResponseWriter, r *http.Request, c *dctx) {
		if c.Owner() {
			h(w, r, c)
			return
		}
		d := a.dashPage(c, r, "", "Nur für Inhaber")
		d["Icon"] = "schloss"
		a.render(w, http.StatusForbidden, "staff-verboten", d)
	})
}

// dashIcons: buntes 48px-Icon (web/static/img/icons) im Seitenkopf je Menüpunkt.
var dashIcons = map[string]string{"kalender": "kalender", "kunden": "kartei", "leistungen": "schere",
	"team": "team", "einstellungen": "farbe", "abo": "pro", "meine-zeiten": "uhr", "": "hilfe"}

// dashPage füllt die Felder für das Dashboard-Layout (dash-top/dash-bottom).
// nav = aktiver Menüpunkt: kalender, kunden, leistungen, team, einstellungen, abo, meine-zeiten.
// IsStaff = Mitarbeiter-Login: Layout und Seiten blenden dann Inhaber-Sachen aus
// (fehlt der Schlüssel, z. B. bei eigenen Daten, gilt die Inhaber-Ansicht).
// Nach ?gespeichert zeigt das Layout d["SavedMsg"] (vom Handler gesetzt) oder "Gespeichert.".
// Seitenkopf: Heading + Icon, optional d["Lead"] (ein Satz Erklärung unter der Überschrift).
// title ist der deutsche Text (Schlüssel), übersetzt wird hier; Lead und SavedMsg übersetzt das Layout.
func (a *app) dashPage(c *dctx, r *http.Request, nav, title string) map[string]any {
	lang := a.langFor(r)
	title = tr(lang, title)
	return map[string]any{
		"Title":      title + " - Kammzeit",
		"Heading":    title,
		"Icon":       dashIcons[nav],
		"Nav":        nav,
		"Salon":      c.Salon,
		"User":       c.User,
		"Accent":     c.Salon.Accent,
		"OnAccent":   onAccent(c.Salon.Accent),
		"BookingURL": a.bookingURL(c.Salon, "/"),
		"Saved":      r.URL.Query().Has("gespeichert"),
		"Dashboard":  true,
		"Lang":       lang,
		"IsStaff":    !c.Owner(),
	}
}

// dashError zeigt einen Fehler im Dashboard-Layout. msg = deutscher Text (wird übersetzt).
func (a *app) dashError(w http.ResponseWriter, r *http.Request, c *dctx, status int, msg string) {
	d := a.dashPage(c, r, "", "Hinweis")
	d["Message"] = tr(d["Lang"].(string), msg)
	a.render(w, status, "dash-error", d)
}

func (a *app) dashServerError(w http.ResponseWriter, r *http.Request, c *dctx, err error) {
	log.Print(err)
	a.dashError(w, r, c, http.StatusInternalServerError, "Da ist etwas schiefgelaufen. Bitte später noch einmal versuchen.")
}
