// Store repositories for Phase 4: guild settings, play history, taste
// profiles, playlists, and player snapshots.
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// --- Guild Settings ---

type GuildSettings struct {
	GuildID          string
	DefaultVolume    int
	Locale           string
	RequestChannelID string
	Mode247          bool
	Autoplay         bool
	AutoplayLevel    string
	DJRoleID         string
	AllowDuplicate   bool
}
func (s *Store) GetGuildSettings(ctx context.Context, guildID string) (*GuildSettings, error) {
	const q = `SELECT default_volume, locale, request_channel_id, mode_247, autoplay, autoplay_level, dj_role_id, duplicate_track
		FROM guild_settings WHERE guild_id = ?`
	var gs GuildSettings
	gs.GuildID = guildID
	err := s.db.QueryRowContext(ctx, q, guildID).Scan(
		&gs.DefaultVolume, &gs.Locale, &gs.RequestChannelID,
		&gs.Mode247, &gs.Autoplay, &gs.AutoplayLevel, &gs.DJRoleID, &gs.AllowDuplicate,
	)
	if err == sql.ErrNoRows {
		_, iErr := s.db.ExecContext(ctx,
			`INSERT INTO guild_settings (guild_id) VALUES (?) ON CONFLICT DO NOTHING`, guildID)
		if iErr != nil {
			return nil, fmt.Errorf("ensure guild settings: %w", iErr)
		}
		err = s.db.QueryRowContext(ctx, q, guildID).Scan(
			&gs.DefaultVolume, &gs.Locale, &gs.RequestChannelID,
			&gs.Mode247, &gs.Autoplay, &gs.AutoplayLevel, &gs.DJRoleID, &gs.AllowDuplicate,
		)
		if err != nil {
			return nil, fmt.Errorf("re-read guild settings: %w", err)
		}
		return &gs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get guild settings: %w", err)
	}
	return &gs, nil
}

func (s *Store) SetGuild247(ctx context.Context, guildID string, enabled bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, mode_247) VALUES (?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET mode_247 = ?, updated_at = datetime('now')`,
		guildID, enabled, enabled)
	return err
}

func (s *Store) SetGuildRequestChannel(ctx context.Context, guildID, channelID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, request_channel_id) VALUES (?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET request_channel_id = ?, updated_at = datetime('now')`,
		guildID, channelID, channelID)
	return err
}

func (s *Store) SetGuildAutoplay(ctx context.Context, guildID string, enabled bool, level string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, autoplay, autoplay_level) VALUES (?, ?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET autoplay = ?, autoplay_level = ?, updated_at = datetime('now')`,
		guildID, enabled, level, enabled, level)
	return err
}

func (s *Store) SetGuildDJRole(ctx context.Context, guildID, roleID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, dj_role_id) VALUES (?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET dj_role_id = ?, updated_at = datetime('now')`,
		guildID, roleID, roleID)
	return err
}

func (s *Store) SetGuildLanguage(ctx context.Context, guildID, lang string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, locale) VALUES (?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET locale = ?, updated_at = datetime('now')`,
		guildID, lang, lang)
	return err
}

func (s *Store) SetGuildDuplicateTrack(ctx context.Context, guildID string, allow bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO guild_settings (guild_id, duplicate_track) VALUES (?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET duplicate_track = ?, updated_at = datetime('now')`,
		guildID, allow, allow)
	return err
}

type PlayRecord struct {
	Title       string
	URI         string
	Author      string
	LengthMS    int64
	RequesterID string
	PlayedAt    time.Time
}

func (s *Store) RecordPlay(ctx context.Context, guildID, title, uri, author string, lengthMS int64, requesterID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO play_history (guild_id, title, uri, author, length_ms, requester_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		guildID, title, uri, author, lengthMS, requesterID)
	return err
}

