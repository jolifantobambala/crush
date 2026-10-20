package ratelimit

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestRegistryLimiterUnlimited(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	require.Nil(t, r.Limiter("p", 0))
	require.Nil(t, r.Limiter("p", -1))
}

func TestRegistryLimiterSharedAndUpdated(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	first := r.Limiter("p", 60)
	require.NotNil(t, first)
	require.Equal(t, rate.Limit(1), first.Limit())

	same := r.Limiter("p", 60)
	require.Same(t, first, same)

	// Reconfiguring the provider updates the limiter in place so clients
	// built earlier keep sharing the same budget.
	updated := r.Limiter("p", 120)
	require.Same(t, first, updated)
	require.Equal(t, rate.Limit(2), updated.Limit())

	other := r.Limiter("q", 60)
	require.NotSame(t, first, other)
}

type countingTransport struct {
	calls int
}

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func TestTransportPacesRequests(t *testing.T) {
	t.Parallel()

	base := &countingTransport{}
	transport := &Transport{
		Base:    base,
		Limiter: rate.NewLimiter(rate.Every(50*time.Millisecond), 1),
	}

	start := time.Now()
	for range 2 {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
		require.NoError(t, err)
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	elapsed := time.Since(start)

	require.Equal(t, 2, base.calls)
	require.GreaterOrEqual(t, elapsed, 40*time.Millisecond,
		"second request should wait for the limiter")
}

func TestTransportHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	transport := &Transport{
		Base:    &countingTransport{},
		Limiter: rate.NewLimiter(rate.Every(time.Minute), 1),
	}

	// Consume the only token so the next request has to wait.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", nil)
	require.NoError(t, err)
	_, err = transport.RoundTrip(req)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	require.NoError(t, err)
	_, err = transport.RoundTrip(req)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
