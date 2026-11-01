package main

// Pro-Abo über Stripe Checkout und Kundenportal, Webhook: /dashboard/abo/..., /stripe/webhook.
// Kein Stripe-SDK: Form-POSTs an die API, Webhook-Signatur selbst geprüft.

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// stripeAPI ist die Basis-URL der Stripe-API. Tests setzen einen httptest-Server ein.
var stripeAPI = "https://api.stripe.com"

var stripeHTTP = &http.Client{Timeout: 15 * time.Second}

// Toleranz für den Zeitstempel in Stripe-Signature (Schutz gegen Wiedereinspielen).
const stripeSigTolerance = 5 * time.Minute

type stripeConf struct{ key, webhookSecret, monthly, yearly string }

// stripeEnv liest die Konfiguration bei jedem Aufruf aus der Umgebung.
func stripeEnv() stripeConf {
	return stripeConf{
		key:           os.Getenv("STRIPE_SECRET_KEY"),
		webhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		monthly:       os.Getenv("STRIPE_PRICE_MONTHLY"),
		yearly:        os.Getenv("STRIPE_PRICE_YEARLY"),
	}
}

// online: Online-Zahlung ist eingerichtet (Schlüssel und beide Preise gesetzt).
func (s stripeConf) online() bool { return s.key != "" && s.monthly != "" && s.yearly != "" }

func (a *app) routesBilling(mux *http.ServeMux) {
	mux.HandleFunc("GET /dashboard/abo", a.dash(a.aboPage))
	mux.HandleFunc("POST /dashboard/abo/checkout", a.dash(a.aboCheckout))
	mux.HandleFunc("POST /dashboard/abo/portal", a.dash(a.aboPortal))
	mux.HandleFunc("POST /stripe/webhook", a.stripeWebhook)
}

func (a *app) aboPage(w http.ResponseWriter, r *http.Request, c *dctx) {
	cfg := stripeEnv()
	d := a.dashPage(c, r, "abo", tr(a.langFor(r), "Abo"))
	d["PageTitle"], d["Heading"] = d["Heading"], "" // Überschrift steht im .page-head des Templates
	d["Online"] = cfg.online()
	d["Portal"] = cfg.key != "" && c.Salon.StripeCustomerID != ""
	d["Erfolg"] = r.URL.Query().Has("erfolg")
	a.render(w, http.StatusOK, "abo-seite", d)
}

func (a *app) aboCheckout(w http.ResponseWriter, r *http.Request, c *dctx) {
	lang := a.langFor(r)
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, tr(lang, "Das Formular war unvollständig."))
		return
	}
	cfg := stripeEnv()
	if !cfg.online() || c.Salon.Pro() {
		http.Redirect(w, r, "/dashboard/abo", http.StatusSeeOther)
		return
	}
	price := map[string]string{"monat": cfg.monthly, "jahr": cfg.yearly}[r.PostFormValue("intervall")]
	if price == "" {
		a.dashError(w, r, c, http.StatusBadRequest, tr(lang, "Bitte monatlich oder jährlich wählen."))
		return
	}
	id := strconv.FormatInt(c.Salon.ID, 10)
	form := url.Values{
		"mode":                                  {"subscription"},
		"line_items[0][price]":                  {price},
		"line_items[0][quantity]":               {"1"},
		"client_reference_id":                   {id},
		"metadata[salon_id]":                    {id},
		"subscription_data[metadata][salon_id]": {id},
		"success_url":                           {a.siteURLFor(r) + "/dashboard/abo?erfolg"},
		"cancel_url":                            {a.siteURLFor(r) + "/dashboard/abo"},
		"locale":                                {stripeLocale(lang)},
	}
	if c.Salon.StripeCustomerID != "" {
		form.Set("customer", c.Salon.StripeCustomerID)
	} else {
		form.Set("customer_email", c.User.Email)
	}
	a.stripeRedirect(w, r, c, "/v1/checkout/sessions", form)
}

func (a *app) aboPortal(w http.ResponseWriter, r *http.Request, c *dctx) {
	if stripeEnv().key == "" || c.Salon.StripeCustomerID == "" {
		http.Redirect(w, r, "/dashboard/abo", http.StatusSeeOther)
		return
	}
	a.stripeRedirect(w, r, c, "/v1/billing_portal/sessions", url.Values{
		"customer":   {c.Salon.StripeCustomerID},
		"return_url": {a.siteURLFor(r) + "/dashboard/abo"},
		"locale":     {stripeLocale(a.langFor(r))},
	})
}

