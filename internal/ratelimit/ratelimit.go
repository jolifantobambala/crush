// Package ratelimit provides per-key request rate limiting for HTTP
// clients.
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Transport is an [http.RoundTripper] that waits for a token from Limiter
// before forwarding each request. It respects the request's context, so a
// canceled or expired request stops waiting.
type Transport struct {
	Base    http.RoundTripper
	Limiter *rate.Limiter
}

// RoundTrip implements [http.RoundTripper].
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.Limiter.Wait(req.Context()); err != nil {
		// Surface context failures as context errors so callers can tell
		// a canceled request from a transport failure. The limiter
		// reports an exhausted deadline as its own error.
		if ctxErr := req.Context().Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if _, ok := req.Context().Deadline(); ok {
			return nil, fmt.Errorf("%w: %w", context.DeadlineExceeded, err)
		}
		return nil, err
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// Registry holds one limiter per key so that every client built for the
// same key shares the configured rate. The zero value is ready to use.
type Registry struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// NewRegistry returns an empty registry.
func NewRegistry() Registry {
	return Registry{limiters: make(map[string]*rate.Limiter)}
}

// Limiter returns the shared limiter for key, paced at requestsPerMinute.
// A non-positive rate means unlimited and returns nil. When a key is
// requested again with a different rate, its existing limiter is updated in
// place so requests already waiting keep their place in line.
func (r *Registry) Limiter(key string, requestsPerMinute int) *rate.Limiter {
	if requestsPerMinute <= 0 {
		return nil
	}

	limit := rate.Every(time.Minute / time.Duration(requestsPerMinute))

	r.mu.Lock()
	defer r.mu.Unlock()

	if limiter, ok := r.limiters[key]; ok {
		limiter.SetLimit(limit)
		limiter.SetBurst(1)
		return limiter
	}

	if r.limiters == nil {
		r.limiters = make(map[string]*rate.Limiter)
	}
	limiter := rate.NewLimiter(limit, 1)
	r.limiters[key] = limiter
	return limiter
}
