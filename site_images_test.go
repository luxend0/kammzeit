package main

import (
	"bytes"
	"html/template"
	"image"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"testing"

	_ "golang.org/x/image/webp"
)

// Bilder auf den öffentlichen Seiten: jede Datei existiert, width/height haben
// das Seitenverhältnis der Datei, und die englischen Seiten zeigen keine Bilder
// mit deutschem Text (Screenshots aus img/shots/en/, Illustrationen *-en.webp),
// die deutschen keine englischen.
func TestSiteImages(t *testing.T) {
	db, err := openDB(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(webFS, "web/*.html")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{cfg: config{BaseDomain: "kammzeit.test", SiteURL: "https://kammzeit.test",
		Domains: parseDomains("kammzeit.test=de,kammzeit-en.test=en", "")}, db: db, tpl: tpl}
	srv := httptest.NewServer(a.routes())
	defer srv.Close()

	paths := map[string][]string{"de": {"/", "/login", "/registrieren"}, "en": {"/", "/login", "/signup"}}
	for p, byLang := range a.pages {
		for lang := range byLang {
			paths[lang] = append(paths[lang], p)
		}
	}
	host := map[string]string{"de": "kammzeit.test", "en": "kammzeit-en.test"}
	// Bilder mit eingebranntem deutschem Text bzw. ihre englischen Fassungen
	germanText := regexp.MustCompile(`^/static/img/(shots/[^/]+\.webp|ill/(kalender|kartei|schritt-entdecken|schritt-erinnerung)\.webp)$`)
	englishText := regexp.MustCompile(`^/static/img/(shots/en/|ill/[^/]+-en\.webp$)`)
	imgRe := regexp.MustCompile(`<img [^>]*>`)
	attr := func(tag, name string) string {
		m := regexp.MustCompile(`\s` + name + `="([^"]*)"`).FindStringSubmatch(tag)
		if m == nil {
			return ""
		}
		return m[1]
	}
	sizes := map[string]image.Config{}
	n := 0
	for _, lang := range []string{"de", "en"} {
		sort.Strings(paths[lang])
		for _, p := range paths[lang] {
			req, _ := http.NewRequest("GET", srv.URL+p, nil)
			req.Host = host[lang]
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Errorf("%s%s: Status %d", host[lang], p, res.StatusCode)
				continue
			}
			for _, tag := range imgRe.FindAllString(string(body), -1) {
				src := attr(tag, "src")
				if !regexp.MustCompile(`^/static/.*\.(webp|png)$`).MatchString(src) {
					continue // SVG-Icons, Salonbilder
				}
				n++
				if lang == "en" && germanText.MatchString(src) {
					t.Errorf("%s%s: Bild mit deutschem Text %s", host[lang], p, src)
				}
				if lang == "de" && englishText.MatchString(src) {
					t.Errorf("%s%s: englisches Bild %s", host[lang], p, src)
				}
				cfg, ok := sizes[src]
				if !ok {
					data, err := webFS.ReadFile("web" + src)
					if err != nil {
						t.Errorf("%s%s: %s fehlt", host[lang], p, src)
						continue
					}
					if cfg, _, err = image.DecodeConfig(bytes.NewReader(data)); err != nil {
						t.Errorf("%s: %v", src, err)
						continue
					}
					sizes[src] = cfg
				}
				w, _ := strconv.Atoi(attr(tag, "width"))
				h, _ := strconv.Atoi(attr(tag, "height"))
				if w == 0 || h == 0 {
					t.Errorf("%s%s: %s ohne width/height", host[lang], p, src)
				} else if got, want := float64(w)/float64(h), float64(cfg.Width)/float64(cfg.Height); math.Abs(got/want-1) > 0.01 {
					t.Errorf("%s%s: %s hat width/height %dx%d, die Datei ist %dx%d", host[lang], p, src, w, h, cfg.Width, cfg.Height)
				}
			}
		}
	}
	if n < 50 {
		t.Errorf("nur %d Bilder geprüft, Seiten fehlen?", n)
	}
}
