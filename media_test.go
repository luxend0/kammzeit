package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mediaPNG: Bild w x h, links rot, rechts blau; transparent = Rand durchsichtig.
func mediaPNG(t *testing.T, w, h int, transparent bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.NRGBA{255, 0, 0, 255}
			if x >= w/2 {
				c = color.NRGBA{0, 0, 255, 255}
			}
			if transparent && (x < w/10 || y < h/10) {
				c.A = 0
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// mediaPNGHeader: gültige PNG-Datei, deren Kopf w x h behauptet (Dekompressionsbombe).
func mediaPNGHeader(t *testing.T, w, h uint32) []byte {
	b := mediaPNG(t, 2, 2, true)
	binary.BigEndian.PutUint32(b[16:], w)
	binary.BigEndian.PutUint32(b[20:], h)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

// mediaExifJPEG: JPEG w x h (links rot, rechts blau) mit EXIF-Block: Orientation
// und eine Bildbeschreibung als Platzhalter für GPS- und Kameradaten.
func mediaExifJPEG(t *testing.T, w, h, orientation int, secret string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{255, 0, 0, 255}
			if x >= w/2 {
				c = color.RGBA{0, 0, 255, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var enc bytes.Buffer
	if err := jpeg.Encode(&enc, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	var tiff bytes.Buffer
	put := func(v any) { binary.Write(&tiff, le, v) }
	desc := secret + "\x00"
	tiff.WriteString("II*\x00")
	put(uint32(8))
	put(uint16(2))
	put(uint16(0x010e)) // ImageDescription, ASCII, Wert hinter dem IFD
	put(uint16(2))
	put(uint32(len(desc)))
	put(uint32(8 + 2 + 2*12 + 4))
	put(uint16(0x0112)) // Orientation, SHORT
	put(uint16(3))
	put(uint32(1))
	put(uint16(orientation))
	put(uint16(0))
	put(uint32(0))
	tiff.WriteString(desc)
	seg := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	out := []byte{0xff, 0xd8, 0xff, 0xe1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)}
	out = append(out, seg...)
	return append(out, enc.Bytes()[2:]...)
}

// WebP (verlustfrei, 1 Bit) aus golang.org/x/image/testdata, BSD-Lizenz.
const mediaWebP = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="

func TestProcessImage(t *testing.T) {
	var gifBuf bytes.Buffer
	gif.Encode(&gifBuf, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil)
	webpData, _ := base64.StdEncoding.DecodeString(mediaWebP)

	cases := []struct {
		name       string
		data       []byte
		kind       string
		err        error
		mime       string
		wantW, wid int
	}{
		{"PNG groß, Galerie", mediaPNG(t, 2000, 1000, false), "gallery", nil, "image/jpeg", 1600, 800},
		{"PNG Titelbild", mediaPNG(t, 2400, 900, false), "cover", nil, "image/jpeg", 1920, 720},
		{"PNG hochkant, Galerie", mediaPNG(t, 300, 2400, false), "gallery", nil, "image/jpeg", 200, 1600},
		{"Logo transparent bleibt PNG", mediaPNG(t, 600, 300, true), "logo", nil, "image/png", 512, 256},
		{"Galerie transparent wird JPEG auf Weiß", mediaPNG(t, 600, 300, true), "gallery", nil, "image/jpeg", 600, 300},
		{"kleines Logo wird nicht vergrößert", mediaPNG(t, 64, 32, false), "logo", nil, "image/jpeg", 64, 32},
		{"WebP", webpData, "gallery", nil, "", 0, 0},
		{"GIF", gifBuf.Bytes(), "logo", errMediaType, "", 0, 0},
		{"SVG", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`), "logo", errMediaType, "", 0, 0},
		{"HTML mit PNG-Endung", []byte("<html><script>alert(1)</script>"), "logo", errMediaType, "", 0, 0},
		{"JPEG kaputt", []byte("\xff\xd8\xff\xe0kaputt"), "logo", errMediaBroken, "", 0, 0},
		{"zu groß", append(mediaPNG(t, 4, 4, false), make([]byte, mediaMaxBytes)...), "logo", errMediaSize, "", 0, 0},
		{"Bombe: 7000 x 7000 RGBA", mediaPNGHeader(t, 7000, 7000), "gallery", errMediaPixels, "", 0, 0},
		{"Bombe: Kante 13000", mediaPNGHeader(t, 13000, 10), "gallery", errMediaPixels, "", 0, 0},
		{"Kopf ohne Pixel", mediaPNGHeader(t, 0, 10), "gallery", errMediaBroken, "", 0, 0},
	}
	for _, c := range cases {
		img, err := processImage(c.data, c.kind)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: Fehler %v, erwartet %v", c.name, err, c.err)
			continue
		}
		if err != nil {
			continue
		}
		if c.mime != "" && img.Mime != c.mime || c.wantW > 0 && (img.Width != c.wantW || img.Height != c.wid) {
			t.Errorf("%s: %s %dx%d, erwartet %s %dx%d", c.name, img.Mime, img.Width, img.Height, c.mime, c.wantW, c.wid)
		}
		// Ergebnis ist ein gültiges Bild in den angegebenen Maßen.
		cfg, format, err := image.DecodeConfig(bytes.NewReader(img.Data))
		if err != nil || "image/"+format != img.Mime || cfg.Width != img.Width || cfg.Height != img.Height {
			t.Errorf("%s: Ergebnis %s %dx%d, %v", c.name, format, cfg.Width, cfg.Height, err)
		}
	}
}

// EXIF (Orientation, Beschreibung, GPS ...) fällt beim Neukodieren weg; die
// Drehung wird vorher angewendet.
func TestProcessImageExif(t *testing.T) {
	in := mediaExifJPEG(t, 40, 20, 6, "GPS-GEHEIM-49.9876")
	if got := exifOrientation(in); got != 6 {
		t.Fatalf("exifOrientation = %d, erwartet 6", got)
	}
	img, err := processImage(in, "gallery")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(img.Data, []byte("Exif")) || bytes.Contains(img.Data, []byte("GEHEIM")) {
		t.Fatal("EXIF-Daten sind noch im Bild")
	}
	if img.Width != 20 || img.Height != 40 {
		t.Fatalf("nicht gedreht: %dx%d", img.Width, img.Height)
	}
	dec, err := jpeg.Decode(bytes.NewReader(img.Data))
	if err != nil {
		t.Fatal(err)
	}
	// 90° im Uhrzeigersinn: die linke (rote) Hälfte liegt oben.
	top, bottom := dec.At(10, 3), dec.At(10, 36)
	if r, _, b, _ := top.RGBA(); r < b {
		t.Errorf("oben nicht rot: %v", top)
	}
	if r, _, b, _ := bottom.RGBA(); b < r {
		t.Errorf("unten nicht blau: %v", bottom)
	}
	if exifOrientation(mediaPNG(t, 2, 2, false)) != 1 || exifOrientation([]byte("\xff\xd8\xff\xe1\x00")) != 1 {
		t.Error("ohne EXIF muss 1 herauskommen")
	}
}

func TestOrient(t *testing.T) {
	// 2x1: links rot, rechts blau.
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	src.SetRGBA(1, 0, color.RGBA{0, 0, 255, 255})
	red := color.RGBA{255, 0, 0, 255}
	cases := []struct {
		o          int
		w, h       int
		redX, redY int
	}{
		{1, 2, 1, 0, 0}, {2, 2, 1, 1, 0}, {3, 2, 1, 1, 0}, {4, 2, 1, 0, 0},
		{5, 1, 2, 0, 0}, {6, 1, 2, 0, 0}, {7, 1, 2, 0, 1}, {8, 1, 2, 0, 1},
	}
	for _, c := range cases {
		dst := orient(src, c.o)
		if dst.Bounds().Dx() != c.w || dst.Bounds().Dy() != c.h || dst.RGBAAt(c.redX, c.redY) != red {
			t.Errorf("Orientation %d: %v, rot bei (%d,%d)? %v", c.o, dst.Bounds(), c.redX, c.redY, dst.RGBAAt(c.redX, c.redY))
		}
	}
}

// --- über HTTP: Upload, Auslieferung, Pro-Gate, Mandantentrennung ---

func mediaPost(a *app, path, cookie string, fields map[string]string, files map[string][][]byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	for k, list := range files {
		for i, data := range list {
			fw, _ := mw.CreateFormFile(k, fmt.Sprintf("bild%d.png", i))
			fw.Write(data)
		}
	}
	mw.Close()
	r := httptest.NewRequest("POST", "https://kammzeit.test"+path, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", "https://kammzeit.test")
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

func mediaGet(a *app, host, path string, hdr ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "https://"+host+path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}

// mediaForm: gültiges Einstellungsformular plus Zusatzfelder.
func mediaForm(extra ...string) map[string]string {
	f := map[string]string{"name": "Salon Test", "accent": "#112233", "timezone": "Europe/Berlin", "min_notice": "60",
		"cancel_notice": "0", "horizon": "60", "lang": "de", "show_hours": "1"}
	for i := 0; i+1 < len(extra); i += 2 {
		f[extra[i]] = extra[i+1]
	}
	return f
}

func mediaHashOf(t *testing.T, a *app, salonID int64, kind string) string {
	t.Helper()
	var h string
	a.db.QueryRow(`SELECT hash FROM salon_media WHERE salon_id = ? AND kind = ? ORDER BY sort, id LIMIT 1`, salonID, kind).Scan(&h)
	return h
}

func TestMediaUploadServeAndTenants(t *testing.T) {
	a := einstApp(t)
	aID, aCookie := einstSeed(t, a.db, "alpha")
	bID, bCookie := einstSeed(t, a.db, "beta")
	a.db.Exec(`UPDATE salons SET plan = 'pro' WHERE id = ?`, aID)

	// Pro: Logo, Titelbild und Design werden gespeichert.
	w := mediaPost(a, "/dashboard/einstellungen", aCookie, mediaForm("theme_mode", "dark", "theme_radius", "round", "theme_font", "klassisch", "welcome", "Hallo!"),
		map[string][][]byte{"logo": {mediaPNG(t, 600, 300, true)}, "cover": {mediaPNG(t, 2400, 900, false)}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Pro speichern: %d\n%s", w.Code, w.Body)
	}
	sa, _ := a.loadSalon(aID)
	if sa.Logo == "" || sa.Cover == "" || sa.ThemeMode != "dark" || sa.ThemeRadius != "round" || sa.ThemeFont != "klassisch" || sa.Welcome != "Hallo!" || !sa.ShowHours {
		t.Fatalf("Pro nicht gespeichert: %+v", sa)
	}

	// Gratis: Logo ja, Titelbild und Design nein.
	w = mediaPost(a, "/dashboard/einstellungen", bCookie, mediaForm("theme_mode", "dark", "welcome", "Hallo!"),
		map[string][][]byte{"logo": {mediaPNG(t, 100, 100, false)}, "cover": {mediaPNG(t, 800, 300, false)}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Gratis speichern: %d\n%s", w.Code, w.Body)
	}
	sb, _ := a.loadSalon(bID)
	if sb.Logo == "" || sb.Cover != "" || sb.ThemeMode != "light" || sb.Welcome != "" {
		t.Fatalf("Gratis: Logo %q, Titelbild %q, Modus %q, Text %q", sb.Logo, sb.Cover, sb.ThemeMode, sb.Welcome)
	}

	// Falscher Dateityp: 422 mit Meldung am Feld, nichts gespeichert.
	w = mediaPost(a, "/dashboard/einstellungen", bCookie, mediaForm("name", "Neu"), map[string][][]byte{"logo": {[]byte("<svg/>")}})
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), errMediaType.Error()) || !strings.Contains(w.Body.String(), `id="einst-err-logo"`) {
		t.Fatalf("SVG: %d", w.Code)
	}
	if s, _ := a.loadSalon(bID); s.Name == "Neu" || s.Logo != sb.Logo {
		t.Fatal("trotz Fehler gespeichert")
	}

	// Auslieferung: Hauptdomain und eigene Subdomain ja, fremde Subdomain nein.
	path := "/media/" + sa.Logo
	w = mediaGet(a, "kammzeit.test", path)
	h := w.Header()
	if w.Code != 200 || h.Get("Content-Type") != "image/png" || h.Get("Cache-Control") != "public, max-age=31536000, immutable" ||
		h.Get("X-Content-Type-Options") != "nosniff" || h.Get("ETag") != `"`+sa.Logo+`"` || !bytes.HasPrefix(w.Body.Bytes(), []byte("\x89PNG")) {
		t.Fatalf("Logo ausliefern: %d %v", w.Code, h)
	}
	if w := mediaGet(a, "kammzeit.test", path, "If-None-Match", `"`+sa.Logo+`"`); w.Code != http.StatusNotModified {
		t.Errorf("If-None-Match: %d", w.Code)
	}
	if w := mediaGet(a, "kammzeit.test", "/media/"+sa.Cover); w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Titelbild: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := mediaGet(a, "alpha.kammzeit.test", path); w.Code != 200 {
		t.Errorf("eigene Subdomain: %d", w.Code)
	}
	for _, p := range []string{"beta.kammzeit.test" + path, "gibtsnicht.kammzeit.test" + path, "kammzeit.test/media/0123456789abcdef0123456789abcdef",
		"kammzeit.test/media/..%2fschema.sql", "kammzeit.test/media/" + strings.ToUpper(sa.Logo), "andere.example" + path} {
		host, p, _ := strings.Cut(p, "/")
		if w := mediaGet(a, host, "/"+p); w.Code != http.StatusNotFound {
			t.Errorf("%s/%s: %d, erwartet 404", host, p, w.Code)
		}
	}

	// Logo entfernen (eigenes), fremdes bleibt.
	if w := mediaPost(a, "/dashboard/einstellungen", bCookie, mediaForm("logo_weg", "1"), nil); w.Code != http.StatusSeeOther {
		t.Fatalf("Logo entfernen: %d", w.Code)
	}
	if s, _ := a.loadSalon(bID); s.Logo != "" {
		t.Error("Logo nicht entfernt")
	}
	if s, _ := a.loadSalon(aID); s.Logo != sa.Logo {
		t.Error("fremdes Logo verändert")
	}

	// Galerie: nur Pro lädt hoch.
	if w := mediaPost(a, "/dashboard/einstellungen/galerie", bCookie, nil, map[string][][]byte{"fotos": {mediaPNG(t, 50, 50, false)}}); w.Code != http.StatusForbidden {
		t.Errorf("Galerie Gratis: %d", w.Code)
	}
	w = mediaPost(a, "/dashboard/einstellungen/galerie", aCookie, nil, map[string][][]byte{"fotos": {mediaPNG(t, 50, 50, false), mediaPNG(t, 60, 40, false), mediaPNG(t, 70, 40, false)}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Galerie hochladen: %d\n%s", w.Code, w.Body)
	}
	items, _ := a.gallery(aID)
	if len(items) != 3 {
		t.Fatalf("Galerie: %d Fotos", len(items))
	}
	first, second := items[0], items[1]

	// Fremder Salon kann Fotos von alpha weder ändern noch löschen.
	for _, aktion := range []string{"text", "vor", "zurueck", "loeschen"} {
		w := einstDo(a, "POST", fmt.Sprintf("/dashboard/einstellungen/galerie/%d", first.ID), bCookie, map[string][]string{"aktion": {aktion}, "alt": {"gehackt"}})
		if w.Code != http.StatusNotFound {
			t.Errorf("fremd %s: %d", aktion, w.Code)
		}
	}
	if items, _ := a.gallery(aID); len(items) != 3 || items[0].Alt != "" || items[0].ID != first.ID {
		t.Fatalf("fremder Salon hat die Galerie verändert: %+v", items)
	}

	// Eigener Salon: Beschreibung, Reihenfolge, löschen.
	do := func(id int64, form map[string][]string) {
		t.Helper()
		if w := einstDo(a, "POST", fmt.Sprintf("/dashboard/einstellungen/galerie/%d", id), aCookie, form); w.Code != http.StatusSeeOther {
			t.Fatalf("%v: %d", form, w.Code)
		}
	}
	do(first.ID, map[string][]string{"aktion": {"text"}, "alt": {"Balayage in Honigblond"}})
	do(first.ID, map[string][]string{"aktion": {"zurueck"}})
	items, _ = a.gallery(aID)
	if items[0].ID != second.ID || items[1].ID != first.ID || items[1].Alt != "Balayage in Honigblond" {
		t.Fatalf("Reihenfolge/Text: %+v", items)
	}
	do(first.ID, map[string][]string{"aktion": {"loeschen"}})
	if items, _ = a.gallery(aID); len(items) != 2 {
		t.Fatalf("nach Löschen: %d", len(items))
	}
	if w := einstDo(a, "POST", fmt.Sprintf("/dashboard/einstellungen/galerie/%d", first.ID), aCookie, map[string][]string{"aktion": {"text"}, "alt": {strings.Repeat("x", 151)}}); w.Code != http.StatusNotFound {
		t.Errorf("gelöschtes Foto: %d", w.Code)
	}

	// Höchstens 12 Fotos.
	for i := range 10 {
		a.db.Exec(`INSERT INTO salon_media (salon_id, kind, mime, data, width, height, hash) VALUES (?, 'gallery', 'image/jpeg', x'00', 1, 1, ?)`, aID, fmt.Sprintf("%032d", i))
	}
	w = mediaPost(a, "/dashboard/einstellungen/galerie", aCookie, nil, map[string][][]byte{"fotos": {mediaPNG(t, 30, 30, false)}})
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Höchstens 12 Fotos") {
		t.Errorf("13. Foto: %d", w.Code)
	}

	// Export nennt die Bilder (ohne Inhalt), nur die eigenen.
	raw, _ := einstExport(t, a, aCookie)
	if !strings.Contains(raw, `"path": "/media/`+sa.Logo+`"`) || strings.Contains(raw, sb.Logo) && sb.Logo != "" {
		t.Errorf("Export: Bilder fehlen oder fremde dabei")
	}

	// Salon löschen räumt seine Bilder mit ab.
	a.db.Exec(`DELETE FROM salons WHERE id = ?`, aID)
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM salon_media WHERE salon_id = ?`, aID).Scan(&n)
	if n != 0 {
		t.Errorf("nach Löschen des Salons noch %d Bilder", n)
	}
}
