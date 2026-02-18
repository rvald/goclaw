package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/rvald/goclaw/internal/pairing"
)

// ServerConfig holds configuration for the gateway server.
type ServerConfig struct {
	Port       int
	Bind       string // "loopback" (127.0.0.1) or "lan" (0.0.0.0)
	Auth       AuthConfig
	PairingSvc *pairing.Service // optional — nil disables device pairing
}

// Server is an HTTP server that upgrades connections to WebSocket
// and manages Conn lifecycles.
type Server struct {
	config   ServerConfig
	handler  ConnHandler
	upgrader websocket.Upgrader
	httpSrv  *http.Server
	addr     string
	mu       sync.Mutex
	conns    []*Conn
	connsMu  sync.Mutex
}

// NewServer creates a new gateway server.
func NewServer(config ServerConfig, handler ConnHandler) *Server {
	return &Server{
		config:  config,
		handler: handler,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// Addr returns the address the server is listening on, or "" if not yet ready.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/health", s.handleHealth)

	bindAddr := "127.0.0.1"
	if s.config.Bind == "lan" {
		bindAddr = "0.0.0.0"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindAddr, s.config.Port))
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.addr = ln.Addr().String()
	s.httpSrv = &http.Server{Handler: mux}
	s.mu.Unlock()

	// Shut down when context is cancelled.
	go func() {
		<-ctx.Done()
		s.closeAllConns()
		s.httpSrv.Close()
	}()

	err = s.httpSrv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.closeAllConns()
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()
	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := NewConn(wsConn, s.config.Auth, s.handler)

	// Attach pairing service if configured
	if s.config.PairingSvc != nil {
		remoteAddr := r.RemoteAddr
		isLocal := isLoopback(remoteAddr)
		conn.WithPairing(s.config.PairingSvc, remoteAddr, isLocal)
	}

	s.connsMu.Lock()
	s.conns = append(s.conns, conn)
	s.connsMu.Unlock()

	conn.Run(r.Context())

	s.removeConn(conn)
}

// isLoopback checks if the remote address is a loopback address.
func isLoopback(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	// Handle IPv4-mapped IPv6 (::ffff:127.0.0.1) and bracket notation
	host = strings.TrimPrefix(host, "::ffff:")
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	// Fallback for "localhost" or similar
	return host == "localhost"
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) closeAllConns() {
	s.connsMu.Lock()
	conns := make([]*Conn, len(s.conns))
	copy(conns, s.conns)
	s.connsMu.Unlock()

	for _, c := range conns {
		c.ws.Close()
	}
}

func (s *Server) removeConn(conn *Conn) {
	s.connsMu.Lock()
	defer s.connsMu.Unlock()
	for i, c := range s.conns {
		if c == conn {
			s.conns = append(s.conns[:i], s.conns[i+1:]...)
			return
		}
	}
}
