-- Wird bei jedem Start angewandt, daher nur IF NOT EXISTS.
-- ponytail: eine Datei statt Migrationssystem; mit der ersten Spaltenänderung
-- kommt eine Versionstabelle (PRAGMA user_version) dazu.

CREATE TABLE IF NOT EXISTS salons (
    id         INTEGER PRIMARY KEY,
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL DEFAULT '',
    accent     TEXT NOT NULL DEFAULT '#3b82f6',
    timezone   TEXT NOT NULL DEFAULT 'Europe/Berlin',
    plan       TEXT NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'pro')),
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS staff (
    id       INTEGER PRIMARY KEY,
    salon_id INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    name     TEXT NOT NULL,
    active   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS services (
    id           INTEGER PRIMARY KEY,
    salon_id     INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    duration_min INTEGER NOT NULL CHECK (duration_min > 0),
    price_cents  INTEGER NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
    active       INTEGER NOT NULL DEFAULT 1
);

-- Mehrere Blöcke pro Tag sind erlaubt (Mittagspause = zwei Blöcke).
CREATE TABLE IF NOT EXISTS working_hours (
    id        INTEGER PRIMARY KEY,
    staff_id  INTEGER NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
    weekday   INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6), -- 0 = Sonntag (wie time.Weekday)
    start_min INTEGER NOT NULL CHECK (start_min BETWEEN 0 AND 1440),
    end_min   INTEGER NOT NULL CHECK (end_min BETWEEN 0 AND 1440),
    CHECK (end_min > start_min)
);

-- Kundenkartei.
CREATE TABLE IF NOT EXISTS customers (
    id         INTEGER PRIMARY KEY,
    salon_id   INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL,
    phone      TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (salon_id, email)
);

CREATE TABLE IF NOT EXISTS appointments (
    id               INTEGER PRIMARY KEY,
    salon_id         INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    staff_id         INTEGER NOT NULL REFERENCES staff(id),
    service_id       INTEGER NOT NULL REFERENCES services(id),
    customer_id      INTEGER NOT NULL REFERENCES customers(id),
    starts_at        INTEGER NOT NULL, -- Unix-Sekunden
    ends_at          INTEGER NOT NULL,
    status           TEXT NOT NULL DEFAULT 'booked' CHECK (status IN ('booked', 'cancelled')),
    cancel_token     TEXT NOT NULL UNIQUE,
    reminder_sent_at INTEGER,
    created_at       INTEGER NOT NULL DEFAULT (unixepoch()),
    CHECK (ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS appointments_staff_time ON appointments (staff_id, starts_at);
CREATE INDEX IF NOT EXISTS appointments_reminder ON appointments (status, starts_at) WHERE reminder_sent_at IS NULL;
