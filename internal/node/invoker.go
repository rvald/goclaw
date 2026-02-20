package node

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/rvald/goclaw/internal/protocol"
)

// NodeInvokeRequest is an alias for the protocol type, re-exported for
// convenience so callers don't need to import protocol directly.
type NodeInvokeRequest = protocol.NodeInvokeRequest

// NodeInvokeResult is an alias for the protocol type.
type NodeInvokeResult = protocol.NodeInvokeResult

// InvokeRequest is the input to Invoker.Invoke.
type InvokeRequest struct {
	NodeID    string
	Command   string
	TimeoutMs int
}

// InvokeResult is the output of Invoker.Invoke.
type InvokeResult struct {
	OK          bool
	PayloadJSON *string
	Error       *protocol.ErrorShape
}

// pendingInvoke tracks a single in-flight invocation.
type pendingInvoke struct {
	result chan protocol.NodeInvokeResult
	cancel chan struct{}
	nodeID string
}

// Invoker manages the request/response lifecycle for node invocations.
type Invoker struct {
	reg     *Registry
	pending map[string]*pendingInvoke
	mu      sync.Mutex
}

// NewInvoker creates a new invoker backed by the given registry.
func NewInvoker(reg *Registry) *Invoker {
	return &Invoker{
		reg:     reg,
		pending: make(map[string]*pendingInvoke),
	}
}

// Invoke sends a command to a node and waits for the result.
func (inv *Invoker) Invoke(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
	session, ok := inv.reg.Get(req.NodeID)
	if !ok {
		return InvokeResult{OK: false}, fmt.Errorf("node %q not connected", req.NodeID)
	}

	id := generateInvokeID()
	pi := &pendingInvoke{
		result: make(chan protocol.NodeInvokeResult, 1),
		cancel: make(chan struct{}),
		nodeID: req.NodeID,
	}

	inv.mu.Lock()
	inv.pending[id] = pi
	inv.mu.Unlock()

	defer func() {
		inv.mu.Lock()
		delete(inv.pending, id)
		inv.mu.Unlock()
	}()

	invokeReq := protocol.NodeInvokeRequest{
		ID:      id,
		NodeID:  req.NodeID,
		Command: req.Command,
	}

	if err := session.Send("node.invoke.request", invokeReq); err != nil {
		return InvokeResult{OK: false}, fmt.Errorf("send failed: %w", err)
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond

	select {
	case result := <-pi.result:
		return InvokeResult{
			OK:          result.OK,
			PayloadJSON: result.PayloadJSON,
			Error:       result.Error,
		}, nil
	case <-pi.cancel:
		return InvokeResult{OK: false}, fmt.Errorf("node disconnected")
	case <-time.After(timeout):
		return InvokeResult{OK: false}, fmt.Errorf("invoke timeout after %dms", req.TimeoutMs)
	case <-ctx.Done():
		return InvokeResult{OK: false}, ctx.Err()
	}
}

// HandleResult delivers a result from a node to the waiting Invoke call.
// Returns true if a matching pending invoke was found, false otherwise.
func (inv *Invoker) HandleResult(result protocol.NodeInvokeResult) bool {
	inv.mu.Lock()
	pi, ok := inv.pending[result.ID]
	inv.mu.Unlock()

	if !ok {
		return false
	}

	pi.result <- result
	return true
}

// CancelPendingForNode cancels all pending invocations targeting the given node.
// This should be called when a node disconnects.
func (inv *Invoker) CancelPendingForNode(nodeID string) {
	inv.mu.Lock()
	var toCancel []*pendingInvoke
	for _, pi := range inv.pending {
		if pi.nodeID == nodeID {
			toCancel = append(toCancel, pi)
		}
	}
	inv.mu.Unlock()

	for _, pi := range toCancel {
		close(pi.cancel)
	}
}

func generateInvokeID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
