package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rvald/goclaw/internal/pairing"
	"github.com/rvald/goclaw/internal/protocol"
)

// ConnState represents the lifecycle state of a connection.
type ConnState string

const (
	StateConnecting    ConnState = "connecting"
	StateAuthenticated ConnState = "authenticated"
	StateClosed        ConnState = "closed"
)

// WebSocket is the interface for the underlying WebSocket connection.
type WebSocket interface {
	ReadMessage() (messageType int, data []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// ConnHandler receives lifecycle events from a Conn.
type ConnHandler interface {
	OnAuthenticated(conn *Conn) error
	OnRequest(conn *Conn, req *protocol.RequestFrame) error
	OnDisconnected(conn *Conn)
}

// Conn manages a single WebSocket connection through the handshake
// and authenticated message loop.
type Conn struct {
	ws            WebSocket
	auth          AuthConfig
	handler       ConnHandler
	State         ConnState
	ConnID        string
	ConnectParams *protocol.ConnectParams
	mu            sync.Mutex
	writeMu       sync.Mutex

	// Device pairing fields (optional — nil when pairing is not enabled).
	pairingSvc     *pairing.Service
	remoteAddr     string
	isLocal        bool
	challengeNonce string

	// Set after successful device verification.
	DeviceID    string
	DeviceToken string
}

// NewConn creates a new connection in the connecting state.
func NewConn(ws WebSocket, auth AuthConfig, handler ConnHandler) *Conn {
	return &Conn{
		ws:      ws,
		auth:    auth,
		handler: handler,
		State:   StateConnecting,
		ConnID:  generateID(),
	}
}

// WithPairing attaches a pairing service and connection metadata to the conn.
func (c *Conn) WithPairing(svc *pairing.Service, remoteAddr string, isLocal bool) {
	c.pairingSvc = svc
	c.remoteAddr = remoteAddr
	c.isLocal = isLocal
}

// SendEvent sends an event frame to this connection (thread-safe).
func (c *Conn) SendEvent(event string, payload any) error {
	data, err := protocol.MarshalEvent(event, payload)
	if err != nil {
		return err
	}
	return c.writeMessage(1, data)
}

// writeMessage sends data with write serialization.
func (c *Conn) writeMessage(messageType int, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteMessage(messageType, data)
}

// Run drives the connection lifecycle: challenge → connect → read loop.
// It blocks until the connection is closed or the context is cancelled.
func (c *Conn) Run(ctx context.Context) {
	defer c.shutdown()

	// Close websocket on context cancellation to unblock reads.
	go func() {
		<-ctx.Done()
		c.ws.Close()
	}()

	// 1. Send challenge
	if err := c.sendChallenge(); err != nil {
		return
	}

	// 2. Wait for connect request
	_, data, err := c.ws.ReadMessage()
	if err != nil {
		return
	}
	if err := c.processConnect(data); err != nil {
		return
	}

	// 3. Authenticated read loop
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		c.processRequest(data)
	}
}

func (c *Conn) sendChallenge() error {
	c.challengeNonce = generateID()
	payload := map[string]any{
		"nonce": c.challengeNonce,
		"ts":    time.Now().Unix(),
	}
	data, err := protocol.MarshalEvent("connect.challenge", payload)
	if err != nil {
		return err
	}
	return c.writeMessage(1, data)
}

func (c *Conn) processConnect(data []byte) error {
	frame, err := protocol.ParseFrame(data)
	if err != nil {
		return err
	}

	req, ok := frame.(*protocol.RequestFrame)
	if !ok {
		return fmt.Errorf("expected request frame")
	}

	if req.Method != "connect" {
		c.sendError(req.ID, "INVALID_METHOD", "first request must be connect")
		return fmt.Errorf("first request must be connect")
	}

	var params protocol.ConnectParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			c.sendError(req.ID, "INVALID_JSON", fmt.Sprintf("invalid connect params: %v", err))
			return err
		}
	}

	// Validate protocol version
	if err := protocol.ValidateConnect(params); err != nil {
		fe := err.(*protocol.FrameError)
		c.sendError(req.ID, fe.Code, fe.Message)
		return err
	}

	// Authenticate (legacy token auth)
	result := Authenticate(c.auth, params.Auth)
	if !result.OK {
		c.sendError(req.ID, "UNAUTHORIZED", result.Reason)
		return fmt.Errorf("auth failed: %s", result.Reason)
	}

	// Device identity verification (when pairing is enabled + client sends device payload)
	var deviceToken string
	if c.pairingSvc != nil && params.Device != nil {
		devToken, err := c.verifyDevice(req.ID, params)
		if err != nil {
			return err // error already sent to client
		}
		deviceToken = devToken
	}

	// Store connect params
	c.ConnectParams = &params
	if deviceToken != "" {
		c.DeviceToken = deviceToken
	}

	// Send success response (include auth info if we have a device token)
	var responsePayload any
	if deviceToken != "" {
		responsePayload = map[string]any{
			"auth": protocol.HelloAuthInfo{DeviceToken: deviceToken},
		}
	}

	resData, err := protocol.MarshalResponse(req.ID, true, responsePayload, nil)
	if err != nil {
		return err
	}
	if err := c.writeMessage(1, resData); err != nil {
		return err
	}

	c.mu.Lock()
	c.State = StateAuthenticated
	c.mu.Unlock()

	c.handler.OnAuthenticated(c)
	return nil
}

