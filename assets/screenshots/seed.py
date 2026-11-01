#!/usr/bin/env python3
"""Legt eine SQLite-DB nur für die Website-Screenshots an: Salon "Salon Lindner"
(Slug lindner) mit erfundenen Mitarbeitern, Leistungen, Kunden und Terminen.

Aufruf: seed.py <db-pfad> <tag YYYY-MM-DD>
<tag> ist der Werktag, den die Screenshots als "heute" zeigen (shoot.sh stellt
die Uhr des Servers auf diesen Tag, 11:20 Uhr). Alle Termine liegen relativ dazu.
Namen, Adressen und Telefonnummern sind erfunden; E-Mails nutzen example.com,
Telefonnummern die Vorwahl 01234 (gibt es in Deutschland nicht).
"""
import secrets
import sqlite3
import sys
from datetime import date, datetime, timedelta
from pathlib import Path
from zoneinfo import ZoneInfo

TZ = ZoneInfo("Europe/Berlin")
LOGIN = "lindner@example.com"

# (Name, Beschreibung, Minuten, Cent)
SERVICES = [
    ("Damenhaarschnitt", "Waschen, Schneiden, Föhnen", 60, 5400),
    ("Herrenhaarschnitt", "Waschen, Schneiden, Styling", 30, 2900),
    ("Herrenhaarschnitt mit Bart", "Haarschnitt und Bart in Form", 45, 3900),
    ("Kinderhaarschnitt", "Bis 12 Jahre, Schneiden ohne Föhnen", 30, 1900),
    ("Ansatzfarbe", "Ansatz färben, Waschen, Föhnen", 90, 7200),
    ("Farbe und Schnitt", "Komplett färben, Schneiden, Föhnen", 120, 9800),
    ("Strähnen und Balayage", "Mit Beratung, Pflege und Föhnen", 150, 13500),
    ("Waschen und Föhnen", "Ohne Schnitt", 30, 2600),
    ("Pony nachschneiden", "Zwischen zwei Terminen", 15, 900),
    ("Hochsteckfrisur", "Nur nach Absprache", 60, 6500),  # inaktiv
]
INACTIVE = {"Hochsteckfrisur"}

# Wochentag wie time.Weekday: 0 = Sonntag. Blöcke in Minuten ab Mitternacht.
def hm(s):
    h, m = s.split(":")
    return int(h) * 60 + int(m)

STAFF = {
    "Jana": {
        "services": [0, 1, 3, 4, 5, 6, 7, 8],
        "hours": {wd: [("9:00", "13:00"), ("13:30", "18:00")] for wd in (2, 3, 4, 5)} | {6: [("8:30", "13:30")]},
    },
    "Murat": {
        "services": [0, 1, 2, 3, 7, 8],
        "hours": {wd: [("9:30", "13:30"), ("14:00", "18:00")] for wd in (1, 2, 3, 5)} | {6: [("8:30", "13:30")]},
    },
    "Sophie": {
        "services": [0, 3, 4, 5, 6, 7, 8],
        "hours": {wd: [("9:00", "12:30"), ("13:00", "16:00")] for wd in (2, 3, 4)}
        | {5: [("9:00", "14:00")], 6: [("8:30", "13:30")]},
    },
}

# Name -> (E-Mail ja/nein, Telefon, Kunde seit so vielen Tagen)
CUSTOMERS = {
    "Claudia Neumann": (True, "01234 47811", 230),
    "Sabine Krüger": (True, "", 410),
    "Katrin Weber": (True, "01234 52290", 220),
    "Stefan Meyer": (False, "01234 61437", 530),
    "Melanie Klein": (True, "", 95),
    "Nicole Braun": (True, "01234 38842", 160),
    "Markus Schmitt": (False, "01234 70215", 640),
    "Sandra Wagner": (True, "", 30),
    "Jan Wolf": (True, "01234 49903", 180),
    "Andrea Fischer": (False, "01234 55160", 365),
    "Frank Schäfer": (True, "", 270),
    "Anna Lange": (True, "01234 63328", 120),
    "Birgit Möller": (True, "01234 41576", 480),
    "Petra Hoffmann": (True, "", 210),
    "Simone Köhler": (False, "01234 58804", 700),
    "Lisa Hartmann": (True, "", 60),
    "Daniel Koch": (True, "01234 66219", 330),
    "Julia Schröder": (True, "", 140),
    "Mia Lehmann": (False, "01234 52817", 75),
}

