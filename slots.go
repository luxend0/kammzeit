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

// bookingDays: erster (heute) und letzter online buchbarer Tag, jeweils
// Mitternacht in der Zeitzone des Salons.
func bookingDays(s *salon, now time.Time) (first, last time.Time) {
	y, m, d := now.In(s.Loc).Date()
	first = time.Date(y, m, d, 0, 0, 0, 0, s.Loc)
	return first, first.AddDate(0, 0, max(s.HorizonDays, 0))
}

// slotsFor: freie Startzeiten am Tag `day`, vereinigt über alle Mitarbeiter in
// staff. Frühestens now + Vorlauf (min_notice_min), nur innerhalb des Horizonts.
func slotsFor(q querier, s *salon, staff []staffMember, dur time.Duration, day, now time.Time) ([]time.Time, error) {
	first, last := bookingDays(s, now)
	y, m, d := day.In(s.Loc).Date()
	day = time.Date(y, m, d, 0, 0, 0, 0, s.Loc)
	if day.Before(first) || day.After(last) {
		return nil, nil
	}
	earliest := now.Add(time.Duration(s.MinNoticeMin) * time.Minute)
	var all []time.Time
	for _, st := range staff {
		free, err := freeSlotsFor(q, st.ID, dur, day, earliest)
		if err != nil {
			return nil, err
		}
		all = append(all, free...)
	}
	slices.SortFunc(all, func(a, b time.Time) int { return a.Compare(b) })
	return slices.CompactFunc(all, func(a, b time.Time) bool { return a.Equal(b) }), nil
}

// pickStaff: der erste Mitarbeiter (Reihenfolge von staff), bei dem `start`
// unter allen Buchungsregeln frei ist, sonst nil.
func pickStaff(q querier, s *salon, staff []staffMember, dur time.Duration, start, now time.Time) (*staffMember, error) {
	for i := range staff {
		free, err := slotsFor(q, s, staff[i:i+1], dur, start, now)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(free, start.Equal) {
			return &staff[i], nil
		}
	}
	return nil, nil
}
