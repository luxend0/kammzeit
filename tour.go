package main

// Geführte Tour durchs Dashboard: Schritte in web/tour.html, Ablauf in
// web/static/tour.js, Aussehen in web/static/tour.css, Texte in i18n_tour.go.
// Kein Server-Zustand: ob die Tour schon lief, steht im localStorage des Browsers.

import "reflect"

func init() {
	// {{if tourStaff .User}} = Mitarbeiter-Login (users.role = 'staff'): kürzere Tour
	// ohne Leistungen, Team, Einstellungen und Abo.
	templateFuncs["tourStaff"] = tourStaff
}

// tourStaff erkennt Mitarbeiter-Logins an einer Methode IsStaff() oder einem Feld
// Role == "staff". ponytail: per Reflection, damit die Tour vor und nach dem Einbau
// der Rollen (user.Role) ohne Änderung kompiliert; ohne Rolle gilt Inhaber.
func tourStaff(u any) bool {
	if s, ok := u.(interface{ IsStaff() bool }); ok {
		return s.IsStaff()
	}
	v := reflect.ValueOf(u)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	f := v.FieldByName("Role")
	return f.IsValid() && f.Kind() == reflect.String && f.String() == "staff"
}
