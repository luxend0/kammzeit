package main

// Bilder der Salons (Logo, Titelbild, Galerie): prüfen, verkleinern, neu
// kodieren, in salon_media speichern (BLOB, das Backup bleibt eine Datei) und
// unter /media/{hash} ausliefern. Angenommen werden nur PNG, JPEG und WebP
// (kein SVG: darin kann Skript stecken). Durch das neue Kodieren fallen alle
// Metadaten weg (EXIF, GPS, Kameradaten); die Drehung aus EXIF wird vorher
// angewendet, damit Handyfotos richtig herum stehen.

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	mediaMaxBytes = 5 << 20   // je Datei
	mediaMaxSide  = 12000     // Pixel je Kante vor dem Dekodieren
	mediaMaxMem   = 160 << 20 // geschätzter Speicher beim Dekodieren (Dekompressionsbombe)
	galleryMax    = 12        // Fotos je Salon
)

// mediaBox: größte Kantenlänge nach dem Verkleinern, je Art. Nie vergrößert.
var mediaBox = map[string]int{"logo": 512, "cover": 1920, "gallery": 1600}

// Fehlertexte (deutsch = Schlüssel, Übersetzung in i18n_custom.go).
var (
	errMediaType   = errors.New("Bitte ein Bild als JPG, PNG oder WebP hochladen.")
	errMediaSize   = errors.New("Das Bild ist zu groß. Höchstens 5 MB je Bild.")
	errMediaPixels = errors.New("Das Bild hat zu viele Pixel. Bitte vorher verkleinern (zum Beispiel auf 4000 Pixel Breite).")
	errMediaBroken = errors.New("Das Bild lässt sich nicht lesen. Bitte ein anderes versuchen.")
)

// mediaSem: höchstens ein Bild gleichzeitig dekodieren, das begrenzt den Speicher.
var mediaSem = make(chan struct{}, 1)

// mediaLimit: Bild-Uploads pro Salon (Rechenzeit).
var mediaLimit = newLimiter(60, time.Hour)

type mediaImage struct {
	Mime          string
	Data          []byte
	Width, Height int
}

// processImage prüft eine hochgeladene Datei und gibt sie verkleinert und neu
// kodiert zurück: JPEG (Qualität 82), ein transparentes Logo als PNG.
func processImage(data []byte, kind string) (*mediaImage, error) {
	if len(data) > mediaMaxBytes {
		return nil, errMediaSize
	}
	var decode func([]byte) (image.Image, error)
	var config func([]byte) (image.Config, error)
	switch {
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		decode = func(b []byte) (image.Image, error) { return jpeg.Decode(bytes.NewReader(b)) }
		config = func(b []byte) (image.Config, error) { return jpeg.DecodeConfig(bytes.NewReader(b)) }
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		decode = func(b []byte) (image.Image, error) { return png.Decode(bytes.NewReader(b)) }
		config = func(b []byte) (image.Config, error) { return png.DecodeConfig(bytes.NewReader(b)) }
	case len(data) > 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		decode = func(b []byte) (image.Image, error) { return webp.Decode(bytes.NewReader(b)) }
		config = func(b []byte) (image.Config, error) { return webp.DecodeConfig(bytes.NewReader(b)) }
	default:
		return nil, errMediaType
	}
	cfg, err := config(data)
	if err != nil {
		return nil, errMediaBroken
	}
	if cfg.Width < 1 || cfg.Height < 1 {
		return nil, errMediaBroken
	}
	if cfg.Width > mediaMaxSide || cfg.Height > mediaMaxSide || cfg.Width*cfg.Height*bytesPerPixel(cfg.ColorModel) > mediaMaxMem {
		return nil, errMediaPixels
	}
	mediaSem <- struct{}{}
	defer func() { <-mediaSem }()
	src, err := decode(data)
	if err != nil {
		return nil, errMediaBroken
	}
	box := mediaBox[kind]
	w, h := fitBox(cfg.Width, cfg.Height, box)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	op := draw.Src
	if kind != "logo" { // Titelbild und Galerie sind Fotos: Transparenz auf Weiß, immer JPEG
		draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
		op = draw.Over
	}
	if w == cfg.Width && h == cfg.Height {
		draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, op)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), op, nil)
	}
	src = nil
	if bytes.HasPrefix(data, []byte("\xff\xd8")) {
		dst = orient(dst, exifOrientation(data)) // erst nach dem Verkleinern: weniger Pixel
	}
	var out bytes.Buffer
	img := &mediaImage{Mime: "image/jpeg", Width: dst.Bounds().Dx(), Height: dst.Bounds().Dy()}
	if dst.Opaque() {
		err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82})
	} else {
		img.Mime = "image/png"
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, dst)
	}
	if err != nil {
		return nil, err
	}
	img.Data = out.Bytes()
	return img, nil
}

