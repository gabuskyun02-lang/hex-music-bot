// Package player holds per-guild playback state: queues, loop modes, and
// play history. It knows nothing about Discord or Lavalink transports —
// handlers drive it, events read from it.
package player

import (
	"math/rand/v2"
	"strings"
	"sync"

	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// LoopMode controls what happens after a track ends.
type LoopMode uint8

const (
	LoopOff LoopMode = iota
	LoopTrack
	LoopQueue
)

func (l LoopMode) String() string {
	switch l {
	case LoopTrack:
		return "track"
	case LoopQueue:
		return "queue"
	default:
		return "off"
	}
}

// ParseLoopMode maps user input onto LoopMode.
func ParseLoopMode(s string) (LoopMode, bool) {
	switch s {
	case "off":
		return LoopOff, true
	case "track":
		return LoopTrack, true
	case "queue":
		return LoopQueue, true
	default:
		return LoopOff, false
	}
}

const historyCap = 50

// State is one guild's playback state. All methods are safe for concurrent
// use from gateway events and interaction handlers.
type State struct {
	mu               sync.Mutex
	maxQueue         int
	queue            []lavalink.Track
	history          []lavalink.Track // finished tracks, most recent last
	loop             LoopMode
	shuffled         bool                    // queue order is no longer enqueue-order
	requesters       map[string]snowflake.ID // track identifier -> requester user ID
	currentRequester snowflake.ID            // who requested the currently playing track
}

// EnqueueAs appends tracks to the queue, recording who requested them.
// Returns the number of tracks actually added and the number rejected
// due to the queue cap (maxQueue). When maxQueue <= 0, no cap applies.
func (s *State) EnqueueAs(requesterID snowflake.ID, tracks ...lavalink.Track) (added, rejected int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.requesters == nil {
		s.requesters = make(map[string]snowflake.ID)
	}
	accept := tracks
	if s.maxQueue > 0 {
		room := s.maxQueue - len(s.queue)
		if room < 0 {
			room = 0
		}
		if len(accept) > room {
			rejected = len(accept) - room
			accept = accept[:room]
		}
	}
	for _, t := range accept {
		s.requesters[t.Info.Identifier] = requesterID
	}
	s.queue = append(s.queue, accept...)
	return len(accept), rejected
}

// Enqueue appends tracks to the back of the queue.
// Returns the number of tracks actually added and the number rejected
// due to the queue cap.
func (s *State) Enqueue(tracks ...lavalink.Track) (added, rejected int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	accept := tracks
	if s.maxQueue > 0 {
		room := s.maxQueue - len(s.queue)
		if room < 0 {
			room = 0
		}
		if len(accept) > room {
			rejected = len(accept) - room
			accept = accept[:room]
		}
	}
	s.queue = append(s.queue, accept...)
	return len(accept), rejected
}

// Next pops and returns the next queued track (no requester info) and
// promotes that track's requester to the current one.
func (s *State) Next() (lavalink.Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return lavalink.Track{}, false
	}
	track := s.queue[0]
	s.queue = s.queue[1:]
	s.currentRequester = s.requesters[track.Info.Identifier]
	delete(s.requesters, track.Info.Identifier)
	return track, true
}

// Drop removes up to n tracks from the front of the queue (used by /skip n).
func (s *State) Drop(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.queue) {
		n = len(s.queue)
	}
	for _, t := range s.queue[:n] {
		delete(s.requesters, t.Info.Identifier)
	}
	s.queue = s.queue[n:]
}

// Shuffle randomizes the queue in place and marks the state shuffled.
func (s *State) Shuffle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	rand.Shuffle(len(s.queue), func(i, j int) {
		s.queue[i], s.queue[j] = s.queue[j], s.queue[i]
	})
	s.shuffled = true
}

// Shuffled reports whether the queue order is currently shuffle-descended.
func (s *State) Shuffled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuffled
}

// ClearQueue empties the queue without touching history or loop mode.
func (s *State) ClearQueue() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = nil
	s.requesters = nil
	s.shuffled = false
}

// Len returns the number of queued tracks.
func (s *State) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// Snapshot copies the queue for display purposes.
func (s *State) Snapshot() []lavalink.Track {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]lavalink.Track, len(s.queue))
	copy(out, s.queue)
	return out
}

// LoopMode returns the current loop mode.
func (s *State) LoopMode() LoopMode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loop
}

// SetShuffled overrides the shuffled flag (restore path only — no unshuffle
// command exists, so nothing else writes it directly).
func (s *State) SetShuffled(shuffled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shuffled = shuffled
}

// SetLoopMode updates the loop mode.
func (s *State) SetLoopMode(mode LoopMode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loop = mode
}

