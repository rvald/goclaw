package gateway

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/rvald/goclaw/internal/node"
	"github.com/rvald/goclaw/internal/pairing"
	"github.com/rvald/goclaw/internal/protocol"
)

// Re-export node types for convenience.
type InvokeRequest = node.InvokeRequest
type InvokeResult = node.InvokeResult

// GatewayConfig configures the gateway.
type GatewayConfig struct {
	Port         int
	Bind         string // "loopback" or "lan"
	AuthToken    string
	TickInterval time.Duration
	PairingSvc   *pairing.Service // optional — nil disables device pairing
}

// Gateway is the top-level orchestrator that ties together the WebSocket
// server, node registry, and invoke system.
type Gateway struct {
	config   GatewayConfig
	server   *Server
	registry *node.Registry
	invoker  *node.Invoker
	conns    map[*Conn]bool
	connsMu  sync.Mutex
}

// New creates and wires up a new Gateway.
func New(config GatewayConfig) (*Gateway, error) {
	reg := node.NewRegistry()
	inv := node.NewInvoker(reg)

	gw := &Gateway{
		config:   config,
		registry: reg,
		invoker:  inv,
		conns:    make(map[*Conn]bool),
	}

	authCfg := AuthConfig{Mode: "none"}
	if config.AuthToken != "" {
		authCfg = AuthConfig{Mode: "token", Token: config.AuthToken}
	}

	gw.server = NewServer(ServerConfig{
		Port:       config.Port,
		Bind:       config.Bind,
		Auth:       authCfg,
		PairingSvc: config.PairingSvc,
	}, gw)
	return gw, nil
}

// Run starts the gateway server and tick loop. Blocks until ctx is cancelled.
// Run starts the gateway server and tick loop. Blocks until ctx is cancelled.
func (gw *Gateway) Run(ctx context.Context) error {
	if gw.config.TickInterval > 0 {
		go gw.tickLoop(ctx)
	}
	return gw.server.ListenAndServe(ctx)
}

// Invoker returns the gateway's invoker for external use (e.g. Discord bot).
func (gw *Gateway) Invoker() *node.Invoker { return gw.invoker }

// Registry returns the gateway's node registry for external use.
func (gw *Gateway) Registry() *node.Registry { return gw.registry }

// PairingSvc returns the gateway's pairing service for external use (e.g. Discord bot).
func (gw *Gateway) PairingSvc() *pairing.Service { return gw.config.PairingSvc }

// Shutdown sends a shutdown event to all connections and gracefully stops the server.
func (gw *Gateway) Shutdown(ctx context.Context) error {
	gw.broadcast("shutdown", nil)
	return gw.server.Shutdown(ctx)
}

// --- ConnHandler implementation ---

func (gw *Gateway) OnAuthenticated(conn *Conn) error {
	if conn.ConnectParams == nil {
		return nil
	}
	role := conn.ConnectParams.Role
	if role == "" {
		role = "node"
	}
	// Only register node sessions; operator sessions should not receive node commands.
	if role != "node" {
		return nil
	}

	session := node.NewNodeSession(
		conn.ConnectParams.Client.ID,
		conn.ConnID,
		conn.ConnectParams.Client.DisplayName,
		conn.ConnectParams.Client.Platform,
		conn.ConnectParams.Client.Version,
		conn.ConnectParams.Commands,
		func(event string, payload any) error {
			return conn.SendEvent(event, payload)
		},
	)

	gw.registry.Register(session)

	gw.connsMu.Lock()
	gw.conns[conn] = true
	gw.connsMu.Unlock()

	return nil
}

func (gw *Gateway) OnRequest(conn *Conn, req *protocol.RequestFrame) error {
	switch req.Method {
	case "node.invoke.result":
		var result protocol.NodeInvokeResult
		if req.Params != nil {
			json.Unmarshal(req.Params, &result)
		}
		gw.invoker.HandleResult(result)
	}
	return nil
}

func (gw *Gateway) OnDisconnected(conn *Conn) {
	gw.connsMu.Lock()
	delete(gw.conns, conn)
	gw.connsMu.Unlock()

	if conn.ConnID != "" {
		nodeID, ok := gw.registry.Unregister(conn.ConnID)
		if ok {
			gw.invoker.CancelPendingForNode(nodeID)
		}
	}
}

// --- tick & broadcast ---

func (gw *Gateway) tickLoop(ctx context.Context) {
	ticker := time.NewTicker(gw.config.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gw.broadcast("tick", map[string]any{"ts": time.Now().Unix()})
		}
	}
}

func (gw *Gateway) broadcast(event string, payload any) {
	gw.connsMu.Lock()
	conns := make([]*Conn, 0, len(gw.conns))
	for c := range gw.conns {
		conns = append(conns, c)
	}
	gw.connsMu.Unlock()

	for _, c := range conns {
		c.SendEvent(event, payload)
	}
}
