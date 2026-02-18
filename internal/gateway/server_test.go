package gateway

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	. "github.com/rvald/goclaw/internal/protocol"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_AcceptConnection(t *testing.T) {
	handler := &MockConnHandler{}
	srv := NewServer(ServerConfig{Port: 0, Auth: AuthConfig{Mode: "none"}}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.ListenAndServe(ctx)
	// Wait for server to be ready
	require.Eventually(t, func() bool {
		return srv.Addr() != ""
	}, 2*time.Second, 10*time.Millisecond)
	// Connect with a real WS client
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/ws", nil)
	require.NoError(t, err)
	defer ws.Close()
	// Should receive the connect.challenge event
	_, msg, err := ws.ReadMessage()
	require.NoError(t, err)
	frame, err := ParseFrame(msg)
	require.NoError(t, err)
	evt := frame.(*EventFrame)
	assert.Equal(t, "connect.challenge", evt.Event)
}

func TestServer_MultipleConnections(t *testing.T) {
	handler := &MockConnHandler{}
	srv := NewServer(ServerConfig{Port: 0, Auth: AuthConfig{Mode: "none"}}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.ListenAndServe(ctx)
	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)
	// Connect two clients
	ws1, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/ws", nil)
	require.NoError(t, err)
	defer ws1.Close()
	ws2, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/ws", nil)
	require.NoError(t, err)
	defer ws2.Close()
	// Both should receive challenge events
	_, msg1, err := ws1.ReadMessage()
	require.NoError(t, err)
	_, msg2, err := ws2.ReadMessage()
	require.NoError(t, err)
	frame1, _ := ParseFrame(msg1)
	frame2, _ := ParseFrame(msg2)
	assert.Equal(t, "connect.challenge", frame1.(*EventFrame).Event)
	assert.Equal(t, "connect.challenge", frame2.(*EventFrame).Event)
}

func TestServer_ShutdownDrains(t *testing.T) {
	handler := &MockConnHandler{}
	srv := NewServer(ServerConfig{Port: 0, Auth: AuthConfig{Mode: "none"}}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	go srv.ListenAndServe(ctx)
	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)
	// Connect a client
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+srv.Addr()+"/ws", nil)
	require.NoError(t, err)
	// Read challenge to confirm connection is alive
	_, _, err = ws.ReadMessage()
	require.NoError(t, err)
	// Trigger shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	cancel() // stop accepting new connections
	// Shutdown should complete (not hang)
	err = srv.Shutdown(shutdownCtx)
	assert.NoError(t, err)
	// Client should see the connection close
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = ws.ReadMessage()
	assert.Error(t, err) // connection should be closed
}

func TestServer_HealthEndpoint(t *testing.T) {
	handler := &MockConnHandler{}
	srv := NewServer(ServerConfig{Port: 0, Auth: AuthConfig{Mode: "none"}}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.ListenAndServe(ctx)
	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)
	resp, err := http.Get("http://" + srv.Addr() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "ok")
}