// stripeLocale: Sprache der Stripe-Seiten. Englisch britisch wie die Seite (EU-Markt, Datum "3 November").
func stripeLocale(lang string) string {
	if lang == "en" {
		return "en-GB"
	}
	return "de"
}

// stripeRedirect legt eine Checkout- oder Portal-Session an und leitet auf deren URL weiter.
func (a *app) stripeRedirect(w http.ResponseWriter, r *http.Request, c *dctx, path string, form url.Values) {
	var s struct {
		URL string `json:"url"`
	}
	if err := stripeCall(http.MethodPost, path, form, &s); err != nil || s.URL == "" {
		log.Printf("Stripe %s (Salon %d): %v", path, c.Salon.ID, err)
		a.dashError(w, r, c, http.StatusBadGateway, tr(a.langFor(r), "Die Zahlungsseite ist gerade nicht erreichbar. Bitte später noch einmal versuchen."))
		return
	}
	http.Redirect(w, r, s.URL, http.StatusSeeOther)
}

// stripeCall schickt eine Anfrage an die Stripe-API (form = nil bei GET) und dekodiert die Antwort nach out.
func stripeCall(method, path string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	key := stripeEnv().key
	if key == "" {
		return errors.New("STRIPE_SECRET_KEY fehlt")
	}
	req, err := http.NewRequest(method, stripeAPI+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(key, "")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := stripeHTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		json.Unmarshal(data, &e)
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error.Message)
	}
	return json.Unmarshal(data, out)
}

// verifyStripeSig prüft den Header Stripe-Signature (t=...,v1=...[,v1=...]):
// HMAC-SHA256 über "t.body" mit dem Webhook-Secret, Zeitstempel höchstens 5 Minuten daneben.
func verifyStripeSig(header string, body []byte, secret string, now time.Time) bool {
	if secret == "" { // ohne Secret wäre die HMAC von jedem berechenbar
		return false
	}
	var ts string
	var sigs [][]byte
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			if b, err := hex.DecodeString(v); err == nil {
				sigs = append(sigs, b)
			}
		}
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || now.Sub(time.Unix(t, 0)).Abs() > stripeSigTolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, s := range sigs {
		if hmac.Equal(s, want) {
			return true
		}
	}
	return false
}

type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object struct {
			ID                string `json:"id"`
			Mode              string `json:"mode"`
			ClientReferenceID string `json:"client_reference_id"`
			Subscription      string `json:"subscription"`
			Created           int64  `json:"created"` // Unix-Sekunden, bei Checkout-Sessions
		} `json:"object"`
	} `json:"data"`
}

type stripeSub struct {
	ID       string            `json:"id"`
	Status   string            `json:"status"`
	Customer string            `json:"customer"`
	Created  int64             `json:"created"` // Unix-Sekunden
	Metadata map[string]string `json:"metadata"`
}

// stripeWebhook gleicht den Plan eines Salons mit seinem Stripe-Abo ab.
// Stripe liefert Ereignisse nicht in Reihenfolge und wiederholt sie nach
// Fehlern. Darum ist das Ereignis nur der Anstoß: der aktuelle Abo-Stand wird
// per API geholt, und jede Ereignis-ID wirkt nur einmal (stripe_events).
func (a *app) stripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		http.Error(w, "Anfrage zu groß", http.StatusBadRequest)
		return
	}
	if !verifyStripeSig(r.Header.Get("Stripe-Signature"), body, stripeEnv().webhookSecret, time.Now()) {
		http.Error(w, "Signatur ungültig", http.StatusBadRequest)
		return
	}
	var ev stripeEvent
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" {
		http.Error(w, "Ereignis unlesbar", http.StatusBadRequest)
		return
	}
	o := ev.Data.Object
	var subID string
	switch ev.Type {
	case "checkout.session.completed":
		if o.Mode == "subscription" {
			subID = o.Subscription
		}
	case "customer.subscription.updated", "customer.subscription.deleted":
		subID = o.ID
	}
	if subID == "" { // andere Ereignisse interessieren nicht
		w.WriteHeader(http.StatusOK)
		return
	}
	var sub stripeSub
	err = stripeCall(http.MethodGet, "/v1/subscriptions/"+url.PathEscape(subID), nil, &sub)
	if err == nil && sub.ID != subID {
		err = fmt.Errorf("Antwort für Abo %q", sub.ID)
	}
	if err != nil {
		log.Printf("Stripe-Webhook %s: Abo %s laden: %v", ev.ID, subID, err)
		http.Error(w, "Abo nicht abrufbar", http.StatusInternalServerError) // Stripe wiederholt
		return
	}
	if err := a.applyStripeSub(ev.ID, o.ClientReferenceID, o.Created, &sub); err != nil {
		log.Printf("Stripe-Webhook %s: %v", ev.ID, err)
		http.Error(w, "Fehler", http.StatusInternalServerError) // Stripe wiederholt
		return
	}
	w.WriteHeader(http.StatusOK)
}

