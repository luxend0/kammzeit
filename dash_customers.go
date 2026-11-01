package main

// Kundenkartei: /dashboard/kunden/...
// Alle Bezeichner dieses Bereichs beginnen mit kunde/kunden (ein Paket für alle Bereiche).

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const kundenProSeite = 50

func (a *app) routesCustomers(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard/kunden", a.dash(a.kundenListe))
	mux.HandleFunc("GET /dashboard/kunden.csv", a.dash(a.kundenCSV))
	mux.HandleFunc("GET /dashboard/kunden/neu", a.dash(a.kundeNeu))
	mux.HandleFunc("POST /dashboard/kunden/neu", a.dash(a.kundeAnlegen))
	mux.HandleFunc("GET /dashboard/kunden/{id}", a.dash(a.kundeKarte))
	mux.HandleFunc("POST /dashboard/kunden/{id}", a.dash(a.kundeSpeichern))
	mux.HandleFunc("GET /dashboard/kunden/{id}/loeschen", a.dash(a.kundeLoeschenSeite))
	mux.HandleFunc("POST /dashboard/kunden/{id}/loeschen", a.dash(a.kundeLoeschen))
}

// kunde = eine Karteikarte. Email leer = keine Adresse (in der DB NULL).
type kunde struct {
	ID                        int64
	Name, Email, Phone, Notes string
	Created                   string // fertig formatiert
}

// --- Liste und Suche ---

type kundenZeile struct {
	ID                  int64
	Name, Email, Phone  string
	Visits              int
	LastVisit, NextAppt string // leer = keiner
}

