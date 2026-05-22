package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	loginMaxAttempts  = 10
	loginWindow       = 5 * time.Minute
	loginBlockFor     = 15 * time.Minute
	loginCleanupEvery = time.Hour

	authScopePassword = "password"
	authScopeToken    = "token"
)

type loginBucket struct {
	failures  int
	windowEnd time.Time
	blockedAt time.Time
}

type loginLimiter struct {
	mu        sync.Mutex
	entries   map[string]*loginBucket
	lastClean time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		entries:   make(map[string]*loginBucket),
		lastClean: time.Now(),
	}
}

// allow returns false if the IP/scope is currently blocked.
func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.maybeClean(now)

	b := l.entries[ip]
	if b == nil {
		return true
	}
	if !b.blockedAt.IsZero() && now.Before(b.blockedAt.Add(loginBlockFor)) {
		return false
	}
	return true
}

// record records a failed authentication attempt and returns whether the key is now blocked.
func (l *loginLimiter) record(ip string) (blocked bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b := l.entries[ip]
	if b == nil {
		b = &loginBucket{}
		l.entries[ip] = b
	}

	if now.After(b.windowEnd) {
		b.failures = 0
		b.windowEnd = now.Add(loginWindow)
	}
	b.failures++

	if b.failures >= loginMaxAttempts {
		b.blockedAt = now
		return true
	}
	return false
}

// reset clears state for an IP on successful login.
func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

func authLimitKey(scope, ip string) string {
	return scope + "\x00" + ip
}

func (l *loginLimiter) maybeClean(now time.Time) {
	if now.Before(l.lastClean.Add(loginCleanupEvery)) {
		return
	}
	for ip, b := range l.entries {
		expired := b.blockedAt.IsZero() && now.After(b.windowEnd)
		unblocked := !b.blockedAt.IsZero() && now.After(b.blockedAt.Add(loginBlockFor))
		if expired || unblocked {
			delete(l.entries, ip)
		}
	}
	l.lastClean = now
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
