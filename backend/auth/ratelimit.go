package auth

import (
	"sort"
	"sync"
	"time"
)

// defaultMaxEntries bounds the attempts map.
//
// The map is keyed by whatever the caller passes, and for login that is the
// email in the request body -- a value the caller chooses. An unbounded map here
// is therefore unbounded attacker-controlled memory on a public endpoint, and it
// was: entries were only ever removed by RecordSuccess, so a flood of distinct
// addresses grew it forever. At roughly 130 bytes an entry, this cap is about
// 1.3 MB.
const defaultMaxEntries = 10000

// evictDivisor is how much of the capacity is freed in one pass when the map is
// full. Trimming a tenth at a time keeps insertion amortised O(1); dropping a
// single entry per insert would make every request during a flood an O(n) scan,
// which is the one moment the limiter must stay cheap.
const evictDivisor = 10

type attempt struct {
	blocked  time.Time
	seen     time.Time
	failures int
}

// RateLimiter blocks a key after maxFailures consecutive failures until the
// cooldown passes. A success resets the counter.
//
// The map is bounded by maxEntries. Expiry alone is not enough to bound it: an
// attacker using a distinct address per request never accumulates enough
// failures to earn a block, so those entries never become removable by age. The
// cap is what makes the memory a constant.
//
// Eviction is not a weakness in the block. It takes the least recently seen
// entries, and a key that is being checked has its last-seen refreshed, so an
// attacker who floods to push their own counter out must spend roughly
// maxEntries requests to do it -- far more than the five failures the counter
// would have blocked. The flood becomes the rate limit.
type RateLimiter struct {
	attempts    map[string]*attempt
	maxFailures int
	cooldown    time.Duration
	maxEntries  int
	mu          sync.Mutex
}

func NewRateLimiter(maxFailures int, cooldown time.Duration) *RateLimiter {
	return &RateLimiter{
		maxFailures: maxFailures,
		cooldown:    cooldown,
		maxEntries:  defaultMaxEntries,
		attempts:    make(map[string]*attempt),
	}
}

// Blocked reports whether the key is in a cooldown.
//
// It also refreshes the entry's last-seen time. That is a write on what reads
// like a read, and it is deliberate: a key that is being actively checked is the
// key the limiter is defending, and refreshing it keeps that entry away from the
// eviction pass. It never creates an entry -- an unknown key is not blocked and
// costs nothing to check.
func (l *RateLimiter) Blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		return false
	}
	a.seen = now
	return a.failures >= l.maxFailures && now.Before(a.blocked)
}

func (l *RateLimiter) RecordFailure(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.attempts[key]
	if !ok {
		if len(l.attempts) >= l.maxEntries {
			l.evictLocked(now)
		}
		a = &attempt{}
		l.attempts[key] = a
	}
	a.seen = now
	a.failures++
	if a.failures >= l.maxFailures {
		a.blocked = now.Add(l.cooldown)
	}
}

func (l *RateLimiter) RecordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

// evictLocked frees room in one pass. The caller holds the lock.
//
// Two passes, in order of what can be given up safely:
//
//  1. A block that has already been served is finished with -- the cooldown it
//     earned has passed, so the entry protects nothing. These are the only
//     entries provably done, so they go first.
//  2. The least recently seen, until the map is back under the target. A key
//     that is being actively checked has its last-seen refreshed by Blocked, so
//     it sorts last and survives: a flood spends its budget on entries nobody is
//     looking at, and the key under attack stays blocked.
func (l *RateLimiter) evictLocked(now time.Time) {
	target := l.maxEntries - l.maxEntries/evictDivisor
	if target < 1 {
		target = 1
	}

	for key, a := range l.attempts {
		if len(l.attempts) <= target {
			return
		}
		if a.failures >= l.maxFailures && !now.Before(a.blocked) {
			delete(l.attempts, key)
		}
	}
	if len(l.attempts) <= target {
		return
	}

	// Sorted once rather than scanned per deletion, which would be quadratic in
	// the number freed -- and this runs during a flood, which is the one moment
	// the limiter has to stay cheap.
	aged := make([]struct {
		key  string
		seen time.Time
	}, 0, len(l.attempts))
	for key, a := range l.attempts {
		aged = append(aged, struct {
			key  string
			seen time.Time
		}{key, a.seen})
	}
	sort.Slice(aged, func(i, j int) bool { return aged[i].seen.Before(aged[j].seen) })
	for _, entry := range aged {
		if len(l.attempts) <= target {
			return
		}
		delete(l.attempts, entry.key)
	}
}