func scanPlayRecords(rows *sql.Rows) ([]PlayRecord, error) {
	defer rows.Close()
	var out []PlayRecord
	for rows.Next() {
		var r PlayRecord
		var playedAt string
		if err := rows.Scan(&r.Title, &r.URI, &r.Author, &r.LengthMS, &r.RequesterID, &playedAt); err != nil {
			return nil, err
		}
		r.PlayedAt, _ = time.Parse("2006-01-02 15:04:05", playedAt)
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RecentPlays(ctx context.Context, guildID string, limit int) ([]PlayRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT title, uri, author, length_ms, requester_id, played_at
		 FROM play_history WHERE guild_id = ? ORDER BY played_at DESC LIMIT ?`,
		guildID, limit)
	if err != nil {
		return nil, err
	}
	return scanPlayRecords(rows)
}

func (s *Store) UserPlays(ctx context.Context, guildID, userID string, limit int) ([]PlayRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT title, uri, author, length_ms, requester_id, played_at
		 FROM play_history WHERE guild_id = ? AND requester_id = ? ORDER BY played_at DESC LIMIT ?`,
		guildID, userID, limit)
	if err != nil {
		return nil, err
	}
	return scanPlayRecords(rows)
}

// SeedArtists returns the most-played artists for autoplay seeding.
func (s *Store) SeedArtists(ctx context.Context, guildID string, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT author, COUNT(*) as plays FROM play_history
		 WHERE guild_id = ? AND author != '' GROUP BY author ORDER BY plays DESC LIMIT ?`,
		guildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var author string
		var plays int
		if err := rows.Scan(&author, &plays); err != nil {
			return nil, err
		}
		out = append(out, author)
	}
	return out, rows.Err()
}

// --- Taste Profiles ---

type TasteArtist struct {
	Artist string
	Weight float64
}

func (s *Store) AddTasteArtist(ctx context.Context, userID, artist string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_taste (user_id, artist) VALUES (?, ?)
		 ON CONFLICT(user_id, artist) DO UPDATE SET weight = MIN(weight + 0.5, 5.0)`,
		userID, artist)
	return err
}

