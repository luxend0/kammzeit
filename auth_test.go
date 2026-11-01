package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newAuthTestApp(t *testing.T) (*app, int64) {
	t.Helper()
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`INSERT INTO salons (slug, name) VALUES ('mueller', 'Salon Müller')`)
	if err != nil {
		t.Fatal(err)
	}
	salonID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO users (salon_id, email) VALUES (?, 'inhaber@example.com')`, salonID)
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := res.LastInsertId()
	loginIPLimit = newLimiter(5, 15*time.Minute)
	loginMailLimit = newLimiter(3, 15*time.Minute)
	return &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "http://kammzeit.test"}, db: db, tpl: tpl}, userID
}

// authRequest schickt eine Anfrage an die Hauptdomain; POST mit gleicher Herkunft.
func authRequest(a *app, method, path, ip string, form url.Values) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, "http://kammzeit.test"+path, body)
	r.RemoteAddr = ip + ":1234"
	if method == http.MethodPost {
		r.Header.Set("Origin", "http://kammzeit.test")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

func TestLoginNext(t *testing.T) {
	for in, want := range map[string]string{
		"/dashboard/kunden":         "/dashboard/kunden",
		"/dashboard?tag=2026-11-02": "/dashboard?tag=2026-11-02",
		"":                          "",
		"dashboard":                 "",
		"//evil.com":                "",
		"https://evil.com":          "",
		`/\evil`:                    "",
		"/\t/evil.com":              "",
		"javascript:alert(1)":       "",
	} {
		if got := loginNext(in); got != want {
			t.Errorf("loginNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoginTokenOnlyOnce(t *testing.T) {
	a, userID := newAuthTestApp(t)
	token, err := a.newLoginToken(userID)
	if err != nil {
		t.Fatal(err)
	}
	path := "/login/" + token

	// Ansehen (Link-Vorschau, Mail-Scanner) verbraucht den Token nicht.
	for i := 0; i < 2; i++ {
		w := authRequest(a, "GET", path, "198.51.100.1", nil)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Jetzt anmelden") {
			t.Fatalf("GET %d: %d %s", i, w.Code, w.Body)
		}
	}

	// Fremde Herkunft (Login-CSRF) wird abgewiesen und verbraucht nichts.
	r := httptest.NewRequest("POST", "http://kammzeit.test"+path, nil)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST fremde Herkunft: %d", w.Code)
	}

	w = authRequest(a, "POST", path, "198.51.100.1", url.Values{"weiter": {"/dashboard/kunden"}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard/kunden" {
		t.Fatalf("Einlösen: %d nach %q", w.Code, w.Header().Get("Location"))
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("kein Session-Cookie")
	}
	check := httptest.NewRequest("GET", "http://kammzeit.test/", nil)
	check.AddCookie(cookie)
	if u, err := a.currentUser(check); err != nil || u == nil || u.ID != userID {
		t.Fatalf("Session gehört nicht zum Konto: %v %v", u, err)
	}

	// Zweites Einlösen scheitert, auch die Bestätigungsseite sagt das.
	w = authRequest(a, "POST", path, "198.51.100.1", nil)
	if w.Code != http.StatusGone || len(w.Result().Cookies()) != 0 {
		t.Fatalf("zweites Einlösen: %d, Cookies %v", w.Code, w.Result().Cookies())
	}
	w = authRequest(a, "GET", path, "198.51.100.1", nil)
	if w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "Neuen Link anfordern") {
		t.Fatalf("GET nach Einlösen: %d", w.Code)
	}

	// Fremdes Ziel in weiter landet im Dashboard.
	token2, _ := a.newLoginToken(userID)
	w = authRequest(a, "POST", "/login/"+token2, "198.51.100.1", url.Values{"weiter": {"//evil.com"}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/dashboard" {
		t.Fatalf("weiter=//evil.com: %d nach %q", w.Code, w.Header().Get("Location"))
	}
}

func TestLoginTokenExpired(t *testing.T) {
	a, userID := newAuthTestApp(t)
	token := randomToken()
	if _, err := a.db.Exec(`INSERT INTO login_tokens (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		hashToken(token), userID, time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST"} {
		w := authRequest(a, method, "/login/"+token, "198.51.100.2", nil)
		if w.Code != http.StatusGone || len(w.Result().Cookies()) != 0 {
			t.Errorf("%s abgelaufener Token: %d", method, w.Code)
		}
	}
}

