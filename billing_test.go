package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const testWebhookSecret = "whsec_test"

func stripeSign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(m.Sum(nil))
}

func TestVerifyStripeSig(t *testing.T) {
	now := time.Now()
	body := []byte(`{"id":"evt_1"}`)
	good := stripeSign(testWebhookSecret, now, body)
	cases := []struct {
		name, header, secret string
		body                 []byte
		want                 bool
	}{
		{"gültig", good, testWebhookSecret, body, true},
		{"falsches Secret", stripeSign("whsec_falsch", now, body), testWebhookSecret, body, false},
		{"Zeitstempel zu alt", stripeSign(testWebhookSecret, now.Add(-6*time.Minute), body), testWebhookSecret, body, false},
		{"Zeitstempel in der Zukunft", stripeSign(testWebhookSecret, now.Add(6*time.Minute), body), testWebhookSecret, body, false},
		{"knapp in der Toleranz", stripeSign(testWebhookSecret, now.Add(-4*time.Minute), body), testWebhookSecret, body, true},
		{"Body manipuliert", good, testWebhookSecret, []byte(`{"id":"evt_2"}`), false},
		{"mehrere v1, eine passt", good[:strings.Index(good, ",")] + ",v1=00ff," + good[strings.Index(good, ",")+1:], testWebhookSecret, body, true},
		{"ohne Secret konfiguriert", stripeSign("", now, body), "", body, false},
		{"ohne Zeitstempel", good[strings.Index(good, ",")+1:], testWebhookSecret, body, false},
		{"leer", "", testWebhookSecret, body, false},
	}
	for _, c := range cases {
		if got := verifyStripeSig(c.header, c.body, c.secret, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// fakeStripe spielt die Stripe-API: Abos abrufen, Checkout- und Portal-Sessions anlegen.
type fakeStripe struct {
	mu    sync.Mutex
	subs  map[string]stripeSub
	calls int
	path  string
	form  url.Values
}

func newFakeStripe(t *testing.T) *fakeStripe {
	f := &fakeStripe{subs: map[string]stripeSub{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls++
		if user, _, _ := r.BasicAuth(); user != "sk_test_x" {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":{"message":"Invalid API Key provided"}}`)
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/"):
			sub, ok := f.subs[strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"error":{"message":"No such subscription"}}`)
				return
			}
			json.NewEncoder(w).Encode(sub)
		case r.Method == http.MethodPost && (r.URL.Path == "/v1/checkout/sessions" || r.URL.Path == "/v1/billing_portal/sessions"):
			r.ParseForm()
			f.path, f.form = r.URL.Path, r.PostForm
			fmt.Fprint(w, `{"id":"cs_test_1","url":"https://checkout.stripe.test/c/pay/cs_test_1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	old := stripeAPI
	stripeAPI = srv.URL
	t.Cleanup(func() { stripeAPI = old; srv.Close() })
	return f
}

func (f *fakeStripe) setSub(id, status, customer, salonID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[id] = stripeSub{ID: id, Status: status, Customer: customer, Metadata: map[string]string{"salon_id": salonID}}
}

func setStripeEnv(t *testing.T, key, monthly, yearly string) {
	t.Setenv("STRIPE_SECRET_KEY", key)
	t.Setenv("STRIPE_WEBHOOK_SECRET", testWebhookSecret)
	t.Setenv("STRIPE_PRICE_MONTHLY", monthly)
	t.Setenv("STRIPE_PRICE_YEARLY", yearly)
}

func newBillingApp(t *testing.T) *app {
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO salons (id, slug, name) VALUES (1, 'mueller', 'Salon Müller'), (2, 'schmidt', 'Salon Schmidt');
		INSERT INTO users (salon_id, email) VALUES (1, 'inhaber@mueller.test'), (2, 'inhaber@schmidt.test')`); err != nil {
		t.Fatal(err)
	}
	return &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test", PublicURL: "https://{slug}.kammzeit.test"}, db: db, tpl: tpl}
}

type salonBilling struct{ plan, customer, sub string }

func billingOf(t *testing.T, a *app, id int64) salonBilling {
	var b salonBilling
	if err := a.db.QueryRow(`SELECT plan, stripe_customer_id, stripe_subscription_id FROM salons WHERE id = ?`, id).Scan(&b.plan, &b.customer, &b.sub); err != nil {
		t.Fatal(err)
	}
	return b
}

func sendWebhook(a *app, body []byte, sig string) int {
	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", sig)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	return rec.Code
}

func signed(a *app, body []byte) int {
	return sendWebhook(a, body, stripeSign(testWebhookSecret, time.Now(), body))
}

func checkoutCompleted(evt, salonID, sub string) []byte {
	return fmt.Appendf(nil, `{"id":%q,"object":"event","type":"checkout.session.completed","data":{"object":{"id":"cs_1","object":"checkout.session",
		"mode":"subscription","client_reference_id":%q,"customer":"cus_x","subscription":%q,"metadata":{"salon_id":%q}}}}`, evt, salonID, sub, salonID)
}

func subEvent(evt, typ, sub string) []byte {
	return fmt.Appendf(nil, `{"id":%q,"object":"event","type":%q,"data":{"object":{"id":%q,"object":"subscription","status":"active","customer":"cus_x"}}}`, evt, typ, sub)
}

func TestStripeWebhook(t *testing.T) {
	setStripeEnv(t, "sk_test_x", "price_m", "price_y")
	f := newFakeStripe(t)
	a := newBillingApp(t)
	events := func() (n int) {
		a.db.QueryRow(`SELECT COUNT(*) FROM stripe_events`).Scan(&n)
		return
	}

	// checkout.session.completed setzt pro und speichert Kunde und Abo, nur beim richtigen Salon.
	f.setSub("sub_1", "active", "cus_1", "1")
	if code := signed(a, checkoutCompleted("evt_1", "1", "sub_1")); code != http.StatusOK {
		t.Fatalf("checkout.session.completed: %d", code)
	}
	if got := billingOf(t, a, 1); got != (salonBilling{"pro", "cus_1", "sub_1"}) {
		t.Fatalf("nach Checkout: %+v", got)
	}
	if got := billingOf(t, a, 2); got.plan != "free" {
		t.Fatalf("anderer Salon verändert: %+v", got)
	}

	// Dasselbe Ereignis noch einmal wirkt nicht: der von Hand gesetzte Plan bleibt.
	a.db.Exec(`UPDATE salons SET plan = 'free' WHERE id = 1`)
	if code := signed(a, checkoutCompleted("evt_1", "1", "sub_1")); code != http.StatusOK {
		t.Fatalf("Wiederholung: %d", code)
	}
	if got := billingOf(t, a, 1); got.plan != "free" || events() != 1 {
		t.Fatalf("doppeltes Ereignis hat gewirkt: %+v, %d Ereignisse", got, events())
	}
	a.db.Exec(`UPDATE salons SET plan = 'pro' WHERE id = 1`)

	// Ungültige Signatur und fremde Ereignistypen ändern nichts.
	f.setSub("sub_1", "canceled", "cus_1", "1")
	if code := sendWebhook(a, subEvent("evt_x", "customer.subscription.deleted", "sub_1"), "t=1,v1=00"); code != http.StatusBadRequest {
		t.Fatalf("ungültige Signatur: %d", code)
	}
	if code := signed(a, subEvent("evt_u", "invoice.paid", "sub_1")); code != http.StatusOK {
		t.Fatalf("unbekannter Typ: %d", code)
	}
	if got := billingOf(t, a, 1); got.plan != "pro" || events() != 1 {
		t.Fatalf("ignorierte Ereignisse haben gewirkt: %+v, %d Ereignisse", got, events())
	}

	// customer.subscription.deleted setzt free, Kunden-ID bleibt fürs nächste Abo.
	if code := signed(a, subEvent("evt_2", "customer.subscription.deleted", "sub_1")); code != http.StatusOK {
		t.Fatalf("deleted: %d", code)
	}
	if got := billingOf(t, a, 1); got != (salonBilling{"free", "cus_1", ""}) {
		t.Fatalf("nach deleted: %+v", got)
	}

	// Reihenfolge vertauscht: Abo schon beendet, bevor checkout.session.completed ankommt -> kein Pro.
	f.setSub("sub_2", "canceled", "cus_2", "2")
	if code := signed(a, checkoutCompleted("evt_3", "2", "sub_2")); code != http.StatusOK {
		t.Fatalf("spätes completed: %d", code)
	}
	if got := billingOf(t, a, 2); got.plan != "free" {
		t.Fatalf("beendetes Abo hat freigeschaltet: %+v", got)
	}

	// customer.subscription.updated (aktiv, Salon über Metadaten) setzt pro;
	// ein spätes Ereignis zum alten, beendeten Abo nimmt das neue nicht weg.
	f.setSub("sub_3", "active", "cus_1", "1")
	if code := signed(a, subEvent("evt_4", "customer.subscription.updated", "sub_3")); code != http.StatusOK {
		t.Fatalf("updated: %d", code)
	}
	if code := signed(a, subEvent("evt_5", "customer.subscription.updated", "sub_1")); code != http.StatusOK {
		t.Fatalf("updated altes Abo: %d", code)
	}
	if got := billingOf(t, a, 1); got != (salonBilling{"pro", "cus_1", "sub_3"}) {
		t.Fatalf("nach updated: %+v", got)
	}

	// Unbezahlt -> free (Salon über die gespeicherte Abo-ID, ohne Metadaten).
	f.mu.Lock()
	f.subs["sub_3"] = stripeSub{ID: "sub_3", Status: "unpaid", Customer: "cus_1"}
	f.mu.Unlock()
	if code := signed(a, subEvent("evt_6", "customer.subscription.updated", "sub_3")); code != http.StatusOK {
		t.Fatalf("updated unpaid: %d", code)
	}
	if got := billingOf(t, a, 1); got != (salonBilling{"free", "cus_1", ""}) {
		t.Fatalf("unpaid: %+v", got)
	}
	// Abo ohne Metadaten (im Stripe-Dashboard angelegt) findet den Salon über die Kunden-ID.
	f.mu.Lock()
	f.subs["sub_5"] = stripeSub{ID: "sub_5", Status: "active", Customer: "cus_1"}
	f.mu.Unlock()
	if code := signed(a, subEvent("evt_7", "customer.subscription.updated", "sub_5")); code != http.StatusOK {
		t.Fatalf("updated ohne Metadaten: %d", code)
	}
	if got := billingOf(t, a, 1); got != (salonBilling{"pro", "cus_1", "sub_5"}) {
		t.Fatalf("ohne Metadaten: %+v", got)
	}

	// Stripe-API nicht erreichbar oder Abo unbekannt -> 500, Ereignis nicht verbucht (Stripe wiederholt).
	before := events()
	if code := signed(a, subEvent("evt_9", "customer.subscription.updated", "sub_gibtsnicht")); code != http.StatusInternalServerError {
		t.Fatalf("Abo unbekannt: %d", code)
	}
	// DB-Fehler -> 500, und der stripe_events-Eintrag ist zurückgerollt.
	f.setSub("sub_4", "active", "cus_4", "2")
	if _, err := a.db.Exec(`ALTER TABLE salons RENAME TO salons_weg`); err != nil {
		t.Fatal(err)
	}
	if code := signed(a, subEvent("evt_8", "customer.subscription.updated", "sub_4")); code != http.StatusInternalServerError {
		t.Fatalf("DB-Fehler: %d", code)
	}
	if events() != before {
		t.Fatalf("Ereignis trotz Fehler verbucht: %d statt %d", events(), before)
	}
}

func dashRequest(method, target string, form url.Values) *http.Request {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return r
}

func dctxFor(t *testing.T, a *app, salonID int64) *dctx {
	s, err := a.loadSalon(salonID)
	if err != nil {
		t.Fatal(err)
	}
	u := &user{SalonID: salonID}
	a.db.QueryRow(`SELECT id, email FROM users WHERE salon_id = ?`, salonID).Scan(&u.ID, &u.Email)
	return &dctx{User: u, Salon: s}
}

func TestAboCheckout(t *testing.T) {
	setStripeEnv(t, "sk_test_x", "price_m", "price_y")
	f := newFakeStripe(t)
	a := newBillingApp(t)

	// Neuer Kunde, jährlich: alle Parameter wie vereinbart, Weiterleitung auf die Stripe-URL.
	rec := httptest.NewRecorder()
	a.aboCheckout(rec, dashRequest(http.MethodPost, "/dashboard/abo/checkout", url.Values{"intervall": {"jahr"}}), dctxFor(t, a, 1))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "https://checkout.stripe.test/c/pay/cs_test_1" {
		t.Fatalf("Antwort %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	want := map[string]string{
		"mode":                                  "subscription",
		"line_items[0][price]":                  "price_y",
		"line_items[0][quantity]":               "1",
		"client_reference_id":                   "1",
		"metadata[salon_id]":                    "1",
		"subscription_data[metadata][salon_id]": "1",
		"customer_email":                        "inhaber@mueller.test",
		"success_url":                           "https://kammzeit.test/dashboard/abo?erfolg",
		"cancel_url":                            "https://kammzeit.test/dashboard/abo",
		"locale":                                "de",
	}
	if f.path != "/v1/checkout/sessions" || len(f.form) != len(want) {
		t.Fatalf("Pfad %s, Parameter %v", f.path, f.form)
	}
	for k, v := range want {
		if f.form.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, f.form.Get(k), v)
		}
	}

	// Bekannter Stripe-Kunde, monatlich: customer statt customer_email.
	a.db.Exec(`UPDATE salons SET stripe_customer_id = 'cus_9' WHERE id = 1`)
	rec = httptest.NewRecorder()
	a.aboCheckout(rec, dashRequest(http.MethodPost, "/dashboard/abo/checkout", url.Values{"intervall": {"monat"}}), dctxFor(t, a, 1))
	if rec.Code != http.StatusSeeOther || f.form.Get("customer") != "cus_9" || f.form.Has("customer_email") || f.form.Get("line_items[0][price]") != "price_m" {
		t.Fatalf("monatlich mit Kunde: %d %v", rec.Code, f.form)
	}

	// Unbekanntes Intervall -> 400 ohne Stripe-Aufruf; schon Pro -> zurück zur Abo-Seite.
	calls := f.calls
	rec = httptest.NewRecorder()
	a.aboCheckout(rec, dashRequest(http.MethodPost, "/dashboard/abo/checkout", url.Values{"intervall": {"price_evil"}}), dctxFor(t, a, 1))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("falsches Intervall: %d", rec.Code)
	}
	a.db.Exec(`UPDATE salons SET plan = 'pro' WHERE id = 1`)
	rec = httptest.NewRecorder()
	a.aboCheckout(rec, dashRequest(http.MethodPost, "/dashboard/abo/checkout", url.Values{"intervall": {"monat"}}), dctxFor(t, a, 1))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/dashboard/abo" || f.calls != calls {
		t.Fatalf("schon Pro: %d %q, %d Stripe-Aufrufe", rec.Code, rec.Header().Get("Location"), f.calls-calls)
	}

	// Kundenportal mit Rücksprung zur Abo-Seite.
	rec = httptest.NewRecorder()
	a.aboPortal(rec, dashRequest(http.MethodPost, "/dashboard/abo/portal", nil), dctxFor(t, a, 1))
	if rec.Code != http.StatusSeeOther || f.path != "/v1/billing_portal/sessions" || f.form.Get("customer") != "cus_9" ||
		f.form.Get("return_url") != "https://kammzeit.test/dashboard/abo" {
		t.Fatalf("Portal: %d %s %v", rec.Code, f.path, f.form)
	}
}

func TestAboPage(t *testing.T) {
	a := newBillingApp(t)
	page := func(salonID int64, target string) string {
		rec := httptest.NewRecorder()
		a.aboPage(rec, dashRequest(http.MethodGet, target, nil), dctxFor(t, a, salonID))
		if rec.Code != http.StatusOK {
			t.Fatalf("Abo-Seite: %d", rec.Code)
		}
		return rec.Body.String()
	}

	// Ohne Stripe-Konfiguration: Hinweis statt Buttons, auch ohne Schlüssel in der Umgebung.
	setStripeEnv(t, "", "", "")
	body := page(1, "/dashboard/abo")
	if strings.Contains(body, "/dashboard/abo/checkout") || !strings.Contains(body, "Online-Zahlung wird gerade eingerichtet") {
		t.Fatalf("ohne Konfiguration:\n%s", body)
	}
	// Schlüssel ohne Preise zählt auch als nicht eingerichtet.
	setStripeEnv(t, "sk_test_x", "", "")
	if body := page(1, "/dashboard/abo"); strings.Contains(body, "/dashboard/abo/checkout") {
		t.Fatal("Buttons ohne Preise")
	}

	setStripeEnv(t, "sk_test_x", "price_m", "price_y")
	body = page(1, "/dashboard/abo")
	for _, s := range []string{"Pro monatlich", "Pro jährlich", `value="monat"`, `value="jahr"`, "14,90 €", "149 €", "§ 19 UStG"} {
		if !strings.Contains(body, s) {
			t.Errorf("Gratis-Seite ohne %q", s)
		}
	}
	// Zurück vom Checkout: Danke, aber keine Kauf-Buttons (kein zweites Abo, solange der Webhook fehlt).
	body = page(1, "/dashboard/abo?erfolg")
	if !strings.Contains(body, "Danke! Dein Pro-Abo ist aktiv, sobald die Zahlung bestätigt ist.") || strings.Contains(body, "/dashboard/abo/checkout") {
		t.Fatalf("nach Checkout:\n%s", body)
	}
	// Pro mit Stripe-Kunde: Portal-Button, keine Kauf-Buttons. Pro ohne Stripe (von Hand): Kontakt.
	a.db.Exec(`UPDATE salons SET plan = 'pro', stripe_customer_id = 'cus_1' WHERE id = 1`)
	body = page(1, "/dashboard/abo")
	if !strings.Contains(body, "Abo verwalten / kündigen") || strings.Contains(body, "/dashboard/abo/checkout") {
		t.Fatalf("Pro-Seite:\n%s", body)
	}
	a.db.Exec(`UPDATE salons SET plan = 'pro' WHERE id = 2`)
	if body := page(2, "/dashboard/abo"); strings.Contains(body, "/dashboard/abo/portal") || !strings.Contains(body, "contact@instantnode.eu") {
		t.Fatalf("Pro ohne Stripe:\n%s", body)
	}
}