// bytesPerPixel schätzt den Speicher je Pixel des dekodierten Bildes.
func bytesPerPixel(m color.Model) int {
	switch m {
	case color.RGBA64Model, color.NRGBA64Model:
		return 8
	case color.GrayModel, color.AlphaModel:
		return 1
	case color.Gray16Model, color.Alpha16Model:
		return 2
	}
	if _, ok := m.(color.Palette); ok {
		return 1
	}
	return 4
}

// fitBox: Maße, die in ein Quadrat der Kantenlänge box passen (Seitenverhältnis bleibt).
func fitBox(w, h, box int) (int, int) {
	if w <= box && h <= box {
		return w, h
	}
	if w >= h {
		return box, max(1, (h*box+w/2)/w)
	}
	return max(1, (w*box+h/2)/h), box
}

// exifOrientation liest den Orientation-Tag (1-8) aus dem EXIF-Block einer
// JPEG-Datei, 1 = normal oder unbekannt.
func exifOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return 1
	}
	for i := 2; i+4 <= len(b); {
		if b[i] != 0xff {
			return 1
		}
		marker := b[i+1]
		switch {
		case marker == 0xff: // Füllbyte
			i++
			continue
		case marker == 0x01 || marker >= 0xd0 && marker <= 0xd8:
			i += 2
			continue
		case marker == 0xda || marker == 0xd9: // Bilddaten: kein EXIF mehr
			return 1
		}
		n := int(binary.BigEndian.Uint16(b[i+2:]))
		if n < 2 || i+2+n > len(b) {
			return 1
		}
		if seg := b[i+4 : i+2+n]; marker == 0xe1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		i += 2 + n
	}
	return 1
}

func tiffOrientation(t []byte) int {
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	if len(t) < 8 {
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	for k := range int(bo.Uint16(t[off:])) {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 {
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient dreht/spiegelt ein Bild nach EXIF-Orientation (2-8), damit es so
// steht, wie die Kamera es angezeigt hat.
func orient(src *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int
			switch o {
			case 2:
				sx, sy = w-1-x, y
			case 3:
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6: // 90° im Uhrzeigersinn
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8: // 90° gegen den Uhrzeigersinn
				sx, sy = w-1-y, x
			}
			si, di := src.PixOffset(sx, sy), dst.PixOffset(x, y)
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

// mediaHash = Adresse des Bildes. Salon und Art gehen mit ein: zwei Salons mit
// demselben Logo bekommen verschiedene Adressen.
func mediaHash(salonID int64, kind string, data []byte) string {
	h := sha256.New()
	h.Write([]byte(strconv.FormatInt(salonID, 10) + ":" + kind + ":"))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// mediaPut speichert ein Bild. Logo und Titelbild ersetzen das bisherige; die
// Galerie nimmt höchstens galleryMax Fotos (false = voll). Dasselbe Foto noch
// einmal in die Galerie zu laden ändert nichts.
func mediaPut(tx *sql.Tx, salonID int64, kind string, img *mediaImage) (bool, error) {
	if kind == "gallery" {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM salon_media WHERE salon_id = ? AND kind = 'gallery'`, salonID).Scan(&n); err != nil {
			return false, err
		}
		if n >= galleryMax {
			return false, nil
		}
	} else if _, err := tx.Exec(`DELETE FROM salon_media WHERE salon_id = ? AND kind = ?`, salonID, kind); err != nil {
		return false, err
	}
	_, err := tx.Exec(`INSERT INTO salon_media (salon_id, kind, mime, data, width, height, hash, sort)
		VALUES (?, ?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(sort), 0) + 1 FROM salon_media WHERE salon_id = ? AND kind = ?))
		ON CONFLICT (hash) DO NOTHING`,
		salonID, kind, img.Mime, img.Data, img.Width, img.Height, mediaHash(salonID, kind, img.Data), salonID, kind)
	return err == nil, err
}

var mediaHashRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (a *app) routesMedia(mux *http.ServeMux) {
	mux.HandleFunc("GET /media/{hash}", a.mediaServe)
}

// mediaServe liefert ein Bild aus. Auf einer Salon-Subdomain nur Bilder dieses
// Salons, auf der Hauptdomain alle (für Mails und Dashboard). Die Adresse ändert
// sich mit dem Inhalt, daher darf alles ein Jahr lang zwischengespeichert werden.
func (a *app) mediaServe(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if !mediaHashRe.MatchString(hash) {
		http.NotFound(w, r)
		return
	}
	q, args := `SELECT mime, data, created_at FROM salon_media WHERE hash = ?`, []any{hash}
	if !a.cfg.SingleTenant {
		slug, root, _ := a.tenant(r.Host)
		switch {
		case slug != "":
			q += ` AND salon_id = (SELECT id FROM salons WHERE slug = ?)`
			args = append(args, slug)
		case !root:
			http.NotFound(w, r)
			return
		}
	}
	var mime string
	var data []byte
	var created int64
	err := a.db.QueryRow(q, args...).Scan(&mime, &data, &created)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, "Fehler", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", mime) // nur image/jpeg oder image/png (CHECK in der Tabelle)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("ETag", `"`+hash+`"`)
	http.ServeContent(w, r, "", time.Unix(created, 0), bytes.NewReader(data))
}