# Der gezeigte Tag: (Mitarbeiter, Beginn, Leistung, Kunde, Status, Quelle)
TODAY = [
    ("Jana", "9:00", 0, "Sabine Krüger", "booked", "online"),
    ("Jana", "10:00", 5, "Katrin Weber", "booked", "online"),
    ("Jana", "12:00", 1, "Stefan Meyer", "booked", "manual"),
    ("Jana", "13:30", 6, "Melanie Klein", "booked", "online"),
    ("Jana", "16:00", 0, "Nicole Braun", "cancelled", "online"),
    ("Murat", "9:30", 1, "Markus Schmitt", "booked", "manual"),
    ("Murat", "10:00", 0, "Sandra Wagner", "noshow", "online"),
    ("Murat", "11:00", 1, "Jan Wolf", "booked", "online"),
    ("Murat", "14:00", 0, "Andrea Fischer", "booked", "manual"),
    ("Murat", "16:30", 2, "Frank Schäfer", "booked", "online"),
    ("Sophie", "9:00", 4, "Claudia Neumann", "booked", "online"),
    ("Sophie", "10:30", 0, "Anna Lange", "booked", "online"),
    ("Sophie", "14:00", 4, "Birgit Möller", "booked", "online"),
]

# Die nächsten Tage (Mittwoch, Donnerstag), damit die Buchungsseite echte Lücken
# zeigt. shoot.mjs bucht am Donnerstag.
LATER = {
    1: [
        ("Jana", "9:00", 5, "Petra Hoffmann", "booked", "online"),
        ("Jana", "13:30", 0, "Lisa Hartmann", "booked", "online"),
        ("Jana", "16:00", 1, "Daniel Koch", "booked", "manual"),
        ("Murat", "9:30", 2, "Daniel Koch", "cancelled", "online"),
        ("Murat", "11:00", 1, "Jan Wolf", "booked", "online"),
        ("Murat", "14:30", 0, "Simone Köhler", "booked", "manual"),
        ("Sophie", "9:00", 6, "Julia Schröder", "booked", "online"),
        ("Sophie", "13:00", 4, "Petra Hoffmann", "booked", "online"),
    ],
    2: [
        ("Jana", "9:00", 5, "Birgit Möller", "booked", "manual"),
        ("Jana", "13:30", 3, "Mia Lehmann", "booked", "manual"),
        ("Jana", "15:00", 1, "Markus Schmitt", "booked", "manual"),
        ("Sophie", "9:00", 6, "Lisa Hartmann", "booked", "online"),
        ("Sophie", "13:00", 4, "Simone Köhler", "booked", "online"),
    ],
}

# Terminhistorie für die Karteikarte: (Tage relativ zum gezeigten Tag, Mitarbeiter, Beginn, Leistung, Status)
CLAUDIA = [
    (-217, "Jana", "10:00", 5, "booked"),
    (-168, "Sophie", "9:00", 4, "booked"),
    (-133, "Sophie", "9:00", 4, "cancelled"),
    (-126, "Sophie", "9:00", 4, "booked"),
    (-84, "Jana", "9:00", 5, "booked"),
    (-42, "Sophie", "9:00", 4, "booked"),
    (42, "Sophie", "9:00", 4, "booked"),
]
CLAUDIA_NOTES = """Farbe: 7/1 + 6 % / 35 Min. Einwirkzeit
Längen nur auffrischen, Spitzen 1-2 cm.
Kopfhaut empfindlich, mildes Shampoo nehmen.
Trinkt Kaffee mit Hafermilch."""


