#!/usr/bin/env python3
"""Füllt eine SQLite-DB nur für die Website-Screenshots: "Salon Lindner" (Slug
lindner, Deutsch) und "Lindner Hair Studio" (Slug lindner-hair, Englisch) mit
erfundenen Mitarbeitern, Leistungen, Kunden, Terminen, Warteschlange und Logo.
Beide Salons haben denselben Ablauf, nur Texte und Namen sind übersetzt.

Aufruf: seed.py <db-pfad> <tag YYYY-MM-DD>
Die DB muss Schema und Migrationen schon haben (shoot.sh legt sie mit der
Screenshot-Binary an). <tag> ist der Werktag, den die Screenshots als "heute"
zeigen (shoot.sh stellt die Uhr des Servers auf diesen Tag, 11:20 Uhr). Alle
Termine liegen relativ dazu. Namen, Adressen und Telefonnummern sind erfunden;
E-Mails nutzen example.com, Telefonnummern die Vorwahl 01234 bzw. +00 (gibt es nicht).
"""
import hashlib
import secrets
import sqlite3
import sys
from datetime import date, datetime, timedelta
from pathlib import Path
from zoneinfo import ZoneInfo

HERE = Path(__file__).resolve().parent

# (Name, Beschreibung, Minuten, Cent); englisch in derselben Reihenfolge
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
SERVICES_EN = [
    ("Women's haircut", "Wash, cut and blow-dry"),
    ("Men's haircut", "Wash, cut and styling"),
    ("Men's haircut and beard", "Haircut and beard trim"),
    ("Children's haircut", "Up to age 12, cut without blow-dry"),
    ("Root colour", "Root colour, wash and blow-dry"),
    ("Colour and cut", "Full colour, cut and blow-dry"),
    ("Highlights and balayage", "With consultation, care and blow-dry"),
    ("Wash and blow-dry", "No cut"),
    ("Fringe trim", "Between two appointments"),
    ("Updo", "By arrangement only"),
]
INACTIVE = 9  # Index der inaktiven Leistung

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