// kundenSuche liefert eine Seite der Kundenliste, gefiltert nach Name, E-Mail
// oder Telefon. Gibt bis zu kundenProSeite+1 Zeilen zurück (die letzte zeigt
// nur an, dass es eine weitere Seite gibt). Besuche = vergangene gebuchte Termine.
// ponytail: LIKE ist nur für ASCII unabhängig von Groß-/Kleinschreibung ("müller" findet "Müller" nicht).
func (a *app) kundenSuche(s *salon, lang, q string, page int) ([]kundenZeile, error) {
	like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	rows, err := a.db.Query(`
		SELECT c.id, c.name, COALESCE(c.email, ''), c.phone,
			(SELECT COUNT(*) FROM appointments a WHERE a.customer_id = c.id AND a.salon_id = c.salon_id AND a.status = 'booked' AND a.starts_at < ?1),
			(SELECT MAX(a.starts_at) FROM appointments a WHERE a.customer_id = c.id AND a.salon_id = c.salon_id AND a.status = 'booked' AND a.starts_at < ?1),
			(SELECT MIN(a.starts_at) FROM appointments a WHERE a.customer_id = c.id AND a.salon_id = c.salon_id AND a.status = 'booked' AND a.starts_at >= ?1)
		FROM customers c
		WHERE c.salon_id = ?2 AND (c.name LIKE ?3 ESCAPE '\' OR c.email LIKE ?3 ESCAPE '\' OR c.phone LIKE ?3 ESCAPE '\')
		ORDER BY c.name COLLATE NOCASE, c.id
		LIMIT ?4 OFFSET ?5`,
		time.Now().Unix(), s.ID, like, kundenProSeite+1, (page-1)*kundenProSeite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kundenZeile
	for rows.Next() {
		var k kundenZeile
		var last, next sql.NullInt64
		if err := rows.Scan(&k.ID, &k.Name, &k.Email, &k.Phone, &k.Visits, &last, &next); err != nil {
			return nil, err
		}
		if last.Valid {
			k.LastVisit = kundenDatum(lang, time.Unix(last.Int64, 0).In(s.Loc))
		}
		if next.Valid {
			t := time.Unix(next.Int64, 0).In(s.Loc)
			k.NextAppt = kundenDatum(lang, t) + ", " + t.Format("15:04")
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// kundenDatum: Datum ohne Uhrzeit, "03.11.2026" bzw. "3 Nov 2026".
func kundenDatum(lang string, t time.Time) string {
	if lang == "en" {
		return fmt.Sprintf("%d %s %d", t.Day(), monthsEN[t.Month()], t.Year())
	}
	return t.Format("02.01.2006")
}

func (a *app) kundenListe(w http.ResponseWriter, r *http.Request, c *dctx) {
	lang := a.langFor(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(q) > 100 {
		q = string([]rune(q)[:100])
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("seite"))
	page = max(1, min(page, 100000))
	list, err := a.kundenSuche(c.Salon, lang, q, page)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	more := len(list) > kundenProSeite
	if more {
		list = list[:kundenProSeite]
	}
	link := func(p int) string {
		v := url.Values{}
		if q != "" {
			v.Set("q", q)
		}
		if p > 1 {
			v.Set("seite", strconv.Itoa(p))
		}
		if len(v) == 0 {
			return "/dashboard/kunden"
		}
		return "/dashboard/kunden?" + v.Encode()
	}
	d := a.dashPage(c, r, "kunden", tr(lang, "Kunden"))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Q"], d["Customers"], d["Deleted"] = q, list, r.URL.Query().Has("geloescht")
	if page > 1 {
		d["PrevURL"] = link(page - 1)
	}
	if more {
		d["NextURL"] = link(page + 1)
	}
	// Die Live-Suche (htmx) ersetzt nur die Tabelle.
	w.Header().Set("Vary", "HX-Target")
	if r.Header.Get("HX-Target") == "kunden-tabelle" {
		a.render(w, http.StatusOK, "kunden-tabelle", d)
		return
	}
	a.render(w, http.StatusOK, "kunden-liste", d)
}

// --- CSV-Export ---

// kundenCSVZelle verhindert Formel-Injection in Excel/LibreOffice: Zellen, die
// mit = + - @ (oder Tab/CR) beginnen, bekommen ein ' vorangestellt.
func kundenCSVZelle(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (a *app) kundenCSV(w http.ResponseWriter, r *http.Request, c *dctx) {
	lang := a.langFor(r)
	rows, err := a.db.Query(`
		SELECT c.name, COALESCE(c.email, ''), c.phone, c.notes, c.created_at,
			(SELECT COUNT(*) FROM appointments a WHERE a.customer_id = c.id AND a.salon_id = c.salon_id AND a.status = 'booked' AND a.starts_at < ?)
		FROM customers c WHERE c.salon_id = ? ORDER BY c.name COLLATE NOCASE, c.id`, time.Now().Unix(), c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	// ponytail: erst alles lesen, dann schreiben. Ein langsamer Download hielte
	// sonst die einzige DB-Verbindung fest. Kundenzahlen eines Salons passen in den Speicher.
	head := []string{"Name", "E-Mail", "Telefon", "Notizen", "Angelegt", "Besuche"}
	for i := range head {
		head[i] = tr(lang, head[i])
	}
	created := func(t time.Time) string { return t.Format("02.01.2006") }
	if lang == "en" {
		created = func(t time.Time) string { return t.Format(time.DateOnly) } // ISO: in jedem Gebietsschema eindeutig
	}
	records := [][]string{head}
	for rows.Next() {
		var name, email, phone, notes string
		var createdAt int64
		var visits int
		if err := rows.Scan(&name, &email, &phone, &notes, &createdAt, &visits); err != nil {
			rows.Close()
			a.dashServerError(w, r, c, err)
			return
		}
		records = append(records, []string{kundenCSVZelle(name), kundenCSVZelle(email), kundenCSVZelle(phone),
			kundenCSVZelle(notes), created(time.Unix(createdAt, 0).In(c.Salon.Loc)), strconv.Itoa(visits)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	file := tr(lang, "kunden") + "-" + c.Salon.Slug + "-" + time.Now().In(c.Salon.Loc).Format(time.DateOnly) + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+file+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte("\xEF\xBB\xBF")) // BOM, damit Excel UTF-8 erkennt
	cw := csv.NewWriter(w)
	cw.Comma = ';' // Excel mit deutschem (und den meisten EU-)Gebietsschemas erwartet Semikolon
	cw.UseCRLF = true
	cw.WriteAll(records)
}

// --- Anlegen und Bearbeiten ---

const kundeEmailVergebenText = "Diese E-Mail-Adresse gehört schon zu einem anderen Kunden. Bitte eine andere angeben oder das Feld leer lassen."

// kundenGespeichert: Erfolgsmeldung je ?gespeichert=<code>. Nur feste Texte, nie Text aus der URL.
var kundenGespeichert = map[string]string{
	"neu":   "Kunde angelegt.",
	"karte": "Kontakt und Notizen gespeichert.",
}

// kundeAusFormular liest und prüft das Kundenformular. Fehler je Feldname
// (name, email, phone, notes); "form" = Formular nicht lesbar. Leer = gültig.
func kundeAusFormular(w http.ResponseWriter, r *http.Request, lang string) (kunde, map[string]string) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		return kunde{}, map[string]string{"form": tr(lang, "Das Formular war zu groß oder unvollständig.")}
	}
	k := kunde{
		Name:  strings.TrimSpace(r.PostFormValue("name")),
		Email: strings.TrimSpace(r.PostFormValue("email")),
		Phone: strings.TrimSpace(r.PostFormValue("phone")),
		Notes: strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("notes"), "\r\n", "\n")),
	}
	errs := map[string]string{}
	switch {
	case k.Name == "":
		errs["name"] = "Bitte einen Namen angeben."
	case utf8.RuneCountInString(k.Name) > 100:
		errs["name"] = "Der Name ist zu lang (höchstens 100 Zeichen)."
	case strings.ContainsFunc(k.Name, unicode.IsControl):
		errs["name"] = "Der Name darf keine Zeilenumbrüche oder Steuerzeichen enthalten."
	}
	if utf8.RuneCountInString(k.Phone) > 40 {
		errs["phone"] = "Die Telefonnummer ist zu lang (höchstens 40 Zeichen)."
	}
	if utf8.RuneCountInString(k.Notes) > 2000 {
		errs["notes"] = "Die Notizen sind zu lang (höchstens 2000 Zeichen)."
	}
	if k.Email != "" {
		if addr, err := mail.ParseAddress(k.Email); err != nil || len(addr.Address) > 200 {
			errs["email"] = "Bitte eine gültige E-Mail-Adresse angeben oder das Feld leer lassen."
		} else {
			k.Email = strings.ToLower(addr.Address) // wie bei der Online-Buchung
		}
	}
	for f, msg := range errs {
		errs[f] = tr(lang, msg)
	}
	return k, errs
}

// kundeNullEmail: leere Adresse als NULL speichern (mehrere NULL sind erlaubt).
func kundeNullEmail(e string) any {
	if e == "" {
		return nil
	}
	return e
}

// kundeEmailVergeben erkennt den Verstoß gegen UNIQUE (salon_id, email).
// Geprüft wird beim Schreiben selbst, nicht vorher, damit es kein Rennen gibt.
func kundeEmailVergeben(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: customers.salon_id, customers.email")
}

func (a *app) kundeNeu(w http.ResponseWriter, r *http.Request, c *dctx) {
	d := a.dashPage(c, r, "kunden", tr(a.langFor(r), "Neuer Kunde"))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Form"], d["Errors"] = kunde{}, map[string]string{}
	a.render(w, http.StatusOK, "kunden-neu", d)
}

func (a *app) kundeAnlegen(w http.ResponseWriter, r *http.Request, c *dctx) {
	lang := a.langFor(r)
	k, errs := kundeAusFormular(w, r, lang)
	if len(errs) == 0 {
		var id int64
		err := a.db.QueryRow(`INSERT INTO customers (salon_id, name, email, phone, notes) VALUES (?, ?, ?, ?, ?) RETURNING id`,
			c.Salon.ID, k.Name, kundeNullEmail(k.Email), k.Phone, k.Notes).Scan(&id)
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/dashboard/kunden/%d?gespeichert=neu", id), http.StatusSeeOther)
			return
		}
		if !kundeEmailVergeben(err) {
			a.dashServerError(w, r, c, err)
			return
		}
		errs["email"] = tr(lang, kundeEmailVergebenText)
	}
	d := a.dashPage(c, r, "kunden", tr(lang, "Neuer Kunde"))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Form"], d["Errors"] = k, errs
	a.render(w, http.StatusBadRequest, "kunden-neu", d)
}

// eigenerKunde lädt den Kunden aus der URL, nur aus dem eigenen Salon. Sonst
// antwortet es selbst (404 bzw. 500) und gibt nil zurück.
func (a *app) eigenerKunde(w http.ResponseWriter, r *http.Request, c *dctx) *kunde {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var k kunde
	var created int64
	if err == nil {
		err = a.db.QueryRow(`SELECT id, name, COALESCE(email, ''), phone, notes, created_at FROM customers WHERE id = ? AND salon_id = ?`,
			id, c.Salon.ID).Scan(&k.ID, &k.Name, &k.Email, &k.Phone, &k.Notes, &created)
	}
	var numErr *strconv.NumError
	switch {
	case errors.As(err, &numErr) || errors.Is(err, sql.ErrNoRows):
		a.dashError(w, r, c, http.StatusNotFound, tr(a.langFor(r), "Diesen Kunden gibt es nicht."))
		return nil
	case err != nil:
		a.dashServerError(w, r, c, err)
		return nil
	}
	k.Created = kundenDatum(a.langFor(r), time.Unix(created, 0).In(c.Salon.Loc))
	return &k
}

func (a *app) kundeKarte(w http.ResponseWriter, r *http.Request, c *dctx) {
	if k := a.eigenerKunde(w, r, c); k != nil {
		a.kundeKarteZeigen(w, r, c, k, *k, nil)
	}
}

func (a *app) kundeSpeichern(w http.ResponseWriter, r *http.Request, c *dctx) {
	k := a.eigenerKunde(w, r, c)
	if k == nil {
		return
	}
	lang := a.langFor(r)
	f, errs := kundeAusFormular(w, r, lang)
	if len(errs) == 0 {
		_, err := a.db.Exec(`UPDATE customers SET name = ?, email = ?, phone = ?, notes = ? WHERE id = ? AND salon_id = ?`,
			f.Name, kundeNullEmail(f.Email), f.Phone, f.Notes, k.ID, c.Salon.ID)
		if err == nil {
			http.Redirect(w, r, fmt.Sprintf("/dashboard/kunden/%d?gespeichert=karte", k.ID), http.StatusSeeOther)
			return
		}
		if !kundeEmailVergeben(err) {
			a.dashServerError(w, r, c, err)
			return
		}
		errs["email"] = tr(lang, kundeEmailVergebenText)
	}
	a.kundeKarteZeigen(w, r, c, k, f, errs)
}

// --- Karteikarte ---

type kundenTermin struct {
	ID, PriceCents       int64
	When, Service, Staff string
	Status, Badge        string // Status = deutscher Schlüssel (für die Chip-Farbe)
	Label, Price         string // Status und Preis in der Sprache der Seite
}

// kundenTermine: Terminliste mit Sprache für das Template "kunden-termine".
type kundenTermine struct {
	Lang string
	List []kundenTermin
}

// kundeKarteZeigen rendert die Karteikarte mit Terminhistorie. form = Werte im
// Bearbeiten-Formular (nach einem Fehler die eingegebenen), errs = Fehler je Feld.
// ponytail: Historie ohne Seitenumbruch; auch Stammkunden haben selten mehr als ein paar hundert Termine.
func (a *app) kundeKarteZeigen(w http.ResponseWriter, r *http.Request, c *dctx, k *kunde, form kunde, errs map[string]string) {
	rows, err := a.db.Query(`SELECT a.id, a.starts_at, a.status, sv.name, sv.price_cents, st.name FROM appointments a
		JOIN services sv ON sv.id = a.service_id JOIN staff st ON st.id = a.staff_id
		WHERE a.customer_id = ? AND a.salon_id = ? ORDER BY a.starts_at, a.id`, k.ID, c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	lang := a.langFor(r)
	now := time.Now().Unix()
	var upcoming, past []kundenTermin
	var visits int
	var revenue int64
	for rows.Next() {
		var t kundenTermin
		var start int64
		var status string
		if err := rows.Scan(&t.ID, &start, &status, &t.Service, &t.PriceCents, &t.Staff); err != nil {
			rows.Close()
			a.dashServerError(w, r, c, err)
			return
		}
		t.When = dateFor(lang, time.Unix(start, 0).In(c.Salon.Loc))
		t.Price = money(lang, t.PriceCents)
		switch {
		case status == "cancelled":
			t.Status, t.Badge = "abgesagt", "badge-muted"
		case status == "noshow":
			t.Status, t.Badge = "nicht erschienen", "badge-danger"
		case start >= now:
			t.Status, t.Badge = "gebucht", "badge-accent"
		default:
			t.Status = "erledigt"
			visits++
			revenue += t.PriceCents // ponytail: aktueller Preis der Leistung, keine Preis-Historie
		}
		t.Label = tr(lang, t.Status)
		if start >= now {
			upcoming = append(upcoming, t)
		} else {
			past = append(past, t)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	slices.Reverse(past) // neueste zuerst
	next := ""
	for _, t := range upcoming {
		if t.Status == "gebucht" {
			next = t.When
			break
		}
	}
	d := a.dashPage(c, r, "kunden", k.Name)
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Customer"], d["Form"], d["Errors"] = k, form, errs
	d["Upcoming"], d["Past"] = kundenTermine{lang, upcoming}, kundenTermine{lang, past}
	d["Visits"], d["Revenue"], d["Next"] = visits, money(lang, revenue), next
	if msg := kundenGespeichert[r.URL.Query().Get("gespeichert")]; msg != "" {
		d["SavedMsg"] = tr(lang, msg)
	}
	status := http.StatusOK
	if len(errs) > 0 {
		status = http.StatusBadRequest
	}
	a.render(w, status, "kunden-karte", d)
}

// --- Löschen (DSGVO) ---

func (a *app) kundeLoeschenSeite(w http.ResponseWriter, r *http.Request, c *dctx) {
	k := a.eigenerKunde(w, r, c)
	if k == nil {
		return
	}
	var total, upcoming int
	err := a.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(status = 'booked' AND starts_at >= ?), 0) FROM appointments
		WHERE customer_id = ? AND salon_id = ?`, time.Now().Unix(), k.ID, c.Salon.ID).Scan(&total, &upcoming)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	lang := a.langFor(r)
	appts := tr(lang, "keine Termine")
	switch {
	case total == 1:
		appts = tr(lang, "1 Termin")
	case total > 1:
		appts = strconv.Itoa(total) + tr(lang, " Termine")
	}
	d := a.dashPage(c, r, "kunden", tr(lang, "Kunde löschen"))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Customer"], d["Appts"], d["Upcoming"] = k, appts, upcoming
	a.render(w, http.StatusOK, "kunden-loeschen", d)
}

// kundeLoeschen löscht Karteikarte samt Terminen (ON DELETE CASCADE).
func (a *app) kundeLoeschen(w http.ResponseWriter, r *http.Request, c *dctx) {
	k := a.eigenerKunde(w, r, c)
	if k == nil {
		return
	}
	if _, err := a.db.Exec(`DELETE FROM customers WHERE id = ? AND salon_id = ?`, k.ID, c.Salon.ID); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/kunden?geloescht", http.StatusSeeOther)
}