def at(day, clock):
    h, m = map(int, clock.split(":"))
    return int(datetime(day.year, day.month, day.day, h, m, tzinfo=TZ).timestamp())


def main():
    db_path, shown = sys.argv[1], date.fromisoformat(sys.argv[2])
    schema = (Path(__file__).resolve().parents[2] / "schema.sql").read_text()
    Path(db_path).unlink(missing_ok=True)
    db = sqlite3.connect(db_path)
    db.executescript(schema)
    noon = at(shown, "11:20")  # "jetzt" laut shoot.sh

    cur = db.execute(
        """INSERT INTO salons (slug, name, email, plan, address, phone, about, imprint_url,
           min_notice_min, cancel_notice_min, booking_horizon_days, created_at)
           VALUES ('lindner', 'Salon Lindner', ?, 'pro', ?, '01234 56780', ?, 'https://salon-lindner.example/impressum',
           120, 1440, 60, ?)""",
        (
            LOGIN,
            "Bahnhofstraße 12\n12345 Musterstadt",
            "Friseursalon mitten in Musterstadt. Schnitt, Farbe und Pflege für Damen, Herren und Kinder.",
            noon - 400 * 86400,
        ),
    )
    salon = cur.lastrowid
    db.execute("INSERT INTO users (salon_id, email) VALUES (?, ?)", (salon, LOGIN))

    service_ids = []
    for i, (name, desc, minutes, cents) in enumerate(SERVICES):
        cur = db.execute(
            "INSERT INTO services (salon_id, name, description, duration_min, price_cents, sort, active) VALUES (?, ?, ?, ?, ?, ?, ?)",
            (salon, name, desc, minutes, cents, i, int(name not in INACTIVE)),
        )
        service_ids.append(cur.lastrowid)

    staff_ids = {}
    for i, (name, spec) in enumerate(STAFF.items()):
        cur = db.execute("INSERT INTO staff (salon_id, name, sort) VALUES (?, ?, ?)", (salon, name, i))
        staff_ids[name] = cur.lastrowid
        for s in spec["services"]:
            db.execute("INSERT INTO staff_services (staff_id, service_id) VALUES (?, ?)", (cur.lastrowid, service_ids[s]))
        for wd in sorted(spec["hours"]):
            for start, end in spec["hours"][wd]:
                db.execute(
                    "INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, ?, ?)",
                    (cur.lastrowid, wd, hm(start), hm(end)),
                )

    customer_ids = {}
    for name, (has_mail, phone, since) in CUSTOMERS.items():
        mail = None
        if has_mail:
            first, last = name.lower().split(" ", 1)
            mail = f"{first}.{last}@example.com".translate(str.maketrans({"ä": "ae", "ö": "oe", "ü": "ue", "ß": "ss"}))
        notes = CLAUDIA_NOTES if name == "Claudia Neumann" else ""
        cur = db.execute(
            "INSERT INTO customers (salon_id, name, email, phone, notes, created_at) VALUES (?, ?, ?, ?, ?, ?)",
            (salon, name, mail, phone, notes, noon - since * 86400),
        )
        customer_ids[name] = cur.lastrowid

    def appt(day, staff, clock, service, customer, status, source):
        start = at(day, clock)
        end = start + SERVICES[service][2] * 60
        # Erinnerung gilt als verschickt, damit der Server beim Aufnehmen keine Mails loggt.
        db.execute(
            """INSERT INTO appointments (salon_id, staff_id, service_id, customer_id, starts_at, ends_at,
               status, source, cancel_token, reminder_sent_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (salon, staff_ids[staff], service_ids[service], customer_ids[customer], start, end,
             status, source, secrets.token_hex(16), start - 86400, start - 10 * 86400),
        )

    for row in TODAY:
        appt(shown, *row)
    for offset, rows in LATER.items():
        for row in rows:
            appt(shown + timedelta(days=offset), *row)
    for days, staff, clock, service, status in CLAUDIA:
        appt(shown + timedelta(days=days), staff, clock, service, "Claudia Neumann", status, "online")

    db.commit()
    db.close()


if __name__ == "__main__":
    main()
