package bot

import (
	"fmt"
	"sync"

	"github.com/disgoorg/snowflake/v2"
)

// VoteManager tracks per-guild, per-action vote sessions.
// When the vote threshold is reached, the stored exec function runs.
type VoteManager struct {
	mu       sync.Mutex
	sessions map[string]*voteSession
}

type voteSession struct {
	voters map[snowflake.ID]bool
	total  int
	exec   func()
}

// NewVoteManager builds an empty manager.
func NewVoteManager() *VoteManager {
	return &VoteManager{sessions: make(map[string]*voteSession)}
}

// sessionKey builds a unique key per guild+action pair.
func sessionKey(guildID snowflake.ID, action string) string {
	return fmt.Sprintf("%d:%s", guildID, action)
}

// VoteOrExecute registers a vote. If the voter already voted, returns
// (voted=true, current count, required) without executing. If the threshold
// is reached (or required <= 0 meaning no voting), executes immediately
// and clears the session.
func (vm *VoteManager) VoteOrExecute(guildID snowflake.ID, userID snowflake.ID, action string, required int, exec func()) (alreadyVoted bool, total int, needed int) {
	key := sessionKey(guildID, action)

	vm.mu.Lock()

	// No voting needed — execute directly.
	if required <= 0 {
		delete(vm.sessions, key)
		vm.mu.Unlock()
		exec()
		return false, 0, 0
	}

	session, ok := vm.sessions[key]
	if !ok {
		session = &voteSession{
			voters: make(map[snowflake.ID]bool),
			total:  required,
			exec:   exec,
		}
		vm.sessions[key] = session
	}

	if session.voters[userID] {
		vm.mu.Unlock()
		return true, len(session.voters), session.total
	}
	session.voters[userID] = true
	count := len(session.voters)
	needed = session.total
	execFn := session.exec

	if count >= session.total {
		delete(vm.sessions, key)
		vm.mu.Unlock()
		execFn()
		return false, count, needed
	}
	vm.mu.Unlock()
	return false, count, needed
}

// Cancel removes a pending vote session for the given guild+action.
func (vm *VoteManager) Cancel(guildID snowflake.ID, action string) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	delete(vm.sessions, sessionKey(guildID, action))
}
