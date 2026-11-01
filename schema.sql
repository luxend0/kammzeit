-- Grundschema, wird bei jedem Start angewandt (IF NOT EXISTS).
-- Änderungen an bestehenden Tabellen NICHT hier, sondern als neuer Eintrag in
-- `migrations` (main.go), gezählt über PRAGMA user_version.

CREATE TABLE IF NOT EXISTS salons (
    id                     INTEGER PRIMARY KEY,
    slug                   TEXT NOT NULL UNIQUE,
    name                   TEXT NOT NULL,
    email                  TEXT NOT NULL DEFAULT '',   -- bekommt Buchungs-Infos
    accent                 TEXT NOT NULL DEFAULT '#3b82f6',
    timezone               TEXT NOT NULL DEFAULT 'Europe/Berlin',
    plan                   TEXT NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'pro')),
    address                TEXT NOT NULL DEFAULT '',   -- mehrzeilig, auf der Buchungsseite
    phone                  TEXT NOT NULL DEFAULT '',
    website                TEXT NOT NULL DEFAULT '',
    imprint_url            TEXT NOT NULL DEFAULT '',   -- Impressum des Salons
    about                  TEXT NOT NULL DEFAULT '',   -- kurzer Text auf der Buchungsseite
    min_notice_min         INTEGER NOT NULL DEFAULT 60,    -- frühestens so viele Minuten im Voraus buchbar
    cancel_notice_min      INTEGER NOT NULL DEFAULT 0,     -- Absage online bis so viele Minuten vorher
    booking_horizon_days   INTEGER NOT NULL DEFAULT 60,    -- so weit im Voraus buchbar
    stripe_customer_id     TEXT NOT NULL DEFAULT '',
    stripe_subscription_id TEXT NOT NULL DEFAULT '',
    created_at             INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS staff (
    id       INTEGER PRIMARY KEY,
    salon_id INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    name     TEXT NOT NULL,
    sort     INTEGER NOT NULL DEFAULT 0,
    active   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS services (
    id           INTEGER PRIMARY KEY,
    salon_id     INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    duration_min INTEGER NOT NULL CHECK (duration_min > 0),
    price_cents  INTEGER NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
    sort         INTEGER NOT NULL DEFAULT 0,
    active       INTEGER NOT NULL DEFAULT 1
);

-- Welcher Mitarbeiter welche Leistung anbietet. Ohne Zeile: nicht buchbar.
-- Neue Leistung -> alle aktiven Mitarbeiter, neuer Mitarbeiter -> alle Leistungen.
CREATE TABLE IF NOT EXISTS staff_services (
    staff_id   INTEGER NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
    service_id INTEGER NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    PRIMARY KEY (staff_id, service_id)
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

-- Kundenkartei. email ist NULL bei Laufkundschaft ohne Mail (mehrere NULL erlaubt).
CREATE TABLE IF NOT EXISTS customers (
    id         INTEGER PRIMARY KEY,
    salon_id   INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    email      TEXT,
    phone      TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (salon_id, email)
);

CREATE TABLE IF NOT EXISTS appointments (
    id               INTEGER PRIMARY KEY,
    salon_id         INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    staff_id         INTEGER NOT NULL REFERENCES staff(id) ON DELETE CASCADE,
    service_id       INTEGER NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    customer_id      INTEGER NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    starts_at        INTEGER NOT NULL, -- Unix-Sekunden
    ends_at          INTEGER NOT NULL,
    status           TEXT NOT NULL DEFAULT 'booked' CHECK (status IN ('booked', 'cancelled', 'noshow')),
    source           TEXT NOT NULL DEFAULT 'online' CHECK (source IN ('online', 'manual')),
    note             TEXT NOT NULL DEFAULT '', -- interne Notiz des Salons
    cancel_token     TEXT NOT NULL UNIQUE,
    reminder_sent_at INTEGER,
    created_at       INTEGER NOT NULL DEFAULT (unixepoch()),
    CHECK (ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS appointments_staff_time ON appointments (staff_id, starts_at);
CREATE INDEX IF NOT EXISTS appointments_salon_time ON appointments (salon_id, starts_at);
CREATE INDEX IF NOT EXISTS appointments_customer ON appointments (customer_id);
CREATE INDEX IF NOT EXISTS appointments_reminder ON appointments (status, starts_at) WHERE reminder_sent_at IS NULL;

-- Konten (MVP: ein Inhaber pro Salon). Login nur per Magic-Link.
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY,
    salon_id      INTEGER NOT NULL REFERENCES salons(id) ON DELETE CASCADE,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    created_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    last_login_at INTEGER
);

-- Einmal-Links, nur als SHA-256 gespeichert.
CREATE TABLE IF NOT EXISTS login_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Stripe-Webhooks nur einmal verarbeiten.
CREATE TABLE IF NOT EXISTS stripe_events (
    id          TEXT PRIMARY KEY,
    received_at INTEGER NOT NULL DEFAULT (unixepoch())
);
