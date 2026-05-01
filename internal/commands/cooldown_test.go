package commands

import (
	"sync"
	"testing"
	"time"
)

func TestCooldown_CheckFirstCall(t *testing.T) {
	c := NewCooldown()

	remaining, ok := c.Check("user1", "emoji-stats")
	if !ok {
		t.Fatal("expected first call to be allowed")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining, got %v", remaining)
	}
}

func TestCooldown_CheckSecondCall(t *testing.T) {
	c := NewCooldown()

	// First call is allowed
	_, ok := c.Check("user1", "emoji-stats")
	if !ok {
		t.Fatal("first call should be allowed")
	}
	c.Set("user1", "emoji-stats")

	// Second call should be on cooldown
	remaining, ok := c.Check("user1", "emoji-stats")
	if ok {
		t.Fatal("second call should be on cooldown")
	}
	if remaining == 0 {
		t.Error("expected positive remaining duration")
	}
	if remaining > cooldownDuration {
		t.Errorf("remaining should not exceed cooldownDuration: got %v", remaining)
	}
}

func TestCooldown_IndependentPerCommand(t *testing.T) {
	c := NewCooldown()

	// User calls /emoji-stats
	c.Set("user1", "emoji-stats")

	// /recap-user should still be allowed
	remaining, ok := c.Check("user1", "recap-user")
	if !ok {
		t.Fatal("/recap-user should be allowed even after /emoji-stats")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining for /recap-user, got %v", remaining)
	}

	// /emoji-stats should still be on cooldown
	remaining, ok = c.Check("user1", "emoji-stats")
	if ok {
		t.Fatal("/emoji-stats should be on cooldown")
	}
	if remaining == 0 {
		t.Error("expected positive remaining for /emoji-stats")
	}
}

func TestCooldown_IndependentPerUser(t *testing.T) {
	c := NewCooldown()

	// User1 calls /emoji-stats
	c.Set("user1", "emoji-stats")

	// User2 should still be allowed
	remaining, ok := c.Check("user2", "emoji-stats")
	if !ok {
		t.Fatal("user2 should be allowed even after user1 used the command")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining for user2, got %v", remaining)
	}

	// User1 should be on cooldown
	remaining, ok = c.Check("user1", "emoji-stats")
	if ok {
		t.Fatal("user1 should be on cooldown")
	}
	if remaining == 0 {
		t.Error("expected positive remaining for user1")
	}
}

func TestCooldown_Expiry(t *testing.T) {
	c := NewCooldown()

	c.Set("user1", "emoji-stats")

	// Manually set the timestamp to be in the past
	c.mu.Lock()
	c.times[cooldownKey{"user1", "emoji-stats"}] = time.Now().Add(-cooldownDuration - time.Second)
	c.mu.Unlock()

	// Should now be allowed
	remaining, ok := c.Check("user1", "emoji-stats")
	if !ok {
		t.Fatal("should be allowed after cooldown expires")
	}
	if remaining != 0 {
		t.Errorf("expected 0 remaining after expiry, got %v", remaining)
	}
}

func TestCooldown_Concurrent(t *testing.T) {
	c := NewCooldown()
	c.Set("user1", "emoji-stats")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Check("user1", "emoji-stats")
		}()
	}
	wg.Wait()
}
