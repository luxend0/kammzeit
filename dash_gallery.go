package main

// Bilder hochladen im Dashboard: Logo und Titelbild kommen mit dem
// Einstellungsformular (dash_settings.go), die Fotogalerie (Pro) hat eine
// eigene Seite /dashboard/einstellungen/galerie.

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errMediaLimit = errors.New("Zu viele Bilder in kurzer Zeit. Bitte versuch es in einer Stunde noch einmal.")

// uploadError: Meldung für einen Fehler beim Lesen eines multipart-Formulars.
func uploadError(err error, tooBig string) string {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return tooBig
	}
	return "Das Formular war unvollständig."
}

// formImage liest das Bild aus einem Dateifeld; nil, wenn keins gewählt ist.
func (a *app) formImage(r *http.Request, field string, salonID int64) (*mediaImage, error) {
	if r.MultipartForm == nil || len(r.MultipartForm.File[field]) == 0 {
		return nil, nil
	}
	return a.readImage(r.MultipartForm.File[field][0], field, salonID)
}

// readImage prüft Größe und Upload-Limit und verarbeitet das Bild (media.go).
func (a *app) readImage(fh *multipart.FileHeader, kind string, salonID int64) (*mediaImage, error) {
	if fh.Size > mediaMaxBytes {
		return nil, errMediaSize
	}
	if !mediaLimit.allow(strconv.FormatInt(salonID, 10)) {
		return nil, errMediaLimit
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, mediaMaxBytes+1))
	if err != nil {
		return nil, err
	}
	return processImage(data, kind)
}

// galerieGespeichert: Erfolgsmeldungen je ?gespeichert=<code> (feste Texte).
var galerieGespeichert = map[string]string{
	"hochgeladen": "Fotos gespeichert. Deine Buchungsseite zeigt sie sofort.",
	"text":        "Beschreibung gespeichert.",
	"sortiert":    "Reihenfolge gespeichert.",
	"geloescht":   "Foto gelöscht.",
}

// galleryUploadMax: so viel darf ein Upload der Galerie zusammen haben.
const galleryUploadMax = 20 << 20

