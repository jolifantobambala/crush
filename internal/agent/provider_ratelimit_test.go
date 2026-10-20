package agent

import (
	"net/http"
	"testing"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/ratelimit"
	"github.com/stretchr/testify/require"
)

func TestProviderHTTPClientRateLimit(t *testing.T) {
	t.Parallel()

	cfg, err := config.Init(t.Context(), t.TempDir(), "", false)
	require.NoError(t, err)
	c := &coordinator{cfg: cfg}

	// Without a rate limit and without debug, the SDK keeps its default
	// client.
	require.Nil(t, c.providerHTTPClient(nil, "provider", 0))

	client := c.providerHTTPClient(nil, "provider", 60)
	require.NotNil(t, client)
	transport, ok := client.Transport.(*ratelimit.Transport)
	require.True(t, ok, "client should carry the rate-limiting transport")
	require.Same(t, transport.Limiter, c.rateLimits.Limiter("provider", 60))

	// A base transport (e.g. OpenAI's OAuth transport) is kept even when
	// no limit applies.
	base := &http.Transport{}
	client = c.providerHTTPClient(base, "other", 0)
	require.NotNil(t, client)
	require.Same(t, base, client.Transport)
}