// verifyDevice performs device identity verification and pairing check.
// On success, returns the device auth token. On failure, sends error to client.
func (c *Conn) verifyDevice(reqID string, params protocol.ConnectParams) (string, error) {
	dev := params.Device

	// 1. Build the signing payload with full context
	role := params.Role
	if role == "" {
		role = "node"
	}

	authToken := ""
	if c.auth.Mode == "token" {
		authToken = c.auth.Token
	}

	payload := pairing.BuildAuthPayload(pairing.AuthPayloadParams{
		DeviceID:   dev.ID,
		ClientID:   params.Client.ID,
		ClientMode: params.Client.Mode,
		Role:       role,
		Scopes:     params.Caps,
		SignedAtMs: dev.SignedAt,
		Token:      authToken,
		Nonce:      dev.Nonce,
	})

	// 2. Verify the signature
	if !pairing.VerifySignature(dev.PublicKey, payload, dev.Signature) {
		c.sendError(reqID, "INVALID_SIGNATURE", "device signature verification failed")
		return "", fmt.Errorf("device signature verification failed")
	}

	// 3. Verify nonce matches the challenge we sent
	if dev.Nonce != c.challengeNonce {
		c.sendError(reqID, "INVALID_NONCE", "nonce does not match challenge")
		return "", fmt.Errorf("nonce mismatch")
	}

	// 4. Derive device ID and verify it matches
	derivedID := pairing.DeriveDeviceID(dev.PublicKey)
	if derivedID != dev.ID {
		c.sendError(reqID, "INVALID_DEVICE_ID", "device ID does not match public key")
		return "", fmt.Errorf("device ID mismatch")
	}
	c.DeviceID = derivedID

	// 5. Check pairing status
	action := c.pairingSvc.CheckPairingStatus(pairing.CheckPairingParams{
		DeviceID:  derivedID,
		PublicKey: dev.PublicKey,
		Role:      role,
		Scopes:    params.Caps,
		IsLocal:   c.isLocal,
	})

	switch action.Status {
	case "paired", "auto-approved":
		// Ensure device has a valid token
		tok := c.pairingSvc.EnsureDeviceToken(derivedID, role, params.Caps)
		if tok != nil {
			return tok.Token, nil
		}
		// Fallback: paired but token generation failed — still allow connection
		return "", nil

	case "pairing-required":
		errPayload := map[string]any{
			"requestId": action.RequestID,
		}
		errJSON, _ := json.Marshal(errPayload)
		c.sendError(reqID, "NOT_PAIRED", string(errJSON))
		return "", fmt.Errorf("device not paired, requestId=%s", action.RequestID)

	default:
		c.sendError(reqID, "PAIRING_ERROR", "unexpected pairing status")
		return "", fmt.Errorf("unexpected pairing status: %s", action.Status)
	}
}

func (c *Conn) processRequest(data []byte) {
	frame, err := protocol.ParseFrame(data)
	if err != nil {
		return
	}

	req, ok := frame.(*protocol.RequestFrame)
	if !ok {
		return
	}

	c.handler.OnRequest(c, req)
}

func (c *Conn) sendError(id, code, message string) {
	data, _ := protocol.MarshalResponse(id, false, nil, &protocol.ErrorShape{
		Code:    code,
		Message: message,
	})
	c.writeMessage(1, data)
}

func (c *Conn) shutdown() {
	c.mu.Lock()
	wasAuthenticated := c.State == StateAuthenticated
	c.State = StateClosed
	c.mu.Unlock()

	c.ws.Close()

	if wasAuthenticated {
		c.handler.OnDisconnected(c)
	}
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
