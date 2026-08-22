-- 0002_persistence: taste profiles, playlists, snapshots, expanded settings.

ALTER TABLE guild_settings ADD COLUMN locale TEXT NOT NULL DEFAULT 'en';
ALTER TABLE guild_settings ADD COLUMN request_channel_id TEXT NOT NULL DEFAULT '';
ALTER TABLE guild_settings ADD COLUMN mode_247 INTEGER NOT NULL DEFAULT 0;
ALTER TABLE guild_settings ADD COLUMN autoplay INTEGER NOT NULL DEFAULT 0;
ALTER TABLE guild_settings ADD COLUMN autoplay_level TEXT NOT NULL DEFAULT 'normal';
ALTER TABLE guild_settings ADD COLUMN dj_role_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS user_taste (
    user_id   TEXT NOT NULL,
    artist    TEXT NOT NULL,
    weight    REAL NOT NULL DEFAULT 1.0,
    added_at  TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (user_id, artist)
);

CREATE TABLE IF NOT EXISTS playlists (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id   TEXT NOT NULL,
    name       TEXT NOT NULL,
    share_code TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_playlists_owner ON playlists (owner_id);

CREATE TABLE IF NOT EXISTS playlist_tracks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    identifier  TEXT NOT NULL,
    title       TEXT NOT NULL,
    author      TEXT NOT NULL DEFAULT '',
    length_ms   INTEGER NOT NULL DEFAULT 0,
    uri         TEXT NOT NULL DEFAULT '',
    position    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_playlist_tracks ON playlist_tracks (playlist_id, position);

CREATE TABLE IF NOT EXISTS player_snapshots (
    guild_id           TEXT PRIMARY KEY,
    text_channel_id    TEXT NOT NULL DEFAULT '',
    voice_channel_id   TEXT NOT NULL DEFAULT '',
    current_identifier TEXT NOT NULL DEFAULT '',
    current_position_ms INTEGER NOT NULL DEFAULT 0,
    queue              TEXT NOT NULL DEFAULT '[]',
    volume             INTEGER NOT NULL DEFAULT 100,
    loop_mode          TEXT NOT NULL DEFAULT 'off',
    saved_at           TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_play_history_user ON play_history (requester_id, played_at DESC);
