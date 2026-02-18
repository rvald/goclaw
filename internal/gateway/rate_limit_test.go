package gateway

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_RateLimiting(t *testing.T) {
	handler := &MockConnHandler{}
	// Limit: 2 req/sec, burst 2. This ensures sending 10 requests rapidly will trigger limit.
	srv := NewServer(ServerConfig{Port: 0, RateLimit: 2.0, RateBurst: 2}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = srv.ListenAndServe(ctx)
	}()

	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	url := "ws://" + srv.Addr() + "/ws"
	
	// Try to connect 10 times rapidly
	successCount := 0
	failureCount := 0
	
	for i := 0; i < 10; i++ {
		ws, resp, err := websocket.DefaultDialer.Dial(url, nil)
		if err == nil {
			successCount++
			ws.Close()
		} else {
			if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
				failureCount++
			}
		}
	}

	// We expect rate limit around 5 requests/sec burst.
	// So we expect at least SOME failures if we burst 10.
	// Currently (Red Phase), we expect ALL 10 to succeed because there is no limiter.
	
	assert.Greater(t, failureCount, 0, "expected some connections to be rate limited")
	assert.Less(t, successCount, 10, "expected successes to be rate limited")
}