// Keine Konto-Aufzählung: bekannte und unbekannte Adresse bekommen dieselbe Antwort.
func TestLoginSameAnswer(t *testing.T) {
	a, _ := newAuthTestApp(t)
	known := authRequest(a, "POST", "/login", "198.51.100.3", url.Values{"email": {"Inhaber@Example.com"}})
	unknown := authRequest(a, "POST", "/login", "198.51.100.4", url.Values{"email": {"niemand@example.com"}})
	if known.Code != http.StatusOK || unknown.Code != known.Code || unknown.Body.String() != known.Body.String() {
		t.Fatalf("Antworten unterscheiden sich: %d vs. %d", known.Code, unknown.Code)
	}
	for _, want := range []string{"Wenn es ein Konto mit dieser Adresse gibt", "Öffne den Link auf dem Gerät, auf dem du angemeldet sein willst."} {
		if !strings.Contains(known.Body.String(), want) {
			t.Fatalf("Hinweis %q fehlt: %s", want, known.Body)
		}
	}
	// Token und Mail entstehen im Hintergrund (gleiche Antwortzeit): kurz warten.
	var n int
	for i := 0; i < 100 && n == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		if err := a.db.QueryRow(`SELECT COUNT(*) FROM login_tokens`).Scan(&n); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("login_tokens = %d, want 1", n)
	}
}

// Ungültige Adresse: Fehler steht am Feld, nicht nur oben.
func TestLoginFieldError(t *testing.T) {
	a, _ := newAuthTestApp(t)
	w := authRequest(a, "POST", "/login", "198.51.100.8", url.Values{"email": {"keine-mail"}})
	body := w.Body.String()
	if w.Code != http.StatusBadRequest || !strings.Contains(body, `aria-invalid="true" aria-describedby="auth-err-email"`) ||
		!strings.Contains(body, `<span class="field-error" id="auth-err-email">Bitte eine gültige E-Mail-Adresse angeben.</span>`) ||
		!strings.Contains(body, `value="keine-mail"`) {
		t.Fatalf("Fehler nicht am Feld: %d %s", w.Code, body)
	}
}

func TestLoginRateLimit(t *testing.T) {
	a, _ := newAuthTestApp(t)
	for i := 1; i <= 6; i++ {
		w := authRequest(a, "POST", "/login", "203.0.113.5", url.Values{"email": {fmt.Sprintf("x%d@example.com", i)}})
		if want := map[bool]int{true: http.StatusOK, false: http.StatusTooManyRequests}[i <= 5]; w.Code != want {
			t.Fatalf("pro IP, Versuch %d: %d, want %d", i, w.Code, want)
		}
	}
	for i := 1; i <= 4; i++ {
		w := authRequest(a, "POST", "/login", fmt.Sprintf("203.0.113.%d", 10+i), url.Values{"email": {"viel@example.com"}})
		if want := map[bool]int{true: http.StatusOK, false: http.StatusTooManyRequests}[i <= 3]; w.Code != want {
			t.Fatalf("pro Mail, Versuch %d: %d, want %d", i, w.Code, want)
		}
	}
}

// Login auf kammzeit.com: englische Seiten; eine Anforderung über eine Domain
// setzt die Sprache des Kontos, damit der Link auf diese Domain geht.
func TestLoginEnglish(t *testing.T) {
	a, userID := newAuthTestApp(t)
	a.cfg.Domains = parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")
	h := a.routes()
	do := func(method, host, path string, form url.Values) *httptest.ResponseRecorder {
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		r := httptest.NewRequest(method, "http://"+host+path, body)
		r.RemoteAddr = "198.51.100.70:1234"
		if method == http.MethodPost {
			r.Header.Set("Origin", "http://"+host)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := do("GET", "kammzeit-en.test", "/login", nil).Body.String()
	for _, want := range []string{`<html lang="en">`, "Send login link", "Sign up for free", "<title>Log in - Kammzeit</title>"} {
		if !strings.Contains(body, want) {
			t.Errorf("Login en: %q fehlt", want)
		}
	}
	if w := do("POST", "kammzeit-en.test", "/login", url.Values{"email": {"kaputt"}}); !strings.Contains(w.Body.String(), "Please enter a valid email address.") {
		t.Errorf("Fehler en: %s", w.Body)
	}
	// Sprache des Kontos, sobald der Token (im Hintergrund) angelegt ist.
	langAfter := func(host string) string {
		a.db.Exec(`DELETE FROM login_tokens`)
		if w := do("POST", host, "/login", url.Values{"email": {"inhaber@example.com"}}); w.Code != http.StatusOK {
			t.Fatalf("Anfordern über %s: %d", host, w.Code)
		}
		var n int
		for i := 0; i < 100 && n == 0; i++ {
			time.Sleep(10 * time.Millisecond)
			a.db.QueryRow(`SELECT COUNT(*) FROM login_tokens WHERE user_id = ?`, userID).Scan(&n)
		}
		var l string
		if err := a.db.QueryRow(`SELECT lang FROM users WHERE id = ?`, userID).Scan(&l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	if l := langAfter("kammzeit-en.test"); l != "en" || a.siteURLLang(l) != "https://kammzeit-en.test" {
		t.Fatalf("Sprache nach Anforderung über .com: %q", l)
	}
	if l := langAfter("kammzeit.test"); l != "de" {
		t.Fatalf("Sprache nach Anforderung über .de: %q", l)
	}
	// Ungültiger Link: Meldung englisch.
	if w := do("GET", "kammzeit-en.test", "/login/"+randomToken(), nil); w.Code != http.StatusGone || !strings.Contains(w.Body.String(), "Link no longer valid") {
		t.Errorf("ungültiger Link en: %d", w.Code)
	}
}
