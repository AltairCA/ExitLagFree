package api

import (
	"sync"
	"time"
)

// limiter tracks failed authentication attempts per client IP and locks an
// IP out once it exceeds maxFails within window.
type limiter struct {
	maxFails int
	window   time.Duration
	lockout  time.Duration
	now      func() time.Time

	mu sync.Mutex
	m  map[string]*attempts
}

type attempts struct {
	fails       int
	windowStart time.Time
	lockedUntil time.Time
}

func newLimiter(maxFails int, window, lockout time.Duration) *limiter {
	return &limiter{maxFails: maxFails, window: window, lockout: lockout, now: time.Now, m: map[string]*attempts{}}
}

func (l *limiter) Allowed(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.m[ip]
	return a == nil || !l.now().Before(a.lockedUntil)
}

func (l *limiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	a := l.m[ip]
	if a == nil || now.Sub(a.windowStart) > l.window {
		if len(l.m) > 10000 {
			l.gcLocked(now)
		}
		a = &attempts{windowStart: now}
		l.m[ip] = a
	}
	a.fails++
	if a.fails >= l.maxFails {
		a.lockedUntil = now.Add(l.lockout)
		a.fails = 0
		a.windowStart = now
	}
}

func (l *limiter) gcLocked(now time.Time) {
	for ip, a := range l.m {
		if now.Sub(a.windowStart) > l.window && !now.Before(a.lockedUntil) {
			delete(l.m, ip)
		}
	}
}