func (a *app) renderGallery(w http.ResponseWriter, r *http.Request, c *dctx, status int, msg, file string) {
	d := a.dashPage(c, r, "einstellungen", "Fotogalerie")
	d["PageTitle"], d["Heading"] = d["Heading"], ""
	items, err := a.gallery(c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	d["Items"], d["Max"], d["Left"], d["Pro"] = items, galleryMax, galleryMax-len(items), a.proFeatures(c.Salon)
	d["Error"], d["ErrorFile"] = tr(d["Lang"].(string), msg), file
	d["SavedMsg"] = galerieGespeichert[r.URL.Query().Get("gespeichert")]
	a.render(w, status, "einst-galerie", d)
}

func (a *app) galleryPage(w http.ResponseWriter, r *http.Request, c *dctx) {
	a.renderGallery(w, r, c, http.StatusOK, "", "")
}

// galleryUpload nimmt ein oder mehrere Fotos. Ist eins davon ungültig oder
// passt nicht mehr in die Galerie, wird keins gespeichert.
func (a *app) galleryUpload(w http.ResponseWriter, r *http.Request, c *dctx) {
	if !a.proFeatures(c.Salon) {
		a.renderGallery(w, r, c, http.StatusForbidden, "Die Fotogalerie gibt es mit Pro.", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, galleryUploadMax+64<<10)
	if err := r.ParseMultipartForm(64 << 10); err != nil {
		a.renderGallery(w, r, c, http.StatusBadRequest, uploadError(err, "Zu viele oder zu große Fotos auf einmal. Bitte höchstens 20 MB pro Upload."), "")
		return
	}
	files := r.MultipartForm.File["fotos"]
	items, err := a.gallery(c.Salon.ID)
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	switch {
	case len(files) == 0:
		a.renderGallery(w, r, c, http.StatusUnprocessableEntity, "Bitte mindestens ein Foto auswählen.", "")
		return
	case len(files) > galleryMax-len(items):
		a.renderGallery(w, r, c, http.StatusUnprocessableEntity, "So viele Fotos passen nicht mehr in die Galerie. Höchstens 12 Fotos, lösch vorher eins oder wähl weniger aus.", "")
		return
	}
	var imgs []*mediaImage
	for _, fh := range files {
		img, err := a.readImage(fh, "gallery", c.Salon.ID)
		if err != nil {
			a.renderGallery(w, r, c, http.StatusUnprocessableEntity, err.Error(), fh.Filename)
			return
		}
		imgs = append(imgs, img)
	}
	tx, err := a.db.Begin()
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	defer tx.Rollback()
	for _, img := range imgs {
		ok, err := mediaPut(tx, c.Salon.ID, "gallery", img)
		if err != nil {
			a.dashServerError(w, r, c, err)
			return
		}
		if !ok { // gleichzeitig in einem anderen Tab hochgeladen
			a.renderGallery(w, r, c, http.StatusConflict, "So viele Fotos passen nicht mehr in die Galerie. Höchstens 12 Fotos, lösch vorher eins oder wähl weniger aus.", "")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/einstellungen/galerie?gespeichert=hochgeladen", http.StatusSeeOther)
}

// galleryEdit ändert ein Foto des eigenen Salons: aktion = text (Beschreibung),
// vor, zurueck (Reihenfolge) oder loeschen. Geht auch ohne Pro, damit nach
// einem Wechsel zu Gratis aufgeräumt werden kann.
func (a *app) galleryEdit(w http.ResponseWriter, r *http.Request, c *dctx) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := r.ParseForm(); err != nil {
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	items, err := a.gallery(c.Salon.ID) // nur Fotos des eigenen Salons
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	i := slices.IndexFunc(items, func(g galleryItem) bool { return g.ID == id })
	if i < 0 {
		a.dashError(w, r, c, http.StatusNotFound, "Dieses Foto gibt es nicht (mehr).")
		return
	}
	code := ""
	switch r.PostFormValue("aktion") {
	case "text":
		alt := strings.TrimSpace(r.PostFormValue("alt"))
		if utf8.RuneCountInString(alt) > 150 || strings.ContainsFunc(alt, unicode.IsControl) {
			a.renderGallery(w, r, c, http.StatusUnprocessableEntity, "Die Beschreibung darf höchstens 150 Zeichen haben.", "")
			return
		}
		code = "text"
		_, err = a.db.Exec(`UPDATE salon_media SET alt = ? WHERE id = ? AND salon_id = ? AND kind = 'gallery'`, alt, id, c.Salon.ID)
	case "vor", "zurueck":
		j := i - 1
		if r.PostFormValue("aktion") == "zurueck" {
			j = i + 1
		}
		code = "sortiert"
		if j >= 0 && j < len(items) {
			items[i], items[j] = items[j], items[i]
			err = a.gallerySort(c.Salon.ID, items)
		}
	case "loeschen":
		code = "geloescht"
		_, err = a.db.Exec(`DELETE FROM salon_media WHERE id = ? AND salon_id = ? AND kind = 'gallery'`, id, c.Salon.ID)
	default:
		a.dashError(w, r, c, http.StatusBadRequest, "Das Formular war unvollständig.")
		return
	}
	if err != nil {
		a.dashServerError(w, r, c, err)
		return
	}
	http.Redirect(w, r, "/dashboard/einstellungen/galerie?gespeichert="+code, http.StatusSeeOther)
}

// gallerySort schreibt die Reihenfolge neu (0, 1, 2, ...).
func (a *app) gallerySort(salonID int64, items []galleryItem) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for n, g := range items {
		if _, err := tx.Exec(`UPDATE salon_media SET sort = ? WHERE id = ? AND salon_id = ?`, n, g.ID, salonID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
