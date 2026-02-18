package gateway

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	. "github.com/rvald/goclaw/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pairingPkg "github.com/rvald/goclaw/internal/pairing"
)

var base64Url = base64.RawURLEncoding


type MockWebSocket struct {
	Incoming chan []byte // test writes here → conn reads
	Outgoing chan []byte // conn writes here → test reads
	closed   bool
	mu       sync.Mutex
}

func NewMockWebSocket() *MockWebSocket {
	return &MockWebSocket{
		Incoming: make(chan []byte, 10),
		Outgoing: make(chan []byte, 10),
	}
}

func (m *MockWebSocket) ReadMessage() (int, []byte, error) {
	msg, ok := <-m.Incoming
	if !ok {
		return 0, nil, fmt.Errorf("connection closed")
	}
	return 1, msg, nil // 1 = TextMessage
}

func (m *MockWebSocket) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("connection closed")
	}
	m.Outgoing <- data
	return nil
}

func (m *MockWebSocket) SetReadLimit(limit int64) {
	// No-op for mock
}

func (m *MockWebSocket) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *MockWebSocket) SetPongHandler(h func(appData string) error) {
}

func (m *MockWebSocket) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.Incoming)
	}
	return nil
}

type MockConnHandler struct {
	AuthenticatedCalls []*Conn
	Requests           []RequestFrame
	DisconnectedCalls  []*Conn
	mu                 sync.Mutex
}

func (h *MockConnHandler) OnAuthenticated(conn *Conn) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.AuthenticatedCalls = append(h.AuthenticatedCalls, conn)
	return nil
}

func (h *MockConnHandler) OnRequest(conn *Conn, req *RequestFrame) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Requests = append(h.Requests, *req)
	return nil
}

func (h *MockConnHandler) OnDisconnected(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.DisconnectedCalls = append(h.DisconnectedCalls, conn)
}

func TestConn_SendsChallenge(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	// First frame out should be a connect.challenge event
	frame := readFrame(t, ws)
	evt, ok := frame.(*EventFrame)
	require.True(t, ok, "expected EventFrame")
	assert.Equal(t, "connect.challenge", evt.Event)
	assert.NotNil(t, evt.Payload) // should contain nonce + ts
}

func TestConn_HandshakeHappy(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "token", Token: "secret"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	// 1. Read the challenge
	_ = readFrame(t, ws) // connect.challenge event
	// 2. Send connect request (simulating iOS)
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3,
		MaxProtocol: 3,
		Client:      ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
		Auth:        &ConnectAuth{Token: "secret"},
	})
	ws.Incoming <- connectReq
	// 3. Read hello-ok response
	frame := readFrame(t, ws)
	res, ok := frame.(*ResponseFrame)
	require.True(t, ok, "expected ResponseFrame")
	assert.Equal(t, "req-1", res.ID)
	assert.True(t, res.OK)
	// 4. Handler should have been notified
	time.Sleep(50 * time.Millisecond) // let goroutine process
	handler.mu.Lock()
	assert.Len(t, handler.AuthenticatedCalls, 1)
	handler.mu.Unlock()
	assert.Equal(t, StateAuthenticated, conn.State)
}

func TestConn_AuthFail(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "token", Token: "secret"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	// Read challenge
	_ = readFrame(t, ws)
	// Send connect with wrong token
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3,
		MaxProtocol: 3,
		Client:      ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
		Auth:        &ConnectAuth{Token: "wrong"},
	})
	ws.Incoming <- connectReq
	// Should get an error response
	frame := readFrame(t, ws)
	res, ok := frame.(*ResponseFrame)
	require.True(t, ok)
	assert.Equal(t, "req-1", res.ID)
	assert.False(t, res.OK)
	assert.NotNil(t, res.Error)
	assert.Equal(t, "UNAUTHORIZED", res.Error.Code)
	// Handler should NOT have been called
	handler.mu.Lock()
	assert.Empty(t, handler.AuthenticatedCalls)
	handler.mu.Unlock()
}

func TestConn_ProtocolMismatch(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	_ = readFrame(t, ws) // challenge
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 1,
		MaxProtocol: 2, // too low
		Client:      ClientInfo{ID: "old-app", Version: "0.1", Platform: "ios", Mode: "node"},
	})
	ws.Incoming <- connectReq
	frame := readFrame(t, ws)
	res, ok := frame.(*ResponseFrame)
	require.True(t, ok)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error.Message, "protocol")
}

func TestConn_FirstFrameMustBeConnect(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	_ = readFrame(t, ws) // challenge
	// Send a non-connect request as the first frame
	badReq, _ := MarshalRequest("req-1", "node.list", nil)
	ws.Incoming <- badReq
	frame := readFrame(t, ws)
	res, ok := frame.(*ResponseFrame)
	require.True(t, ok)
	assert.False(t, res.OK)
	assert.Contains(t, res.Error.Message, "connect")
}