func (s *Store) RemoveTasteArtist(ctx context.Context, userID, artist string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_taste WHERE user_id = ? AND artist = ?`, userID, artist)
	return err
}

func (s *Store) TasteArtists(ctx context.Context, userID string) ([]TasteArtist, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT artist, weight FROM user_taste WHERE user_id = ? ORDER BY weight DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TasteArtist
	for rows.Next() {
		var t TasteArtist
		if err := rows.Scan(&t.Artist, &t.Weight); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// BlendedTasteArtists returns the union of preferred artists for multiple
// users, ordered by combined weight. Used by autoplay when several people
// are in voice — everyone's taste contributes.
func (s *Store) BlendedTasteArtists(ctx context.Context, userIDs []string) ([]TasteArtist, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(userIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT artist, SUM(weight) as total FROM user_taste
		 WHERE user_id IN (`+placeholders+`) GROUP BY artist ORDER BY total DESC LIMIT 20`,
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TasteArtist
	for rows.Next() {
		var t TasteArtist
		if err := rows.Scan(&t.Artist, &t.Weight); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Player Snapshots (24/7 restart survival) ---

type PlayerSnapshot struct {
	GuildID           string
	TextChannelID     string
	VoiceChannelID    string
	CurrentIdentifier string
	CurrentPositionMS int64
	Queue             []string
	Volume            int
	LoopMode          string
}

func (s *Store) SavePlayerSnapshot(ctx context.Context, snap *PlayerSnapshot) error {
	queueJSON, err := json.Marshal(snap.Queue)
	if err != nil {
		return fmt.Errorf("marshal queue: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO player_snapshots (guild_id, text_channel_id, voice_channel_id, current_identifier, current_position_ms, queue, volume, loop_mode)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(guild_id) DO UPDATE SET
		   text_channel_id = ?, voice_channel_id = ?, current_identifier = ?,
		   current_position_ms = ?, queue = ?, volume = ?, loop_mode = ?,
		   saved_at = datetime('now')`,
		snap.GuildID, snap.TextChannelID, snap.VoiceChannelID,
		snap.CurrentIdentifier, snap.CurrentPositionMS, string(queueJSON),
		snap.Volume, snap.LoopMode,
		snap.TextChannelID, snap.VoiceChannelID, snap.CurrentIdentifier,
		snap.CurrentPositionMS, string(queueJSON), snap.Volume, snap.LoopMode,
	)
	return err
}

func (s *Store) DeletePlayerSnapshot(ctx context.Context, guildID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM player_snapshots WHERE guild_id = ?`, guildID)
	return err
}

// --- Playlists ---

type PlaylistMeta struct {
	ID        int64
	OwnerID   string
	Name      string
	ShareCode string
	TrackCnt  int
	CreatedAt time.Time
}

type PlaylistTrack struct {
	Identifier string
	Title      string
	Author     string
	LengthMS   int64
	URI        string
}

type Playlist struct {
	ID        int64
	OwnerID   string
	Name      string
	ShareCode string
	Tracks    []PlaylistTrack
}

func generateShareCode() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) CreatePlaylist(ctx context.Context, ownerID, name string) (*PlaylistMeta, error) {
	code := generateShareCode()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO playlists (owner_id, name, share_code) VALUES (?, ?, ?)`,
		ownerID, name, code)
	if err != nil {
		return nil, fmt.Errorf("create playlist: %w", err)
	}
	id, _ := res.LastInsertId()
	return &PlaylistMeta{ID: id, OwnerID: ownerID, Name: name, ShareCode: code}, nil
}

func (s *Store) GetPlaylistByCode(ctx context.Context, code string) (*Playlist, error) {
	var p Playlist
	var id int64
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, owner_id, name, created_at FROM playlists WHERE share_code = ?`, code,
	).Scan(&id, &p.OwnerID, &p.Name, &createdAt)
	if err != nil {
		return nil, err
	}
	p.ID = id
	p.ShareCode = code
	p.Tracks, err = s.getPlaylistTracks(ctx, id)
	return &p, err
}

func (s *Store) getPlaylistTracks(ctx context.Context, playlistID int64) ([]PlaylistTrack, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT identifier, title, author, length_ms, uri FROM playlist_tracks
		 WHERE playlist_id = ? ORDER BY position`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlaylistTrack
	for rows.Next() {
		var t PlaylistTrack
		if err := rows.Scan(&t.Identifier, &t.Title, &t.Author, &t.LengthMS, &t.URI); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) ListUserPlaylists(ctx context.Context, ownerID string) ([]PlaylistMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT p.id, p.name, p.share_code, p.created_at, COUNT(pt.id)
		 FROM playlists p LEFT JOIN playlist_tracks pt ON pt.playlist_id = p.id
		 WHERE p.owner_id = ? GROUP BY p.id ORDER BY p.created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlaylistMeta
	for rows.Next() {
		var m PlaylistMeta
		var createdAt string
		if err := rows.Scan(&m.ID, &m.Name, &m.ShareCode, &createdAt, &m.TrackCnt); err != nil {
			return nil, err
		}
		m.OwnerID = ownerID
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) DeletePlaylist(ctx context.Context, playlistID int64, ownerID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM playlists WHERE id = ? AND owner_id = ?`, playlistID, ownerID)
	return err
}

func (s *Store) AddPlaylistTrack(ctx context.Context, playlistID int64, t PlaylistTrack) error {
	var maxPos int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), 0) FROM playlist_tracks WHERE playlist_id = ?`, playlistID,
	).Scan(&maxPos)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("get max position: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO playlist_tracks (playlist_id, identifier, title, author, length_ms, uri, position)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		playlistID, t.Identifier, t.Title, t.Author, t.LengthMS, t.URI, maxPos+1)
	return err
}

// AllSnapshots returns every saved player snapshot (for boot-time restore).
func (s *Store) AllSnapshots(ctx context.Context) ([]*PlayerSnapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT guild_id, text_channel_id, voice_channel_id, current_identifier, current_position_ms, queue, volume, loop_mode
		 FROM player_snapshots`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PlayerSnapshot
	for rows.Next() {
		var snap PlayerSnapshot
		var queueJSON string
		if err := rows.Scan(&snap.GuildID, &snap.TextChannelID, &snap.VoiceChannelID,
			&snap.CurrentIdentifier, &snap.CurrentPositionMS, &queueJSON, &snap.Volume, &snap.LoopMode); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(queueJSON), &snap.Queue); err != nil {
			slog.Warn("snapshot queue unmarshal failed", slog.String("guild", snap.GuildID), slog.Any("err", err))
		}
		out = append(out, &snap)
	}
	return out, rows.Err()
}