// applyStripeSub setzt in einer Transaktion Ereignis-Eintrag und Plan. Bei
// einem Fehler rollt beides zurück, damit Stripes Wiederholung wieder wirkt.
// clientRef und refCreated stammen aus der Checkout-Session (sonst leer und 0).
func (a *app) applyStripeSub(eventID, clientRef string, refCreated int64, sub *stripeSub) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO stripe_events (id) VALUES (?) ON CONFLICT (id) DO NOTHING`, eventID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // schon verarbeitet
	}
	switch sub.Status {
	case "active", "trialing", "past_due":
		salonID, err := findStripeSalon(tx, clientRef, refCreated, sub)
		if err != nil {
			return err
		}
		if salonID == 0 {
			log.Printf("Stripe: Abo %s (Kunde %s) gehört zu keinem Salon", sub.ID, sub.Customer)
			break
		}
		if _, err := tx.Exec(`UPDATE salons SET plan = 'pro', stripe_customer_id = ?, stripe_subscription_id = ? WHERE id = ?`,
			sub.Customer, sub.ID, salonID); err != nil {
			return err
		}
	default:
		// canceled, unpaid, incomplete_expired, paused (und incomplete: Erstzahlung offen).
		// Nur das gespeicherte Abo zählt: ein altes, beendetes Abo nimmt einem
		// Salon mit neuerem Abo nichts weg, und ein Abo, das schon vor seinem
		// checkout.session.completed beendet war, schaltet nie frei.
		if _, err := tx.Exec(`UPDATE salons SET plan = 'free', stripe_subscription_id = ''
			WHERE stripe_subscription_id != '' AND stripe_subscription_id = ?`,
			sub.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// findStripeSalon: Salon-ID aus den Metadaten des Abos (von uns beim Checkout
// gesetzt), sonst client_reference_id der Checkout-Session, sonst gespeicherte
// Abo- oder Kunden-ID (z. B. ein im Stripe-Dashboard angelegtes Abo). 0 = keiner.
// SQLite vergibt die ID eines gelöschten Salons neu (INTEGER PRIMARY KEY ohne
// AUTOINCREMENT): eine Salon-ID zählt darum nur, wenn der Salon nicht jünger
// ist als das Abo bzw. die Session, die sie nennt.
func findStripeSalon(q querier, clientRef string, refCreated int64, sub *stripeSub) (int64, error) {
	for _, ref := range []struct {
		id      string
		created int64
	}{{sub.Metadata["salon_id"], sub.Created}, {clientRef, refCreated}} {
		id, err := strconv.ParseInt(ref.id, 10, 64)
		if err != nil {
			continue
		}
		var createdAt int64
		err = q.QueryRow(`SELECT created_at FROM salons WHERE id = ?`, id).Scan(&createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if createdAt > ref.created {
			log.Printf("Stripe: Abo %s nennt Salon %d, der aber erst nach dem Abo bzw. der Session angelegt wurde (ID neu vergeben?), ignoriert", sub.ID, id)
			continue
		}
		return id, nil
	}
	var id int64
	err := q.QueryRow(`SELECT id FROM salons WHERE (stripe_subscription_id != '' AND stripe_subscription_id = ?)
		OR (stripe_customer_id != '' AND stripe_customer_id = ?) ORDER BY id LIMIT 1`, sub.ID, sub.Customer).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}
