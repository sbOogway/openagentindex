CREATE TABLE IF NOT EXISTS node (
    id         TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS observers (
    observer TEXT PRIMARY KEY,
    weight   REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS cards (
    card_hash  TEXT PRIMARY KEY,
    card_json  TEXT NOT NULL,
    first_seen INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS observations (
    id          TEXT PRIMARY KEY,
    observer    TEXT NOT NULL REFERENCES observers (observer),
    host        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('card', 'endpoint')),
    at          INTEGER NOT NULL,
    ok          INTEGER NOT NULL,
    card_hash   TEXT REFERENCES cards (card_hash),
    http_status INTEGER NOT NULL,
    latency_ms  INTEGER NOT NULL,
    error       TEXT NOT NULL
);
