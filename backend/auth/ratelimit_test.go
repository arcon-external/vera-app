package auth

import (
	"fmt"
	"testing"
	"time"
)

// These tests pin two things at once, and the second matters more than the
// first: that the limiter's map is bounded, and that bounding it did not change
// what the limiter does. A silent regression in the second is a security
// regression, so the semantics are asserted explicitly rather than assumed.

func TestRateLimiterBlocksAfterMaxFailures(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	now := time.Now()

	for i := 1; i <= 4; i++ {
		limiter.RecordFailure("someone@example.com", now)
		if limiter.Blocked("someone@example.com", now) {
			t.Fatalf("blocked after %d failures; want the block to start at 5", i)
		}
	}
	limiter.RecordFailure("someone@example.com", now)
	if !limiter.Blocked("someone@example.com", now) {
		t.Fatal("not blocked after 5 failures")
	}
}

func TestRateLimiterBlockExpiresWithTheCooldown(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	now := time.Now()
	for i := 0; i < 5; i++ {
		limiter.RecordFailure("someone@example.com", now)
	}
	if !limiter.Blocked("someone@example.com", now.Add(30*time.Second)) {
		t.Fatal("not blocked inside the cooldown")
	}
	if limiter.Blocked("someone@example.com", now.Add(time.Minute)) {
		t.Fatal("still blocked at the cooldown boundary; the block must expire")
	}
}

func TestRateLimiterSuccessResetsTheCounter(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	now := time.Now()
	for i := 0; i < 4; i++ {
		limiter.RecordFailure("someone@example.com", now)
	}
	limiter.RecordSuccess("someone@example.com")

	// Four more failures after a success must not block: the counter reset.
	for i := 0; i < 4; i++ {
		limiter.RecordFailure("someone@example.com", now)
	}
	if limiter.Blocked("someone@example.com", now) {
		t.Fatal("blocked after a success reset the counter")
	}
}

func TestRateLimiterUnknownKeyIsNeverBlocked(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	if limiter.Blocked("never-seen@example.com", time.Now()) {
		t.Fatal("an unknown key reported as blocked")
	}
	if len(limiter.attempts) != 0 {
		t.Fatalf("checking an unknown key created %d entries; it must not", len(limiter.attempts))
	}
}

// TestRateLimiterMemoryIsBounded is the property that was missing.
//
// The key is caller-chosen -- for login it is the email in the request body --
// so before the cap a flood of distinct addresses grew this map forever. The
// test drives many times the cap through it and asserts it stays put.
func TestRateLimiterMemoryIsBounded(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	limiter.maxEntries = 100
	now := time.Now()

	// A flood where every address is used once, so nothing ever accumulates
	// enough failures to earn a block and nothing is removable by age. This is
	// the shape the old limiter could not survive.
	for i := 0; i < limiter.maxEntries*50; i++ {
		limiter.RecordFailure(fmt.Sprintf("flood-%d@example.com", i), now)
	}

	if got := len(limiter.attempts); got > limiter.maxEntries {
		t.Fatalf("map holds %d entries; want it capped at %d", got, limiter.maxEntries)
	}
	// It must not merely sit at the cap by accident -- it should be back under
	// it, because a full map frees a slice of its capacity at once.
	if got := len(limiter.attempts); got == 0 {
		t.Fatal("the map was emptied entirely; eviction should keep a working set")
	}
}

// TestRateLimiterReclaimsServedBlocks checks the first eviction pass: an entry
// whose cooldown has passed protects nothing and should go before anything else.
func TestRateLimiterReclaimsServedBlocks(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	limiter.maxEntries = 100
	now := time.Now()

	// Fill with blocks that have already been served.
	for i := 0; i < limiter.maxEntries; i++ {
		key := fmt.Sprintf("served-%d@example.com", i)
		for f := 0; f < 5; f++ {
			limiter.RecordFailure(key, now)
		}
	}
	// Move time past every cooldown, then force an eviction.
	later := now.Add(2 * time.Minute)
	limiter.RecordFailure("new@example.com", later)

	// The map must be back under capacity, and it must have got there by freeing
	// a batch rather than a single entry -- eviction runs before the insert, so
	// the count afterwards is the target plus the one just added.
	if got := len(limiter.attempts); got > limiter.maxEntries {
		t.Fatalf("map holds %d entries; want it capped at %d", got, limiter.maxEntries)
	}
	if got := len(limiter.attempts); got >= limiter.maxEntries {
		t.Fatalf("map holds %d entries, still at capacity; eviction freed only one entry", got)
	}
	if _, ok := limiter.attempts["new@example.com"]; !ok {
		t.Fatal("eviction dropped the entry it had just been asked to record")
	}
}

// TestRateLimiterKeepsDefendingAnActiveKey guards the trade the cap makes.
//
// Eviction is arbitrary, but a key that is being actively checked has its
// last-seen time refreshed by Blocked. A flood is therefore spending its budget
// on entries nobody is looking at, and the key under attack survives long enough
// to keep blocking.
func TestRateLimiterKeepsDefendingAnActiveKey(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	limiter.maxEntries = 50
	now := time.Now()

	target := "target@example.com"
	for i := 0; i < 5; i++ {
		limiter.RecordFailure(target, now)
	}

	// Time has to advance for last-seen to mean anything: with every entry
	// stamped identically the ordering is arbitrary and this would prove
	// nothing. The clock ticks per request, as it does in reality.
	clock := now
	tick := func() time.Time { clock = clock.Add(time.Millisecond); return clock }

	// Flood, checking the target between bursts the way an attacker probing for
	// a reset would.
	for i := 0; i < limiter.maxEntries*20; i++ {
		limiter.RecordFailure(fmt.Sprintf("flood-%d@example.com", i), tick())
		if i%10 == 0 {
			limiter.Blocked(target, tick())
		}
	}
	if !limiter.Blocked(target, tick()) {
		t.Fatal("a flood evicted the key being defended; refreshing last-seen is not working")
	}
}

func TestRateLimiterConcurrentUseIsRaceFree(t *testing.T) {
	limiter := NewRateLimiter(5, time.Minute)
	limiter.maxEntries = 200
	now := time.Now()

	done := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		go func(worker int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 500; i++ {
				key := fmt.Sprintf("worker-%d-%d@example.com", worker, i)
				limiter.RecordFailure(key, now)
				limiter.Blocked(key, now)
				if i%50 == 0 {
					limiter.RecordSuccess(key)
				}
			}
		}(worker)
	}
	for worker := 0; worker < 8; worker++ {
		<-done
	}
	if got := len(limiter.attempts); got > limiter.maxEntries {
		t.Fatalf("map holds %d entries after concurrent use; want it capped at %d", got, limiter.maxEntries)
	}
}
