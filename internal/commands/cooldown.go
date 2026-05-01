package commands

import (
	"sync"
	"time"
)

// cooldownDuration is the default cooldown period for each slash command.
const cooldownDuration = 60 * time.Second

// cooldownKey is the composite key format for the cooldown map.
type cooldownKey struct {
	userID    string
	command   string
}

// Cooldown tracks per-user, per-command cooldowns.
// It is safe for concurrent use.
type Cooldown struct {
	mu    sync.RWMutex
	times map[cooldownKey]time.Time
}

// NewCooldown creates a new Cooldown instance.
func NewCooldown() *Cooldown {
	return &Cooldown{
		times: make(map[cooldownKey]time.Time),
	}
}

// Check returns whether the given user can use the given command.
// If on cooldown, it returns the remaining duration and false.
// If not on cooldown, it returns 0 and true.
func (c *Cooldown) Check(userID, command string) (time.Duration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cooldownKey{userID, command}
	t, ok := c.times[key]
	if !ok {
		return 0, true
	}

	remaining := cooldownDuration - time.Since(t)
	if remaining > 0 {
		return remaining, false
	}
	return 0, true
}

// Set records the current time for the given user and command.
func (c *Cooldown) Set(userID, command string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.times[cooldownKey{userID, command}] = time.Now()
}
