package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/rvald/goclaw/internal/protocol"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrStr(s string) *string { return &s }

func TestIntegration_ConnectAndInvoke(t *testing.T) {
	gw, err := New(GatewayConfig{
		Port:      0,
		AuthToken: "test-token",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go gw.Run(ctx)

	require.Eventually(t, func() bool { return gw.server.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	// --- Connect as iOS node ---
	ws, _, err := websocket.DefaultDialer.Dial("ws://"+gw.server.Addr()+"/ws", nil)
	require.NoError(t, err)
	defer ws.Close()

	// 1. Read challenge
	_, msg, err := ws.ReadMessage()
	require.NoError(t, err)
	frame, _ := ParseFrame(msg)
	assert.Equal(t, "connect.challenge", frame.(*EventFrame).Event)

	// 2. Send connect
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{
			ID: "iphone-test", DisplayName: "Test iPhone",
			Version: "1.0", Platform: "ios", Mode: "node",
		},
		Commands: []string{"location.get"},
		Auth:     &ConnectAuth{Token: "test-token"},
	})
	ws.WriteMessage(websocket.TextMessage, connectReq)

	// 3. Read hello-ok
	_, msg, err = ws.ReadMessage()
	require.NoError(t, err)
	frame, _ = ParseFrame(msg)
	res := frame.(*ResponseFrame)
	assert.True(t, res.OK)

	// 4. Verify node appears in registry
	nodes := gw.registry.List()
	require.Len(t, nodes, 1)
	assert.Equal(t, "iphone-test", nodes[0].NodeID)

	// 5. Invoke a command from the gateway side (simulating what Discord would do)
	go func() {
		// Read the invoke request event from the WS
		_, invokeMsg, _ := ws.ReadMessage()
		invokeFrame, _ := ParseFrame(invokeMsg)
		invokeEvt := invokeFrame.(*EventFrame)
		assert.Equal(t, "node.invoke.request", invokeEvt.Event)

		// Parse the invoke request payload
		var invokeReq NodeInvokeRequest
		json.Unmarshal(invokeEvt.Payload, &invokeReq)

		// Send back the result (as iOS would)
		resultReq, _ := MarshalRequest("req-2", "node.invoke.result", NodeInvokeResult{
			ID:          invokeReq.ID,
			NodeID:      "iphone-test",
			OK:          true,
			PayloadJSON: ptrStr(`{"lat":40.7128,"lon":-74.0060}`),
		})
		ws.WriteMessage(websocket.TextMessage, resultReq)
	}()

	// 6. Invoke from the gateway
	result, err := gw.invoker.Invoke(ctx, InvokeRequest{
		NodeID:    "iphone-test",
		Command:   "location.get",
		TimeoutMs: 5000,
	})
	require.NoError(t, err)
	assert.True(t, result.OK)
	assert.Contains(t, *result.PayloadJSON, "40.7128")
}

func TestIntegration_OperatorSessionNotRegistered(t *testing.T) {
	gw, err := New(GatewayConfig{
		Port:      0,
		AuthToken: "test-token",
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go gw.Run(ctx)

	require.Eventually(t, func() bool { return gw.server.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	ws, _, err := websocket.DefaultDialer.Dial("ws://"+gw.server.Addr()+"/ws", nil)
	require.NoError(t, err)
	defer ws.Close()

	_, _, _ = ws.ReadMessage() // challenge
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{
			ID: "openclaw-ios", Version: "1.0", Platform: "ios", Mode: "ui",
		},
		Role: "operator",
		Auth: &ConnectAuth{Token: "test-token"},
	})
	ws.WriteMessage(websocket.TextMessage, connectReq)
	_, _, _ = ws.ReadMessage() // hello-ok

	nodes := gw.registry.List()
	assert.Len(t, nodes, 0, "operator session should not be registered as node")
}

func TestIntegration_TickKeepAlive(t *testing.T) {
	gw, err := New(GatewayConfig{
		Port:         0,
		AuthToken:    "test-token",
		TickInterval: 100 * time.Millisecond, // fast for testing
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go gw.Run(ctx)

	require.Eventually(t, func() bool { return gw.server.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	// Connect and handshake
	ws, _, _ := websocket.DefaultDialer.Dial("ws://"+gw.server.Addr()+"/ws", nil)
	defer ws.Close()

	_, _, _ = ws.ReadMessage() // challenge
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-test", Version: "1.0", Platform: "ios", Mode: "node"},
		Auth:   &ConnectAuth{Token: "test-token"},
	})
	ws.WriteMessage(websocket.TextMessage, connectReq)
	_, _, _ = ws.ReadMessage() // hello-ok

	// Wait for tick events
	ws.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	tickCount := 0
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break // deadline exceeded
		}
		frame, _ := ParseFrame(msg)
		if evt, ok := frame.(*EventFrame); ok && evt.Event == "tick" {
			tickCount++
		}
	}

	assert.GreaterOrEqual(t, tickCount, 2, "should have received at least 2 ticks in 500ms at 100ms interval")
}

func TestIntegration_GracefulShutdown(t *testing.T) {
	gw, err := New(GatewayConfig{Port: 0, AuthToken: "test-token"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go gw.Run(ctx)

	require.Eventually(t, func() bool { return gw.server.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	// Connect and handshake
	ws, _, _ := websocket.DefaultDialer.Dial("ws://"+gw.server.Addr()+"/ws", nil)
	defer ws.Close()

	_, _, _ = ws.ReadMessage() // challenge
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-test", Version: "1.0", Platform: "ios", Mode: "node"},
		Auth:   &ConnectAuth{Token: "test-token"},
	})
	ws.WriteMessage(websocket.TextMessage, connectReq)
	_, _, _ = ws.ReadMessage() // hello-ok

	// Trigger shutdown
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	gw.Shutdown(shutdownCtx)

	// Client should see the connection close
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	sawShutdown := false
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			break
		}
		frame, _ := ParseFrame(msg)
		if evt, ok := frame.(*EventFrame); ok && evt.Event == "shutdown" {
			sawShutdown = true
		}
	}

	assert.True(t, sawShutdown, "should have received shutdown event before connection closed")
}

func TestIntegration_ReconnectAfterDrop(t *testing.T) {
	gw, err := New(GatewayConfig{Port: 0, AuthToken: "test-token"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go gw.Run(ctx)

	require.Eventually(t, func() bool { return gw.server.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	connectAndHandshake := func() *websocket.Conn {
		ws, _, err := websocket.DefaultDialer.Dial("ws://"+gw.server.Addr()+"/ws", nil)
		require.NoError(t, err)
		_, _, _ = ws.ReadMessage() // challenge
		req, _ := MarshalRequest("req-1", "connect", ConnectParams{
			MinProtocol: 3, MaxProtocol: 3,
			Client: ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
			Auth:   &ConnectAuth{Token: "test-token"},
		})
		ws.WriteMessage(websocket.TextMessage, req)
		_, _, _ = ws.ReadMessage() // hello-ok
		return ws
	}

	// First connection
	ws1 := connectAndHandshake()
	assert.Len(t, gw.registry.List(), 1)

	// Drop it
	ws1.Close()
	time.Sleep(100 * time.Millisecond)

	// Reconnect — same nodeID
	ws2 := connectAndHandshake()
	defer ws2.Close()

	// Should still be exactly 1 node, not 2
	nodes := gw.registry.List()
	assert.Len(t, nodes, 1)
	assert.Equal(t, "iphone-1", nodes[0].NodeID)
}
