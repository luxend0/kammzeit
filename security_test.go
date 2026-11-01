package main

// Nachweise zu den Befunden aus dem Sicherheitsreview (November 2026).

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// doFrom schickt eine Anfrage von einer bestimmten Gegenstelle (RemoteAddr).
func (ts *testSalon) doFrom(remote, method, host, path string, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(form.Encode()))
	r.RemoteAddr = remote
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

// H1: Name und Telefon landen in Mails an Kunde und Salon. Steuerzeichen
// (CR, LF, Tab, NUL, C1) werden abgelehnt, nichts wird gespeichert.
func TestBookRejectsControlChars(t *testing.T) {
	ts := newTestSalon(t)
	s, _ := ts.a.loadSalon(ts.salon)
	start := dayAfterTomorrow10(s.Loc)
	for _, c := range []struct{ field, value string }{
		{"name", "Eva\r\nBcc: opfer@example.com"},
		{"name", "Eva\r.\rMAIL FROM:<x@example.com>"},
		{"name", "Eva\tMüller"},
		{"name", "Eva\u0085Müller"},
		{"phone", "0123\n456"},
		{"phone", "0123\x00"},
	} {
		f := bookForm(ts.schnitt, 0, start)
		f.Set(c.field, c.value)
		w := ts.do("POST", "test.kammzeit.test", "/book", f)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "ohne Zeilenumbrüche") {
			t.Errorf("%s=%q: %d", c.field, c.value, w.Code)
		}
	}
	if n := ts.count(t, `SELECT (SELECT COUNT(*) FROM appointments) + (SELECT COUNT(*) FROM customers WHERE email = 'eva@example.com')`); n != 0 {
		t.Fatalf("%d Zeilen trotz Steuerzeichen gespeichert", n)
	}
	// Umlaute, Bindestrich, Plus und Leerzeichen bleiben erlaubt.
	f := bookForm(ts.schnitt, 0, start)
	f.Set("name", "Jörg-Ünal Groß")
	f.Set("phone", "+49 6761 123-45")
	if w := ts.do("POST", "test.kammzeit.test", "/book", f); w.Code != http.StatusOK {
		t.Fatalf("normaler Name: %d", w.Code)
	}
}

// H1: Wer ein IPv6-/64 hat, bekommt mit neuen Adressen kein neues Kontingent.
func TestBookLimitIPv6Prefix(t *testing.T) {
	ts := newTestSalon(t)
	f := url.Values{"name": {""}}
	for i := range 10 {
		ts.doFrom(fmt.Sprintf("[2001:db8:1:2::%x]:4000", i+1), "POST", "test.kammzeit.test", "/book", f)
	}
	if w := ts.doFrom("[2001:db8:1:2:ffff:ffff:ffff:ffff]:4000", "POST", "test.kammzeit.test", "/book", f); w.Code != http.StatusTooManyRequests {
		t.Fatalf("neue Adresse im selben /64: %d", w.Code)
	}
	if w := ts.doFrom("[2001:db8:1:3::1]:4000", "POST", "test.kammzeit.test", "/book", f); w.Code == http.StatusTooManyRequests {
		t.Fatal("anderes /64 mitgesperrt")
	}
}

// H1: Auch mit wechselnden Anschlüssen höchstens 3 Buchungen pro Empfänger-Mail
// und 30 pro Salon und Stunde. Vergebene Uhrzeiten zählen nicht mit.
func TestBookLimitPerMailAndSalon(t *testing.T) {
	ts := newTestSalon(t)
	log.SetOutput(io.Discard) // 30 Bestätigungsmails im Log
	defer log.SetOutput(os.Stderr)
	s, _ := ts.a.loadSalon(ts.salon)
	base := dayAfterTomorrow10(s.Loc)
	n := 0 // jede Anfrage von einem neuen Anschluss
	book := func(slot int, email string) int {
		n++
		f := bookForm(ts.schnitt, ts.anna, base.Add(time.Duration(slot)*30*time.Minute))
		f.Set("email", email)
		return ts.doFrom("198.51.100."+strconv.Itoa(n)+":4000", "POST", "test.kammzeit.test", "/book", f).Code
	}
	for slot := range 3 {
		if code := book(slot, "opfer@example.com"); code != http.StatusOK {
			t.Fatalf("Buchung %d: %d", slot+1, code)
		}
	}
	if code := book(0, "opfer@example.com"); code != http.StatusConflict {
		t.Fatalf("vergebene Uhrzeit: %d", code)
	}
	if code := book(3, "Opfer@Example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("4. Buchung an dieselbe Adresse: %d", code)
	}
	for slot := 3; slot < 30; slot++ {
		if code := book(slot, fmt.Sprintf("kunde%d@example.com", slot)); code != http.StatusOK {
			t.Fatalf("Buchung %d: %d", slot+1, code)
		}
	}
	if code := book(30, "neu@example.com"); code != http.StatusTooManyRequests {
		t.Fatalf("31. Buchung im Salon: %d", code)
	}
	if got := ts.count(t, `SELECT COUNT(*) FROM appointments`); got != 30 {
		t.Fatalf("%d Termine, erwartet 30", got)
	}
}

