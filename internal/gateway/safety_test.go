package gateway

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_MaxMessageSize(t *testing.T) {
	handler := &MockConnHandler{}
	srv := NewServer(ServerConfig{Port: 0, Auth: AuthConfig{Mode: "none"}}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = srv.ListenAndServe(ctx)
	}()

	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	ws, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/ws", nil)
	require.NoError(t, err)
	defer ws.Close()

	// Read challenge first
	_, _, err = ws.ReadMessage()
	require.NoError(t, err)

	// Send a message larger than 512KB (e.g., 600KB)
	largeData := make([]byte, 600*1024)
	rand.Read(largeData)

	err = ws.WriteMessage(websocket.BinaryMessage, largeData)
	require.NoError(t, err)

	// Attempt to read next message (should fail if server closes connection due to size limit)
	ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, err = ws.ReadMessage()

	// Expect proper CloseError
	assert.Error(t, err, "connection should be closed")
	v, ok := err.(*websocket.CloseError)
	assert.True(t, ok, "error should be a CloseError")
	if ok {
		assert.Equal(t, websocket.CloseMessageTooBig, v.Code, "should be CloseMessageTooBig (1009)")
	}
}

func TestServer_ReadDeadline(t *testing.T) {
	handler := &MockConnHandler{}
	// Set very short deadlines for testing: Wait 200ms for Pong, Ping every 100ms
	cfg := ServerConfig{
		Port:       0,
		Auth:       AuthConfig{Mode: "none"},
		PongWait:   200 * time.Millisecond,
		PingPeriod: 100 * time.Millisecond,
	}
	srv := NewServer(cfg, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = srv.ListenAndServe(ctx)
	}()

	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	ws, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/ws", nil)
	require.NoError(t, err)
	defer ws.Close()

	// Read challenge
	_, _, err = ws.ReadMessage()
	require.NoError(t, err)

	// Now wait. The server expects Pongs. We send NOTHING.
	// Server has PongWait=200ms. It should disconnect us around 200ms (plus some processing time).
	
	// We'll try to read the next message. 
	// If heartbeat is implemented, the server will send Pings.
	// If we ignore them, eventually the server will close connection.
	// OR: The server will close connection if it doesn't receive a Pong in response to Ping.
	
	// Since we are NOT reading, we won't see Pings (unless we read in loop).
	// But the server tracks time since last Pong.
	
	// Crucial: By default, ws.ReadMessage will automatically reply to Pings with Pongs.
	// We must disable this to simulate a zombie client that doesn't respond.
	ws.SetPingHandler(func(appData string) error {
		return nil // Do nothing (don't send Pong)
	})

	// We expect a read error (Close) eventually.
	// Safety net: wait longer than the server's timeout
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	
	// Server should close around 200ms.
	_, _, err = ws.ReadMessage()
	
	// Expect strict close error
	assert.Error(t, err, "connection should be closed")
	if err != nil {
		// Check if it's a timeout (bad) or close (good)
		if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
			assert.Fail(t, "connection timed out instead of closing (server didn't enforce heartbeat)")
		} else {
			// Ensure it's a close error
			assert.True(t, websocket.IsCloseError(err, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) || websocket.IsUnexpectedCloseError(err), "expected close error, got %v", err)
		}
	}
}
