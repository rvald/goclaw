package gateway

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_MetricsEndpoint(t *testing.T) {
	handler := &MockConnHandler{}
	srv := NewServer(ServerConfig{Port: 0, Auth: AuthConfig{Mode: "none"}}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = srv.ListenAndServe(ctx)
	}()

	// Wait for server to be ready
	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	// Request /metrics
	resp, err := http.Get("http://" + srv.Addr() + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Should be 200 OK (Will fail initially as it returns 404)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "metrics endpoint should return 200 OK")
}