// Nackte CR im Mail-Text (z. B. ein von Hand gepflegter Kundenname in der
// Erinnerung) gehen nicht auf die Leitung.
func TestMailStripsBareCR(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() { // SMTP-Gegenstelle, gerade genug für net/smtp
		var data strings.Builder
		defer func() { got <- data.String() }()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		rd := bufio.NewReader(c)
		fmt.Fprint(c, "220 test\r\n")
		inData := false
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			switch {
			case inData && line == ".\r\n":
				inData = false
				fmt.Fprint(c, "250 ok\r\n")
			case inData:
				data.WriteString(line)
			case strings.HasPrefix(line, "DATA"):
				inData = true
				fmt.Fprint(c, "354 los\r\n")
			case strings.HasPrefix(line, "QUIT"):
				fmt.Fprint(c, "221 tschüss\r\n")
				return
			default:
				fmt.Fprint(c, "250 ok\r\n")
			}
		}
	}()
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	m := mailer{host: host, port: port, from: "Kammzeit <noreply@kammzeit.test>"}
	if err := m.send("kim@example.com", "Test", "Hallo Kim\r.\rMAIL FROM:<x@example.com>\nZeile 2\n"); err != nil {
		t.Fatal(err)
	}
	data := <-got
	if strings.Contains(strings.ReplaceAll(data, "\r\n", ""), "\r") {
		t.Fatalf("nackte CR auf der Leitung: %q", data)
	}
	if !strings.Contains(data, "\r\n\r\nHallo Kim.MAIL FROM:<x@example.com>\r\nZeile 2\r\n") {
		t.Fatalf("Text verändert: %q", data)
	}
}

// N1: Ein voller Limiter lehnt neue Schlüssel ab und räumt nur nach Zeit auf,
// nicht bei jedem Aufruf (früher eine Schleife über alle ab 10000 Schlüsseln).
func TestLimiterCapAndTimedSweep(t *testing.T) {
	l := newLimiter(2, time.Hour)
	now := time.Now()
	for i := range limiterMaxKeys {
		l.hits["k"+strconv.Itoa(i)] = []time.Time{now}
	}
	l.swept = now
	if l.allow("neu") {
		t.Fatal("neuer Schlüssel trotz vollem Limiter")
	}
	if !l.allow("k1") || l.allow("k1") {
		t.Fatal("bekannter Schlüssel zählt nicht normal weiter")
	}
	if !l.swept.Equal(now) || len(l.hits) != limiterMaxKeys {
		t.Fatalf("Aufräumen ohne Zeitablauf (%d Schlüssel)", len(l.hits))
	}
	// Nach dem Zeitfenster räumt der nächste Aufruf auf, dann ist wieder Platz.
	old := now.Add(-2 * time.Hour)
	for k := range l.hits {
		l.hits[k] = []time.Time{old}
	}
	l.swept = old
	if !l.allow("neu") || len(l.hits) != 1 {
		t.Fatalf("nach Ablauf: %d Schlüssel", len(l.hits))
	}
}

