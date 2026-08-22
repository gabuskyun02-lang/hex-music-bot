-- 0001_init: baseline schema.
-- Kept minimal by design; later migrations extend, never rewrite.

CREATE TABLE IF NOT EXISTS guild_settings (
    guild_id       TEXT PRIMARY KEY,
    default_volume INTEGER NOT NULL DEFAULT 100 CHECK (default_volume BETWEEN 0 AND 1000),
    created_at     TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS play_history (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    guild_id     TEXT NOT NULL,
    title        TEXT NOT NULL,
    uri          TEXT,
    author       TEXT,
    length_ms    INTEGER NOT NULL DEFAULT 0,
    requester_id TEXT NOT NULL DEFAULT '',
    played_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_play_history_guild_time
    ON play_history (guild_id, played_at DESC);
