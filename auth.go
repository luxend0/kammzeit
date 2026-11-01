package main

// Login per Magic-Link: /login, /login/{token}, /logout.

import "net/http"

func (a *app) routesAuth(mux *http.ServeMux) {}

// sendLoginLink mailt einen Einmal-Link (15 Minuten gültig) an das Konto.
// Wird auch von der Registrierung benutzt. Signatur nicht ändern.
func (a *app) sendLoginLink(userID int64, email string) error { return nil }
