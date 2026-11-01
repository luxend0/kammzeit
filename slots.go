package main

import (
	"slices"
	"time"
)

// WorkingHours ist ein Arbeitsblock eines Mitarbeiters an einem Wochentag,
// in Minuten ab Mitternacht (Ortszeit des Salons).
type WorkingHours struct {
	Weekday  time.Weekday
	StartMin int
	EndMin   int
}

// Busy ist ein belegter Zeitraum [Start, End).
type Busy struct {
	Start, End time.Time
}

// freeSlots liefert alle Startzeiten am Tag `day` (dessen Location zählt), zu
// denen ein Termin der Länge dur ganz in einen Arbeitsblock passt, ohne einen
// belegten Zeitraum zu überlappen. Raster ab Blockbeginn im Abstand step.
// ponytail: O(slots * termine) pro Mitarbeiter und Tag, reicht bei < 100 Terminen am Tag.
func freeSlots(hours []WorkingHours, busy []Busy, dur time.Duration, day time.Time, step time.Duration) []time.Time {
	if dur <= 0 || step <= 0 {
		return nil
	}
	y, m, d := day.Date()
	loc := day.Location()
	var out []time.Time
	for _, h := range hours {
		if h.Weekday != day.Weekday() || h.EndMin <= h.StartMin {
			continue
		}
		// time.Date statt Mitternacht + Add, damit Sommer-/Winterzeit die Wanduhr nicht verschiebt.
		start := time.Date(y, m, d, h.StartMin/60, h.StartMin%60, 0, 0, loc)
		end := time.Date(y, m, d, h.EndMin/60, h.EndMin%60, 0, 0, loc)
		for t := start; !t.Add(dur).After(end); t = t.Add(step) {
			if !overlapsAny(t, t.Add(dur), busy) {
				out = append(out, t)
			}
		}
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	return slices.CompactFunc(out, func(a, b time.Time) bool { return a.Equal(b) })
}

func overlapsAny(start, end time.Time, busy []Busy) bool {
	for _, b := range busy {
		if start.Before(b.End) && b.Start.Before(end) {
			return true
		}
	}
	return false
}