# Englischer Salon: Namen für Kunden in ganz Europa
NAMES_EN = {
    "Jana": "Emma", "Murat": "Luca", "Sophie": "Ines",
    "Claudia Neumann": "Claire Martin", "Sabine Krüger": "Sarah Jansen", "Katrin Weber": "Laura Rossi",
    "Stefan Meyer": "Tom Peeters", "Melanie Klein": "Megan Walsh", "Nicole Braun": "Nina Novak",
    "Markus Schmitt": "Marco Bianchi", "Sandra Wagner": "Camille Dubois", "Jan Wolf": "Jan de Vries",
    "Andrea Fischer": "Andrea Costa", "Frank Schäfer": "Felix Lambert", "Anna Lange": "Anna Larsen",
    "Birgit Möller": "Ingrid Olsen", "Petra Hoffmann": "Paula Moreau", "Simone Köhler": "Simone Ricci",
    "Lisa Hartmann": "Lisa Murphy", "Daniel Koch": "Daniel Silva", "Julia Schröder": "Julia Nowak",
    "Mia Lehmann": "Mia Horvat",
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

# Warteschlange des gezeigten Tages: (Status, de-Name, en-Name, Leistung, Mitarbeiter, angestellt um, E-Mail ja/nein)
QUEUE = [
    ("done", "Tobias", "Oliver", 1, "Murat", "9:40", False),
    ("done", "Elif", "Amira", 7, None, "10:05", True),
    ("serving", "Paul", "Noah", 2, "Murat", "10:35", False),
    ("called", "Leonie", "Lea", 8, None, "10:50", True),
    ("waiting", "Ben", "Leo", 3, None, "11:02", False),
    ("waiting", "Mara", "Maya", None, None, "11:09", True),
    ("waiting", "Kemal", "Karim", 1, "Murat", "11:15", False),
]

SALONS = {
    "de": dict(
        slug="lindner", name="Salon Lindner", login="lindner@example.com", tz="Europe/Berlin", phone="01234 56780",
        address="Bahnhofstraße 12\n12345 Musterstadt",
        about="Friseursalon mitten in Musterstadt. Schnitt, Farbe und Pflege für Damen, Herren und Kinder.",
        imprint="https://salon-lindner.example/impressum",
        notes="""Farbe: 7/1 + 6 % / 35 Min. Einwirkzeit
Längen nur auffrischen, Spitzen 1-2 cm.
Kopfhaut empfindlich, mildes Shampoo nehmen.
Trinkt Kaffee mit Hafermilch.""",
        queue_note="Bitte bleib in der Nähe, wir rufen dich auf.",
    ),
    "en": dict(
        slug="lindner-hair", name="Lindner Hair Studio", login="lindner.hair@example.com", tz="Europe/Amsterdam",
        phone="+00 123 456 780", address="12 Market Street\n1000 Sampletown",
        about="Hair salon in the centre of Sampletown. Cuts, colour and care for women, men and children.",
        imprint="https://lindner-hair.example/imprint",
        notes="""Colour: 7/1 + 6% / 35 min processing time
Refresh the lengths only, trim ends 1-2 cm.
Sensitive scalp, use a mild shampoo.
Likes coffee with oat milk.""",
        queue_note="Please stay nearby, we will call you.",
    ),
}


def seed(db, lang, shown):
    cfg = SALONS[lang]
    tz = ZoneInfo(cfg["tz"])

    def name(n):
        return NAMES_EN.get(n, n) if lang == "en" else n

    def at(day, clock):
        h, m = map(int, clock.split(":"))
        return int(datetime(day.year, day.month, day.day, h, m, tzinfo=tz).timestamp())

    noon = at(shown, "11:20")  # "jetzt" laut shoot.sh

    cur = db.execute(
        """INSERT INTO salons (slug, name, email, plan, address, phone, about, imprint_url, timezone, lang,
           min_notice_min, cancel_notice_min, booking_horizon_days, queue_enabled, queue_note, created_at)
           VALUES (?, ?, ?, 'pro', ?, ?, ?, ?, ?, ?, 120, 1440, 60, 1, ?, ?)""",
        (cfg["slug"], cfg["name"], cfg["login"], cfg["address"], cfg["phone"], cfg["about"], cfg["imprint"],
         cfg["tz"], lang, cfg["queue_note"], noon - 400 * 86400),
    )
    salon = cur.lastrowid
    # last_login_at gesetzt: kein Erstlogin, die Tour markiert shoot.mjs zusätzlich als gesehen
    db.execute("INSERT INTO users (salon_id, email, lang, last_login_at) VALUES (?, ?, ?, ?)",
               (salon, cfg["login"], lang, noon - 3600))
    logo = (HERE / "logo.png").read_bytes()
    digest = hashlib.sha256(f"{salon}:logo:".encode() + logo).hexdigest()[:32]  # wie mediaHash in media.go
    db.execute("INSERT INTO salon_media (salon_id, kind, mime, data, width, height, hash) VALUES (?, 'logo', 'image/png', ?, 256, 256, ?)",
               (salon, logo, digest))

    service_ids = []
    for i, (sname, desc, minutes, cents) in enumerate(SERVICES):
        if lang == "en":
            sname, desc = SERVICES_EN[i]
        cur = db.execute(
            "INSERT INTO services (salon_id, name, description, duration_min, price_cents, sort, active) VALUES (?, ?, ?, ?, ?, ?, ?)",
            (salon, sname, desc, minutes, cents, i, int(i != INACTIVE)),
        )
        service_ids.append(cur.lastrowid)

    staff_ids = {}
    for i, (sname, spec) in enumerate(STAFF.items()):
        cur = db.execute("INSERT INTO staff (salon_id, name, sort) VALUES (?, ?, ?)", (salon, name(sname), i))
        staff_ids[sname] = cur.lastrowid
        for s in spec["services"]:
            db.execute("INSERT INTO staff_services (staff_id, service_id) VALUES (?, ?)", (cur.lastrowid, service_ids[s]))
        for wd in sorted(spec["hours"]):
            for start, end in spec["hours"][wd]:
                db.execute(
                    "INSERT INTO working_hours (staff_id, weekday, start_min, end_min) VALUES (?, ?, ?, ?)",
                    (cur.lastrowid, wd, hm(start), hm(end)),
                )

    customer_ids = {}
    for cname, (has_mail, phone, since) in CUSTOMERS.items():
        shown_name = name(cname)
        mail = None
        if has_mail:
            local = shown_name.lower().replace(" ", ".", 1).replace(" ", "")  # jan.devries
            mail = f"{local}@example.com".translate(str.maketrans({"ä": "ae", "ö": "oe", "ü": "ue", "ß": "ss"}))
        if lang == "en" and phone:
            phone = phone.replace("01234 ", "+00 123 ")
        notes = cfg["notes"] if cname == "Claudia Neumann" else ""
        cur = db.execute(
            "INSERT INTO customers (salon_id, name, email, phone, notes, created_at) VALUES (?, ?, ?, ?, ?, ?)",
            (salon, shown_name, mail, phone, notes, noon - since * 86400),
        )
        customer_ids[cname] = cur.lastrowid

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

    for nr, (status, de_name, en_name, service, staff, clock, has_mail) in enumerate(QUEUE, 1):
        qname = en_name if lang == "en" else de_name
        created = at(shown, clock)
        called = created + 25 * 60 if status != "waiting" else None
        done = created + 50 * 60 if status == "done" else None
        db.execute(
            """INSERT INTO queue_entries (salon_id, number, day, name, email, service_id, staff_id, status, token,
               created_at, called_at, done_at, notified_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)""",
            (salon, nr, shown.isoformat(), qname, f"{qname.lower()}@example.com" if has_mail else None,
             service_ids[service] if service is not None else None, staff_ids[staff] if staff else None,
             status, secrets.token_hex(16), created, called, done, called),
        )


def main():
    db_path, shown = sys.argv[1], date.fromisoformat(sys.argv[2])
    db = sqlite3.connect(db_path)
    if db.execute("PRAGMA user_version").fetchone()[0] < 4:
        sys.exit("seed.py: DB ohne Migrationen, erst mit der Screenshot-Binary anlegen (siehe shoot.sh)")
    for lang in SALONS:
        seed(db, lang, shown)
    db.commit()
    db.close()


if __name__ == "__main__":
    main()
