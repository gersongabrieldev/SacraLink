// Package ratelimit limita requisições por IP e bloqueia temporariamente IPs
// com falhas repetidas de autenticação.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter combina limite de taxa por IP com bloqueio por falhas.
type Limiter struct {
	mu        sync.Mutex
	rate      rate.Limit
	burst     int
	maxFails  int
	window    time.Duration
	blockFor  time.Duration
	clients   map[string]*client
	now       func() time.Time
	lastSweep time.Time
}

type client struct {
	lim          *rate.Limiter
	fails        int
	firstFail    time.Time
	blockedUntil time.Time
	lastSeen     time.Time
}

// New cria um limitador: perSecond/burst por IP; maxFails falhas em window
// bloqueiam o IP por blockFor.
func New(perSecond float64, burst, maxFails int, window, blockFor time.Duration) *Limiter {
	return &Limiter{
		rate: rate.Limit(perSecond), burst: burst, maxFails: maxFails, window: window, blockFor: blockFor,
		clients: map[string]*client{}, now: time.Now,
	}
}

func (l *Limiter) get(ip string) *client {
	now := l.now()
	if now.Sub(l.lastSweep) > time.Minute {
		for k, c := range l.clients {
			if now.Sub(c.lastSeen) > 10*time.Minute && now.After(c.blockedUntil) {
				delete(l.clients, k)
			}
		}
		l.lastSweep = now
	}
	c := l.clients[ip]
	if c == nil {
		c = &client{lim: rate.NewLimiter(l.rate, l.burst)}
		l.clients[ip] = c
	}
	c.lastSeen = now
	return c
}

// Allow informa se a requisição do IP pode prosseguir.
func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.get(ip)
	if l.now().Before(c.blockedUntil) {
		return false
	}
	return c.lim.AllowN(l.now(), 1)
}

// Fail registra uma falha de autenticação do IP.
func (l *Limiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.get(ip)
	now := l.now()
	if c.fails == 0 || now.Sub(c.firstFail) > l.window {
		c.fails, c.firstFail = 0, now
	}
	c.fails++
	if c.fails >= l.maxFails {
		c.blockedUntil = now.Add(l.blockFor)
		c.fails = 0
	}
}

// Success limpa as falhas do IP.
func (l *Limiter) Success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.get(ip).fails = 0
}

// Blocked informa se o IP está bloqueado.
func (l *Limiter) Blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.now().Before(l.get(ip).blockedUntil)
}