func TestConn_RequestRoutingAfterAuth(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	// Complete handshake
	_ = readFrame(t, ws) // challenge
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
	})
	ws.Incoming <- connectReq
	_ = readFrame(t, ws) // hello-ok
	// Now send a real request
	invokeResult, _ := MarshalRequest("req-2", "node.invoke.result", map[string]any{
		"id": "inv-1", "nodeId": "iphone-1", "ok": true,
	})
	ws.Incoming <- invokeResult
	// Give the goroutine time to process
	time.Sleep(50 * time.Millisecond)
	handler.mu.Lock()
	require.Len(t, handler.Requests, 1)
	assert.Equal(t, "node.invoke.result", handler.Requests[0].Method)
	assert.Equal(t, "req-2", handler.Requests[0].ID)
	handler.mu.Unlock()
}

func TestConn_GracefulClose(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)
	// Complete handshake
	_ = readFrame(t, ws)
	connectReq, _ := MarshalRequest("req-1", "connect", ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
	})
	ws.Incoming <- connectReq
	_ = readFrame(t, ws)
	// Close the connection (simulates iOS disconnecting)
	ws.Close()
	time.Sleep(100 * time.Millisecond)
	handler.mu.Lock()
	assert.Len(t, handler.DisconnectedCalls, 1)
	handler.mu.Unlock()
	assert.Equal(t, StateClosed, conn.State)
}

func TestConn_ContextCancel(t *testing.T) {
	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	ctx, cancel := context.WithCancel(context.Background())
	go conn.Run(ctx)
	_ = readFrame(t, ws) // challenge
	// Cancel the context (simulates server shutdown)
	cancel()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, StateClosed, conn.State)
}

func readFrame(t *testing.T, ws *MockWebSocket) any {
	t.Helper()
	select {
	case data := <-ws.Outgoing:
		frame, err := ParseFrame(data)
		require.NoError(t, err)
		return frame
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frame from conn")
		return nil
	}
}

// --- Device Pairing Handshake Tests ---

// signDevicePayload creates a valid signed device connect payload for testing.
func signDevicePayload(t *testing.T, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, nonce string, params ConnectParams) *DeviceConnectPayload {
	t.Helper()
	pubKeyB64 := base64Url.EncodeToString(pubKey)
	deviceID := pairingPkg.DeriveDeviceID(pubKeyB64)
	signedAt := time.Now().UnixMilli()

	role := params.Role
	if role == "" {
		role = "node"
	}

	authToken := ""
	payload := pairingPkg.BuildAuthPayload(pairingPkg.AuthPayloadParams{
		DeviceID:   deviceID,
		ClientID:   params.Client.ID,
		ClientMode: params.Client.Mode,
		Role:       role,
		Scopes:     params.Caps,
		SignedAtMs: signedAt,
		Token:      authToken,
		Nonce:      nonce,
	})

	sig := ed25519.Sign(privKey, []byte(payload))

	return &DeviceConnectPayload{
		ID:        deviceID,
		PublicKey: pubKeyB64,
		Signature: base64Url.EncodeToString(sig),
		SignedAt:  signedAt,
		Nonce:     nonce,
	}
}

func TestConn_DevicePairing_LoopbackAutoApprove(t *testing.T) {
	// Setup: create a pairing service with temp store
	store, err := pairingPkg.NewStore(t.TempDir())
	require.NoError(t, err)
	svc := pairingPkg.NewService(store)

	// Generate keypair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	conn.WithPairing(svc, "127.0.0.1:54321", true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)

	// 1. Read challenge and extract nonce
	challengeFrame := readFrame(t, ws)
	evt := challengeFrame.(*EventFrame)
	require.Equal(t, "connect.challenge", evt.Event)
	challengePayload := make(map[string]any)
	json.Unmarshal(evt.Payload, &challengePayload)
	nonce := challengePayload["nonce"].(string)

	// 2. Build connect params with device identity
	connectParams := ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
	}
	dev := signDevicePayload(t, privKey, pubKey, nonce, connectParams)
	connectParams.Device = dev

	connectReq, _ := MarshalRequest("req-1", "connect", connectParams)
	ws.Incoming <- connectReq

	// 3. Should get success response with device token
	frame := readFrame(t, ws)
	res, ok := frame.(*ResponseFrame)
	require.True(t, ok)
	assert.Equal(t, "req-1", res.ID)
	assert.True(t, res.OK, "expected OK response, got error: %+v", res.Error)

	// 4. Verify conn has device ID set
	time.Sleep(50 * time.Millisecond)
	assert.NotEmpty(t, conn.DeviceID)
	assert.NotEmpty(t, conn.DeviceToken)

	// 5. Handler should be notified
	handler.mu.Lock()
	assert.Len(t, handler.AuthenticatedCalls, 1)
	handler.mu.Unlock()
}

