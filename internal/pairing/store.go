package pairing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const PendingTTLMs = 5 * 60 * 1000 // 5 minutes

// PendingRequest represents a device waiting for operator approval.
type PendingRequest struct {
	RequestID   string   `json:"requestId"`
	DeviceID    string   `json:"deviceId"`
	PublicKey   string   `json:"publicKey"`              // base64url
	DisplayName string   `json:"displayName,omitempty"`
	Platform    string   `json:"platform,omitempty"`
	ClientID    string   `json:"clientId,omitempty"`
	ClientMode  string   `json:"clientMode,omitempty"`
	Role        string   `json:"role,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	RemoteIP    string   `json:"remoteIP,omitempty"`
	Silent      bool     `json:"silent,omitempty"`   // true for loopback auto-approve
	IsRepair    bool     `json:"isRepair,omitempty"` // true if re-pairing existing device
	Timestamp   int64    `json:"ts"`                 // Unix ms
}

// DeviceAuthToken is issued per-role after pairing approval.
type DeviceAuthToken struct {
	Token       string   `json:"token"`
	Role        string   `json:"role"`
	Scopes      []string `json:"scopes"`
	CreatedAtMs int64    `json:"createdAtMs"`
	RotatedAtMs int64    `json:"rotatedAtMs,omitempty"`
	RevokedAtMs int64    `json:"revokedAtMs,omitempty"`
	LastUsedMs  int64    `json:"lastUsedAtMs,omitempty"`
}

// PairedDevice represents a fully paired device.
type PairedDevice struct {
	DeviceID     string                     `json:"deviceId"`
	PublicKey    string                     `json:"publicKey"`
	DisplayName  string                     `json:"displayName,omitempty"`
	Platform     string                     `json:"platform,omitempty"`
	ClientID     string                     `json:"clientId,omitempty"`
	ClientMode   string                     `json:"clientMode,omitempty"`
	Role         string                     `json:"role,omitempty"`
	Scopes       []string                   `json:"scopes,omitempty"`
	RemoteIP     string                     `json:"remoteIP,omitempty"`
	Tokens       map[string]DeviceAuthToken `json:"tokens,omitempty"` // keyed by role
	CreatedAtMs  int64                      `json:"createdAtMs"`
	ApprovedAtMs int64                      `json:"approvedAtMs"`
}

// PairingState is the root state serialized to disk.
type PairingState struct {
	PendingByID    map[string]PendingRequest `json:"pendingById"`
	PairedByDevice map[string]PairedDevice   `json:"pairedByDeviceId"`
}

// DeviceMetadataPatch holds optional fields for updating device metadata.
// Only non-nil fields are applied.
type DeviceMetadataPatch struct {
	DisplayName *string
	Platform    *string
	ClientID    *string
	ClientMode  *string
	Role        *string
	Scopes      *[]string
	RemoteIP    *string
}

// Store manages persistent pairing state.
// All methods are concurrency-safe (internal mutex).
type Store struct {
	mu       sync.Mutex
	state    PairingState
	stateDir string
}

// NewStore loads existing state from disk or initializes empty state.
func NewStore(stateDir string) (*Store, error) {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	s := &Store{
		stateDir: stateDir,
		state: PairingState{
			PendingByID:    make(map[string]PendingRequest),
			PairedByDevice: make(map[string]PairedDevice),
		},
	}

	// Load pending
	if err := s.loadJSON("pending.json", &s.state.PendingByID); err != nil {
		return nil, err
	}

	// Load paired
	if err := s.loadJSON("paired.json", &s.state.PairedByDevice); err != nil {
		return nil, err
	}

	return s, nil
}

// --- Read operations ---

// GetPendingRequest returns a pending request by ID, or nil if not found.
func (s *Store) GetPendingRequest(requestID string) *PendingRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.state.PendingByID[requestID]
	if !ok {
		return nil
	}
	return &req
}

// GetPairedDevice returns a paired device by ID, or nil if not found.
func (s *Store) GetPairedDevice(deviceID string) *PairedDevice {
	s.mu.Lock()
	defer s.mu.Unlock()

	dev, ok := s.state.PairedByDevice[deviceID]
	if !ok {
		return nil
	}
	return &dev
}

// ListPending returns all pending requests sorted by timestamp descending.
func (s *Store) ListPending() []PendingRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]PendingRequest, 0, len(s.state.PendingByID))
	for _, req := range s.state.PendingByID {
		result = append(result, req)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp > result[j].Timestamp
	})

	return result
}

// ListPaired returns all paired devices sorted by approvedAt descending.
func (s *Store) ListPaired() []PairedDevice {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]PairedDevice, 0, len(s.state.PairedByDevice))
	for _, dev := range s.state.PairedByDevice {
		result = append(result, dev)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ApprovedAtMs > result[j].ApprovedAtMs
	})

	return result
}

// --- Write operations ---

// AddPending adds or overwrites a pending request and persists to disk.
func (s *Store) AddPending(req PendingRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.PendingByID[req.RequestID] = req
	return s.savePending()
}

// RemovePending removes a pending request by ID.
// Returns the removed request, or nil if not found.
func (s *Store) RemovePending(requestID string) *PendingRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	req, ok := s.state.PendingByID[requestID]
	if !ok {
		return nil
	}

	delete(s.state.PendingByID, requestID)
	s.savePending()
	return &req
}

// SetPaired adds or updates a paired device and persists to disk.
func (s *Store) SetPaired(device PairedDevice) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if device.Tokens == nil {
		device.Tokens = make(map[string]DeviceAuthToken)
	}
	s.state.PairedByDevice[device.DeviceID] = device
	return s.savePaired()
}

// UpdateDeviceMetadata applies a metadata patch to a paired device.
func (s *Store) UpdateDeviceMetadata(deviceID string, patch DeviceMetadataPatch) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dev, ok := s.state.PairedByDevice[deviceID]
	if !ok {
		return fmt.Errorf("device %q not found", deviceID)
	}

	if patch.DisplayName != nil {
		dev.DisplayName = *patch.DisplayName
	}
	if patch.Platform != nil {
		dev.Platform = *patch.Platform
	}
	if patch.ClientID != nil {
		dev.ClientID = *patch.ClientID
	}
	if patch.ClientMode != nil {
		dev.ClientMode = *patch.ClientMode
	}
	if patch.Role != nil {
		dev.Role = *patch.Role
	}
	if patch.Scopes != nil {
		dev.Scopes = *patch.Scopes
	}
	if patch.RemoteIP != nil {
		dev.RemoteIP = *patch.RemoteIP
	}

	s.state.PairedByDevice[deviceID] = dev
	return s.savePaired()
}

// SetDeviceToken sets a device's token for a given role.
func (s *Store) SetDeviceToken(deviceID, role string, token DeviceAuthToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dev, ok := s.state.PairedByDevice[deviceID]
	if !ok {
		return fmt.Errorf("device %q not found", deviceID)
	}

	if dev.Tokens == nil {
		dev.Tokens = make(map[string]DeviceAuthToken)
	}
	dev.Tokens[role] = token
	s.state.PairedByDevice[deviceID] = dev
	return s.savePaired()
}

// PruneExpiredPending removes entries older than PendingTTL.
// Returns the number of entries pruned.
func (s *Store) PruneExpiredPending(now int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	pruned := 0
	for id, req := range s.state.PendingByID {
		age := now - req.Timestamp
		if age > PendingTTLMs {
			delete(s.state.PendingByID, id)
			pruned++
		}
	}

	if pruned > 0 {
		s.savePending()
	}
	return pruned
}

// --- Persistence helpers ---

func (s *Store) savePending() error {
	return s.saveJSON("pending.json", s.state.PendingByID)
}

func (s *Store) savePaired() error {
	return s.saveJSON("paired.json", s.state.PairedByDevice)
}

// saveJSON writes data as JSON to a file using atomic rename.
func (s *Store) saveJSON(filename string, data interface{}) error {
	target := filepath.Join(s.stateDir, filename)
	tmp := target + ".tmp"

	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := os.WriteFile(tmp, bytes, 0600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}

	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", filename, err)
	}

	return nil
}

// loadJSON loads JSON from a file into target. Missing files are ignored.
func (s *Store) loadJSON(filename string, target interface{}) error {
	path := filepath.Join(s.stateDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh state
		}
		return fmt.Errorf("read %s: %w", filename, err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal %s: %w", filename, err)
	}

	return nil
}