// N2 und H1: X-Forwarded-For nur hinter dem Proxy auf localhost, dann nur der
// letzte Eintrag der letzten Zeile und nur, wenn er eine IP ist.
// clientKey fasst IPv6 auf /64 zusammen.
func TestClientIPAndKey(t *testing.T) {
	for _, c := range []struct {
		remote  string
		xff     []string
		ip, key string
	}{
		{"203.0.113.7:5000", nil, "203.0.113.7", "203.0.113.7"},
		{"203.0.113.7:5000", []string{"198.51.100.1"}, "203.0.113.7", "203.0.113.7"}, // kein Proxy: Header egal
		{"127.0.0.1:5000", nil, "127.0.0.1", "127.0.0.1"},
		{"127.0.0.1:5000", []string{"198.51.100.1"}, "198.51.100.1", "198.51.100.1"},
		{"127.0.0.1:5000", []string{"10.0.0.1, 198.51.100.1"}, "198.51.100.1", "198.51.100.1"},
		{"127.0.0.1:5000", []string{"10.0.0.1", "198.51.100.1"}, "198.51.100.1", "198.51.100.1"}, // eigene erste Zeile des Clients
		{"127.0.0.1:5000", []string{"198.51.100.1, kaputt"}, "127.0.0.1", "127.0.0.1"},
		{"127.0.0.1:5000", []string{""}, "127.0.0.1", "127.0.0.1"},
		{"127.0.0.1:5000", []string{"::ffff:198.51.100.1"}, "198.51.100.1", "198.51.100.1"},
		{"[::1]:5000", []string{"2001:db8:1:2:3:4:5:6"}, "2001:db8:1:2:3:4:5:6", "2001:db8:1:2::/64"},
		{"[2001:db8:aa:bb::1]:5000", nil, "2001:db8:aa:bb::1", "2001:db8:aa:bb::/64"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		if ip, key := clientIP(r), clientKey(r); ip != c.ip || key != c.key {
			t.Errorf("%s %q: clientIP %q, clientKey %q; want %q, %q", c.remote, c.xff, ip, key, c.ip, c.key)
		}
	}
}

// N3: SQLite vergibt die ID eines gelöschten Salons neu. Ein altes Abo (oder
// eine alte Checkout-Session), das diese ID nennt, schaltet den neuen Salon nicht frei.
func TestStripeIgnoresReusedSalonID(t *testing.T) {
	setStripeEnv(t, "sk_test_x", "price_m", "price_y")
	f := newFakeStripe(t)
	a := newBillingApp(t) // Salons 1 und 2
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr)
	hourAgo := time.Now().Add(-time.Hour).Unix()
	// Salon 2 hatte ein Abo, das nirgends gespeichert ist (Webhook kam nie an).
	f.mu.Lock()
	f.subs["sub_alt"] = stripeSub{ID: "sub_alt", Status: "active", Customer: "cus_alt", Created: hourAgo, Metadata: map[string]string{"salon_id": "2"}}
	f.subs["sub_ohne"] = stripeSub{ID: "sub_ohne", Status: "active", Customer: "cus_ohne", Created: hourAgo}
	f.mu.Unlock()
	if _, err := a.db.Exec(`DELETE FROM salons WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	res, err := a.db.Exec(`INSERT INTO salons (slug, name) VALUES ('neu', 'Neuer Salon')`)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := res.LastInsertId(); id != 2 {
		t.Fatalf("ID %d, Test erwartet die neu vergebene 2", id)
	}

	if code := signed(a, subEvent("evt_alt", "customer.subscription.updated", "sub_alt")); code != http.StatusOK {
		t.Fatalf("Webhook: %d", code)
	}
	session := fmt.Appendf(nil, `{"id":"evt_cs","type":"checkout.session.completed","data":{"object":{"id":"cs_alt",
		"mode":"subscription","client_reference_id":"2","subscription":"sub_ohne","created":%d}}}`, hourAgo)
	if code := signed(a, session); code != http.StatusOK {
		t.Fatalf("Webhook Session: %d", code)
	}
	if got := billingOf(t, a, 2); got != (salonBilling{"free", "", ""}) {
		t.Fatalf("altes Abo wirkt auf den neuen Salon: %+v", got)
	}
	// Gegenprobe: ein Abo des neuen Salons wirkt.
	f.setSub("sub_neu", "active", "cus_neu", "2")
	if code := signed(a, subEvent("evt_neu", "customer.subscription.updated", "sub_neu")); code != http.StatusOK {
		t.Fatalf("Webhook neu: %d", code)
	}
	if got := billingOf(t, a, 2); got != (salonBilling{"pro", "cus_neu", "sub_neu"}) {
		t.Fatalf("neues Abo: %+v", got)
	}
}

// N4: Kein Einbetten in fremde Seiten, kein <base>, HSTS nur über HTTPS.
// Kein form-action: das würde die Weiterleitung zu Stripe nach dem POST blockieren.
func TestSecurityHeaders(t *testing.T) {
	ts := newTestSalon(t)
	w := ts.do("GET", "test.kammzeit.test", "/", nil)
	h := w.Header()
	csp := h.Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP ohne %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "form-action") {
		t.Errorf("CSP mit form-action: %s", csp)
	}
	if h.Get("X-Frame-Options") != "DENY" || h.Get("Strict-Transport-Security") != "" {
		t.Errorf("über HTTP: X-Frame-Options %q, HSTS %q", h.Get("X-Frame-Options"), h.Get("Strict-Transport-Security"))
	}
	for _, r := range []*http.Request{
		httptest.NewRequest("GET", "https://test.kammzeit.test/", nil),
		func() *http.Request { // hinter Caddy
			r := httptest.NewRequest("GET", "http://test.kammzeit.test/", nil)
			r.Header.Set("X-Forwarded-Proto", "https")
			return r
		}(),
	} {
		w := httptest.NewRecorder()
		ts.h.ServeHTTP(w, r)
		if got := w.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
			t.Errorf("HSTS über HTTPS: %q", got)
		}
	}
}

// N6: Im Gratis-Plan ist nur der erste aktive Mitarbeiter (sort, id) online
// buchbar, etwa nach dem Wechsel von Pro zurück zu Gratis. Gelöscht oder
// deaktiviert wird nichts. Pro und Einzelbetrieb haben keine Grenze.
func TestFreePlanOffersFirstStaffOnly(t *testing.T) {
	ts := newTestSalon(t)
	ts.exec(t, `UPDATE salons SET plan = 'free' WHERE id = ?`, ts.salon)
	staffFor := func(service int64) string {
		return ts.do("GET", "test.kammzeit.test", "/mitarbeiter?service="+strconv.FormatInt(service, 10), nil).Body.String()
	}
	offers := func(body string, anna, ben bool) bool {
		return strings.Contains(body, "Anna") == anna && strings.Contains(body, "Ben") == ben
	}
	if !offers(staffFor(ts.schnitt), true, false) {
		t.Fatal("Gratis: nur Anna erwartet")
	}
	if strings.Contains(ts.do("GET", "test.kammzeit.test", "/", nil).Body.String(), "Ben") {
		t.Fatal("Gratis: Startseite bietet Ben an") // erste Leistung ist Farbe, die nur Ben kann
	}
	if offers(staffFor(ts.farbe), false, true) {
		t.Fatal("Gratis: Farbe (nur Ben) trotzdem buchbar")
	}
	s, _ := ts.a.loadSalon(ts.salon)
	start := dayAfterTomorrow10(s.Loc)
	ts.appt(t, ts.anna, ts.cid, start, time.Hour)
	q := "/slots?service=" + strconv.FormatInt(ts.schnitt, 10) + "&staff=egal&date=" + start.Format(time.DateOnly)
	if strings.Contains(ts.do("GET", "test.kammzeit.test", q, nil).Body.String(), `value="`+strconv.FormatInt(start.Unix(), 10)+`"`) {
		t.Fatal("Gratis: Slot über Ben angeboten")
	}
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, 0, start)); w.Code != http.StatusConflict {
		t.Fatalf("Gratis, egal, Anna belegt: %d", w.Code)
	}
	if w := ts.do("POST", "test.kammzeit.test", "/book", bookForm(ts.schnitt, ts.ben, start.Add(2*time.Hour))); w.Code != http.StatusBadRequest {
		t.Fatalf("Gratis, Ben direkt: %d", w.Code)
	}
	// Reihenfolge nach sort, bei Gleichstand die kleinere ID; Inaktive rücken nicht vor.
	ts.exec(t, `UPDATE staff SET sort = -1 WHERE id = ?`, ts.ben)
	if !offers(staffFor(ts.schnitt), false, true) {
		t.Fatal("sort zählt nicht")
	}
	ts.exec(t, `UPDATE staff SET sort = 0`)
	if !offers(staffFor(ts.schnitt), true, false) {
		t.Fatal("bei gleichem sort zählt die kleinere ID")
	}
	ts.exec(t, `UPDATE staff SET active = 0 WHERE id = ?`, ts.anna)
	if !offers(staffFor(ts.farbe), false, true) {
		t.Fatal("nach Deaktivieren von Anna rückt Ben nicht nach")
	}
	ts.exec(t, `UPDATE staff SET active = 1`)
	if n := ts.count(t, `SELECT COUNT(*) FROM staff WHERE salon_id = ? AND active = 1`, ts.salon); n != 2 {
		t.Fatalf("%d aktive Mitarbeiter, nichts sollte sich ändern", n)
	}

	ts.a.cfg.SingleTenant = true // Einzelbetrieb: keine Grenze, auch ohne Pro
	if !offers(staffFor(ts.schnitt), true, true) {
		t.Fatal("Einzelbetrieb begrenzt")
	}
	ts.a.cfg.SingleTenant = false
	ts.exec(t, `UPDATE salons SET plan = 'pro' WHERE id = ?`, ts.salon)
	if !offers(staffFor(ts.schnitt), true, true) {
		t.Fatal("Pro begrenzt")
	}
}
