package pairing

import (
	"fmt"
	"time"
)

// Service orchestrates pairing: request/approve/reject/revoke/verify.
type Service struct {
	store *Store
}

// NewService creates a new pairing service wrapping the given store.
func NewService(store *Store) *Service {
	return &Service{store: store}
}

// PairingRequestInput holds fields for requesting device pairing.
type PairingRequestInput struct {
	DeviceID    string
	PublicKey   string
	DisplayName string
	Platform    string
	ClientID    string
	ClientMode  string
	Role        string
	Scopes      []string
	RemoteIP    string
	IsLocal     bool // true → silent auto-approve
}

// VerifyTokenParams holds fields for token verification.
type VerifyTokenParams struct {
	DeviceID string
	Token    string
	Role     string
	Scopes   []string
}

// VerifyTokenResult is the outcome of a token verification.
type VerifyTokenResult struct {
	OK     bool
	Reason string // "device-not-paired", "token-missing", "token-revoked",
	// "token-mismatch", "scope-mismatch"
}

// CheckPairingParams holds fields for checking pairing status during handshake.
type CheckPairingParams struct {
	DeviceID  string
	PublicKey string
	Role      string
	Scopes    []string
	IsLocal   bool
}

// PairingAction is the result of a pairing status check.
type PairingAction struct {
	Status    string // "paired", "pairing-required", "auto-approved"
	RequestID string // set when Status == "pairing-required"
	Device    *PairedDevice
}

// RequestPairing checks if a device needs pairing and creates a pending request.
// If already paired with matching public key, returns (nil, nil) — no action needed.
// If pending request already exists for this device, returns existing request.
// For new requests, returns the created PendingRequest.
func (s *Service) RequestPairing(req PairingRequestInput) (*PendingRequest, error) {
	if req.DeviceID == "" {
		return nil, fmt.Errorf("deviceID is required")
	}

	// Check if already paired with same key
	existing := s.store.GetPairedDevice(req.DeviceID)
	if existing != nil && existing.PublicKey == req.PublicKey {
		return nil, nil // already paired, no action
	}

	// Check if there's already a pending request for this device
	for _, pending := range s.store.ListPending() {
		if pending.DeviceID == req.DeviceID {
			return &pending, nil
		}
	}

	// Create new pending request
	isRepair := existing != nil && existing.PublicKey != req.PublicKey

	pending := PendingRequest{
		RequestID:   GenerateNonce(),
		DeviceID:    req.DeviceID,
		PublicKey:   req.PublicKey,
		DisplayName: req.DisplayName,
		Platform:    req.Platform,
		ClientID:    req.ClientID,
		ClientMode:  req.ClientMode,
		Role:        req.Role,
		Scopes:      req.Scopes,
		RemoteIP:    req.RemoteIP,
		Silent:      req.IsLocal,
		IsRepair:    isRepair,
		Timestamp:   time.Now().UnixMilli(),
	}

	if err := s.store.AddPending(pending); err != nil {
		return nil, fmt.Errorf("add pending: %w", err)
	}

	return &pending, nil
}

// Approve approves a pending pairing request.
// Generates a pairing token for the requested role.
// Moves the device from pending to paired.
// Returns the PairedDevice with token, or nil if requestID not found.
func (s *Service) Approve(requestID string) (*PairedDevice, error) {
	removed := s.store.RemovePending(requestID)
	if removed == nil {
		return nil, nil
	}

	now := time.Now().UnixMilli()

	// Check if device already exists (merge)
	existing := s.store.GetPairedDevice(removed.DeviceID)
	var device PairedDevice
	if existing != nil {
		device = *existing
		// Update metadata from the request
		device.PublicKey = removed.PublicKey
		if removed.DisplayName != "" {
			device.DisplayName = removed.DisplayName
		}
		if removed.Platform != "" {
			device.Platform = removed.Platform
		}
		if removed.ClientID != "" {
			device.ClientID = removed.ClientID
		}
		if removed.ClientMode != "" {
			device.ClientMode = removed.ClientMode
		}
		if removed.RemoteIP != "" {
			device.RemoteIP = removed.RemoteIP
		}
	} else {
		device = PairedDevice{
			DeviceID:     removed.DeviceID,
			PublicKey:    removed.PublicKey,
			DisplayName:  removed.DisplayName,
			Platform:     removed.Platform,
			ClientID:     removed.ClientID,
			ClientMode:   removed.ClientMode,
			Role:         removed.Role,
			Scopes:       removed.Scopes,
			RemoteIP:     removed.RemoteIP,
			CreatedAtMs:  now,
			ApprovedAtMs: now,
			Tokens:       make(map[string]DeviceAuthToken),
		}
	}

	device.ApprovedAtMs = now

	if err := s.store.SetPaired(device); err != nil {
		return nil, fmt.Errorf("set paired: %w", err)
	}

	// Generate token for the requested role
	if removed.Role != "" {
		token := DeviceAuthToken{
			Token:       GeneratePairingToken(),
			Role:        removed.Role,
			Scopes:      removed.Scopes,
			CreatedAtMs: now,
		}
		if err := s.store.SetDeviceToken(removed.DeviceID, removed.Role, token); err != nil {
			return nil, fmt.Errorf("set token: %w", err)
		}
	}

	// Re-fetch to get the updated device with token
	result := s.store.GetPairedDevice(removed.DeviceID)
	return result, nil
}