func TestConn_DevicePairing_InvalidSignature(t *testing.T) {
	store, err := pairingPkg.NewStore(t.TempDir())
	require.NoError(t, err)
	svc := pairingPkg.NewService(store)

	pubKey, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	// Use a DIFFERENT private key to produce an invalid signature
	_, wrongPrivKey, _ := ed25519.GenerateKey(nil)

	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	conn.WithPairing(svc, "127.0.0.1:54321", true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)

	// Read challenge
	challengeFrame := readFrame(t, ws)
	evt := challengeFrame.(*EventFrame)
	challengePayload := make(map[string]any)
	json.Unmarshal(evt.Payload, &challengePayload)
	nonce := challengePayload["nonce"].(string)

	// Sign with wrong key
	connectParams := ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
	}
	dev := signDevicePayload(t, wrongPrivKey, pubKey, nonce, connectParams)
	connectParams.Device = dev

	connectReq, _ := MarshalRequest("req-1", "connect", connectParams)
	ws.Incoming <- connectReq

	// Should get INVALID_SIGNATURE error
	frame := readFrame(t, ws)
	res := frame.(*ResponseFrame)
	assert.False(t, res.OK)
	assert.Equal(t, "INVALID_SIGNATURE", res.Error.Code)
}

func TestConn_DevicePairing_NonceMismatch(t *testing.T) {
	store, err := pairingPkg.NewStore(t.TempDir())
	require.NoError(t, err)
	svc := pairingPkg.NewService(store)

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	conn.WithPairing(svc, "127.0.0.1:54321", true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)

	// Read challenge
	_ = readFrame(t, ws)

	// Sign with a WRONG nonce (not the challenge nonce)
	connectParams := ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
	}
	dev := signDevicePayload(t, privKey, pubKey, "wrong-nonce-value", connectParams)
	connectParams.Device = dev

	connectReq, _ := MarshalRequest("req-1", "connect", connectParams)
	ws.Incoming <- connectReq

	// Signature will fail because nonce is part of the payload and won't match
	// the challenge nonce stored on the conn
	frame := readFrame(t, ws)
	res := frame.(*ResponseFrame)
	assert.False(t, res.OK)
	// Could be INVALID_SIGNATURE (nonce in payload mismatch) or INVALID_NONCE
	assert.True(t, res.Error.Code == "INVALID_SIGNATURE" || res.Error.Code == "INVALID_NONCE")
}

func TestConn_DevicePairing_RemoteRequiresPairing(t *testing.T) {
	store, err := pairingPkg.NewStore(t.TempDir())
	require.NoError(t, err)
	svc := pairingPkg.NewService(store)

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	ws := NewMockWebSocket()
	handler := &MockConnHandler{}
	auth := AuthConfig{Mode: "none"}
	conn := NewConn(ws, ServerConfig{Auth: auth}, handler)
	conn.WithPairing(svc, "192.168.1.100:54321", false) // NOT local

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go conn.Run(ctx)

	// Read challenge
	challengeFrame := readFrame(t, ws)
	evt := challengeFrame.(*EventFrame)
	challengePayload := make(map[string]any)
	json.Unmarshal(evt.Payload, &challengePayload)
	nonce := challengePayload["nonce"].(string)

	connectParams := ConnectParams{
		MinProtocol: 3, MaxProtocol: 3,
		Client: ClientInfo{ID: "iphone-1", Version: "1.0", Platform: "ios", Mode: "node"},
	}
	dev := signDevicePayload(t, privKey, pubKey, nonce, connectParams)
	connectParams.Device = dev

	connectReq, _ := MarshalRequest("req-1", "connect", connectParams)
	ws.Incoming <- connectReq

	// Should get NOT_PAIRED error with requestId
	frame := readFrame(t, ws)
	res := frame.(*ResponseFrame)
	assert.False(t, res.OK)
	assert.Equal(t, "NOT_PAIRED", res.Error.Code)
	// Error message contains JSON with requestId
	assert.Contains(t, res.Error.Message, "requestId")
}

func TestServer_IsLoopback(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"127.0.0.1:54321", true},
		{"127.0.0.1", true},
		{"[::1]:54321", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"192.168.1.100:54321", false},
		{"10.0.0.1:8080", false},
		{"0.0.0.0:9999", false},
		{"localhost", true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			assert.Equal(t, tt.expected, isLoopback(tt.addr))
		})
	}
}