// PushHistory records a finished track, trimming to historyCap.
func (s *State) PushHistory(track lavalink.Track) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, track)
	if len(s.history) > historyCap {
		s.history = s.history[len(s.history)-historyCap:]
	}
}

// PopHistory returns the most recently finished track.
func (s *State) PopHistory() (lavalink.Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return lavalink.Track{}, false
	}
	track := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	return track, true
}

// Manager owns all guild states.
type Manager struct {
	mu       sync.RWMutex
	queueMax int
	states   map[snowflake.ID]*State
}

// PeekHistory returns the most recently finished track without popping it.
func (s *State) PeekHistory() (lavalink.Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return lavalink.Track{}, false
	}
	return s.history[len(s.history)-1], true
}

// NewManager returns an empty Manager. queueMax caps each guild's queue;
// 0 means unlimited.
func NewManager(queueMax int) *Manager {
	return &Manager{queueMax: queueMax, states: make(map[snowflake.ID]*State)}
}

// Get returns the guild's state, creating it if missing.
func (m *Manager) Get(guildID snowflake.ID) *State {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.states[guildID]
	if !ok {
		state = &State{maxQueue: m.queueMax}
		m.states[guildID] = state
	}
	return state
}

// Delete drops the guild's state entirely (called when the bot leaves VC).
func (m *Manager) Delete(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.states, guildID)
}

// RemoveAt deletes the queue item at the given 0-based index and returns it.
func (s *State) RemoveAt(index int) (lavalink.Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.queue) {
		return lavalink.Track{}, false
	}
	track := s.queue[index]
	delete(s.requesters, track.Info.Identifier)
	s.queue = append(s.queue[:index], s.queue[index+1:]...)
	return track, true
}

// MoveTrack moves a track from one 0-based index to another.
func (s *State) MoveTrack(from, to int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if from < 0 || from >= len(s.queue) || to < 0 || to >= len(s.queue) {
		return false
	}
	track := s.queue[from]
	s.queue = append(s.queue[:from], s.queue[from+1:]...)
	s.queue = append(s.queue[:to], append([]lavalink.Track{track}, s.queue[to:]...)...)
	return true
}

// SwapTracks exchanges two tracks at the given 0-based indices.
func (s *State) SwapTracks(a, b int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a < 0 || a >= len(s.queue) || b < 0 || b >= len(s.queue) {
		return false
	}
	s.queue[a], s.queue[b] = s.queue[b], s.queue[a]
	return true
}

// RemoveDuplicates removes tracks with duplicate titles, keeping first occurrence.
// Returns count removed.
func (s *State) RemoveDuplicates() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]bool)
	kept := make([]lavalink.Track, 0, len(s.queue))
	removed := 0
	for _, t := range s.queue {
		key := strings.ToLower(t.Info.Title)
		if seen[key] {
			delete(s.requesters, t.Info.Identifier)
			removed++
			continue
		}
		seen[key] = true
		kept = append(kept, t)
	}
	if removed > 0 {
		s.queue = kept
	}
	return removed
}

// PeekNext returns the next queued track without consuming it.
func (s *State) PeekNext() (lavalink.Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return lavalink.Track{}, false
	}
	return s.queue[0], true
}

// InsertAtAs inserts a track at the given 0-based index, recording its
// requester. Negative index clamps to the front. Returns false if the
// queue is at cap.
func (s *State) InsertAtAs(index int, track lavalink.Track, requesterID snowflake.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxQueue > 0 && len(s.queue) >= s.maxQueue {
		return false
	}
	if index < 0 {
		index = 0
	}
	if s.requesters == nil {
		s.requesters = make(map[string]snowflake.ID)
	}
	s.requesters[track.Info.Identifier] = requesterID
	if index >= len(s.queue) {
		s.queue = append(s.queue, track)
		return true
	}
	s.queue = append(s.queue[:index], append([]lavalink.Track{track}, s.queue[index:]...)...)
	return true
}

// HasDuplicate checks if a track with the same title is already in the queue.
func (s *State) HasDuplicate(title string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(title)
	for _, t := range s.queue {
		if strings.ToLower(t.Info.Title) == key {
			return true
		}
	}
	return false
}

// RequesterFor returns the user ID that requested a track by identifier.
// Returns 0 if not found (e.g., track was enqueued without requester tracking).
func (s *State) RequesterFor(identifier string) snowflake.ID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requesters[identifier]
}

// SetCurrentRequester records who requested the currently playing track.
func (s *State) SetCurrentRequester(userID snowflake.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentRequester = userID
}

// GetCurrentRequester returns who requested the currently playing track.
func (s *State) GetCurrentRequester() snowflake.ID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentRequester
}