// Reject removes a pending pairing request without approving.
// Returns the rejected request, or nil if not found.
func (s *Service) Reject(requestID string) (*PendingRequest, error) {
	removed := s.store.RemovePending(requestID)
	return removed, nil
}

// VerifyDeviceToken validates a device token for a given role + scopes.
// Updates lastUsedMs on success.
func (s *Service) VerifyDeviceToken(params VerifyTokenParams) VerifyTokenResult {
	device := s.store.GetPairedDevice(params.DeviceID)
	if device == nil {
		return VerifyTokenResult{OK: false, Reason: "device-not-paired"}
	}

	tok, ok := device.Tokens[params.Role]
	if !ok {
		return VerifyTokenResult{OK: false, Reason: "token-missing"}
	}

	if tok.RevokedAtMs > 0 {
		return VerifyTokenResult{OK: false, Reason: "token-revoked"}
	}

	if !VerifyPairingToken(params.Token, tok.Token) {
		return VerifyTokenResult{OK: false, Reason: "token-mismatch"}
	}

	// Check scopes: all requested scopes must be present in token scopes
	if !scopesContainAll(tok.Scopes, params.Scopes) {
		return VerifyTokenResult{OK: false, Reason: "scope-mismatch"}
	}

	// Update lastUsedMs
	tok.LastUsedMs = time.Now().UnixMilli()
	s.store.SetDeviceToken(params.DeviceID, params.Role, tok)

	return VerifyTokenResult{OK: true}
}

// EnsureDeviceToken returns or creates a token for a paired device + role.
// If an existing non-revoked token with sufficient scopes exists, returns it.
// Otherwise generates a new one (rotating if previous existed).
func (s *Service) EnsureDeviceToken(deviceID, role string, scopes []string) *DeviceAuthToken {
	device := s.store.GetPairedDevice(deviceID)
	if device == nil {
		return nil
	}

	now := time.Now().UnixMilli()

	tok, exists := device.Tokens[role]
	if exists && tok.RevokedAtMs == 0 && scopesContainAll(tok.Scopes, scopes) {
		// Existing valid token with sufficient scopes
		return &tok
	}

	// Need new token (create or rotate)
	newTok := DeviceAuthToken{
		Token:       GeneratePairingToken(),
		Role:        role,
		Scopes:      scopes,
		CreatedAtMs: now,
	}

	if exists {
		newTok.RotatedAtMs = now
	}

	s.store.SetDeviceToken(deviceID, role, newTok)
	return &newTok
}

// RevokeDeviceToken marks a device's token for a role as revoked.
// Returns the revoked token, or nil if not found.
func (s *Service) RevokeDeviceToken(deviceID, role string) *DeviceAuthToken {
	device := s.store.GetPairedDevice(deviceID)
	if device == nil {
		return nil
	}

	tok, ok := device.Tokens[role]
	if !ok {
		return nil
	}

	tok.RevokedAtMs = time.Now().UnixMilli()
	s.store.SetDeviceToken(deviceID, role, tok)
	return &tok
}

// CheckPairingStatus determines what action is needed during handshake.
// Called by the conn module after signature verification succeeds.
func (s *Service) CheckPairingStatus(params CheckPairingParams) PairingAction {
	// Best-effort reload in case another process (CLI) updated the store.
	_ = s.store.Reload()

	device := s.store.GetPairedDevice(params.DeviceID)

	// Already paired with matching key
	if device != nil && device.PublicKey == params.PublicKey {
		return PairingAction{
			Status: "paired",
			Device: device,
		}
	}

	// Not paired or key mismatch — needs pairing
	if params.IsLocal {
		// Auto-approve for loopback
		req := PairingRequestInput{
			DeviceID:  params.DeviceID,
			PublicKey: params.PublicKey,
			Role:      params.Role,
			Scopes:    params.Scopes,
			IsLocal:   true,
		}

		pending, err := s.RequestPairing(req)
		if err != nil || pending == nil {
			return PairingAction{Status: "paired", Device: device}
		}

		approved, err := s.Approve(pending.RequestID)
		if err != nil {
			return PairingAction{Status: "pairing-required", RequestID: pending.RequestID}
		}

		return PairingAction{
			Status: "auto-approved",
			Device: approved,
		}
	}

	// Remote — create pending request
	req := PairingRequestInput{
		DeviceID:  params.DeviceID,
		PublicKey: params.PublicKey,
		Role:      params.Role,
		Scopes:    params.Scopes,
		IsLocal:   false,
	}

	pending, _ := s.RequestPairing(req)
	requestID := ""
	if pending != nil {
		requestID = pending.RequestID
	}

	return PairingAction{
		Status:    "pairing-required",
		RequestID: requestID,
	}
}

// scopesContainAll checks if 'have' contains all scopes in 'need'.
func scopesContainAll(have, need []string) bool {
	if len(need) == 0 {
		return true
	}

	haveSet := make(map[string]bool, len(have))
	for _, s := range have {
		haveSet[s] = true
	}

	for _, s := range need {
		if !haveSet[s] {
			return false
		}
	}
	return true
}
