package node

import (
	"sync"
)

// NodeSession represents a connected node (e.g. an iPhone).
type NodeSession struct {
	NodeID      string
	ConnID      string
	DisplayName string
	Platform    string
	Version     string
	Commands    []string
	sendFunc    func(event string, payload any) error
}

// Send dispatches an event to this node's underlying connection.
func (s *NodeSession) Send(event string, payload any) error {
	return s.sendFunc(event, payload)
}

// NewNodeSession creates a NodeSession with the given send function.
func NewNodeSession(nodeID, connID, displayName, platform, version string, commands []string, send func(string, any) error) *NodeSession {
	return &NodeSession{
		NodeID:      nodeID,
		ConnID:      connID,
		DisplayName: displayName,
		Platform:    platform,
		Version:     version,
		Commands:    commands,
		sendFunc:    send,
	}
}

// Registry is a thread-safe store of connected node sessions.
type Registry struct {
	byNodeID map[string]*NodeSession
	byConnID map[string]string // connID → nodeID
	mu       sync.RWMutex
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		byNodeID: make(map[string]*NodeSession),
		byConnID: make(map[string]string),
	}
}

// Register adds or replaces a node session.
func (r *Registry) Register(session *NodeSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If this nodeID already exists, clean up the old connID mapping.
	if old, exists := r.byNodeID[session.NodeID]; exists {
		delete(r.byConnID, old.ConnID)
	}

	r.byNodeID[session.NodeID] = session
	r.byConnID[session.ConnID] = session.NodeID
	return nil
}

// Get retrieves a node session by nodeID.
func (r *Registry) Get(nodeID string) (*NodeSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byNodeID[nodeID]
	return s, ok
}

// Unregister removes a node session by connID. Returns the nodeID and true
// if found, or empty string and false if not.
func (r *Registry) Unregister(connID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	nodeID, ok := r.byConnID[connID]
	if !ok {
		return "", false
	}

	delete(r.byNodeID, nodeID)
	delete(r.byConnID, connID)
	return nodeID, true
}

// List returns a snapshot of all connected node sessions.
func (r *Registry) List() []*NodeSession {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*NodeSession, 0, len(r.byNodeID))
	for _, s := range r.byNodeID {
		out = append(out, s)
	}
	return out
}
