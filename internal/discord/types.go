package discord

import (
	"context"

	"github.com/rvald/goclaw/internal/node"
	"github.com/rvald/goclaw/internal/pairing"
)

// Type aliases so callers don't need to import node directly.
type InvokeRequest = node.InvokeRequest
type InvokeResult = node.InvokeResult
type NodeSession = node.NodeSession

// Type aliases for pairing types.
type PairedDevice = pairing.PairedDevice
type PendingRequest = pairing.PendingRequest

// Invoker sends commands to nodes and waits for results.
type Invoker interface {
	Invoke(ctx context.Context, req InvokeRequest) (InvokeResult, error)
}

// NodeRegistry provides read access to connected nodes.
type NodeRegistry interface {
	List() []*NodeSession
	Get(id string) (*NodeSession, bool)
}

// PairingService provides pairing operations for Discord commands.
type PairingService interface {
	Approve(requestID string) (*PairedDevice, error)
	Reject(requestID string) (*PendingRequest, error)
	RevokeDeviceToken(deviceID, role string) *pairing.DeviceAuthToken
}

// PairingStore provides read-only pairing state for Discord commands.
type PairingStore interface {
	ListPending() []PendingRequest
	ListPaired() []PairedDevice
}

