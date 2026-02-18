package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"
)

// --- Test helpers ---

func newTestService(t *testing.T) (*Service, *Store) {
	t.Helper()
	s := newTestStore(t)
	return NewService(s), s
}

func makeTestKeypair(t *testing.T) (pubB64, deviceID string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	pubB64 = base64.RawURLEncoding.EncodeToString(pub)
	deviceID = DeriveDeviceID(pubB64)
	return
}

func pairDevice(t *testing.T, store *Store, deviceID, pubB64, role string, scopes []string) {
	t.Helper()
	device := PairedDevice{
		DeviceID:     deviceID,
		PublicKey:    pubB64,
		Role:         role,
		Scopes:       scopes,
		CreatedAtMs:  time.Now().UnixMilli() - 1000,
		ApprovedAtMs: time.Now().UnixMilli(),
		Tokens:       make(map[string]DeviceAuthToken),
	}
	store.SetPaired(device)
}

func pairDeviceWithToken(t *testing.T, store *Store, deviceID, pubB64, role, token string, scopes []string) {
	t.Helper()
	pairDevice(t, store, deviceID, pubB64, role, scopes)
	store.SetDeviceToken(deviceID, role, DeviceAuthToken{
		Token:       token,
		Role:        role,
		Scopes:      scopes,
		CreatedAtMs: time.Now().UnixMilli(),
	})
}

// --- RequestPairing ---

func TestRequestPairing(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T, store *Store) (string, string) // returns pubB64, deviceID
		input         func(pubB64, deviceID string) PairingRequestInput
		wantRequestID bool
		wantNilResult bool
		wantErr       bool
	}{
		{
			name: "new device creates pending",
			setup: func(t *testing.T, _ *Store) (string, string) {
				return makeTestKeypair(t)
			},
			input: func(pubB64, deviceID string) PairingRequestInput {
				return PairingRequestInput{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node", Scopes: []string{"scope1"},
				}
			},
			wantRequestID: true,
		},
		{
			name: "already paired with same key returns nil",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDevice(t, store, id, pub, "node", []string{"scope1"})
				return pub, id
			},
			input: func(pubB64, deviceID string) PairingRequestInput {
				return PairingRequestInput{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node", Scopes: []string{"scope1"},
				}
			},
			wantNilResult: true,
		},
		{
			name: "already pending returns existing request",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				store.AddPending(PendingRequest{
					RequestID: "existing-req",
					DeviceID:  id,
					PublicKey: pub,
					Timestamp: time.Now().UnixMilli(),
				})
				return pub, id
			},
			input: func(pubB64, deviceID string) PairingRequestInput {
				return PairingRequestInput{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node",
				}
			},
			wantRequestID: true,
		},
		{
			name: "re-pair with different key creates repair request",
			setup: func(t *testing.T, store *Store) (string, string) {
				oldPub, id := makeTestKeypair(t)
				pairDevice(t, store, id, oldPub, "node", nil)
				newPub, _ := makeTestKeypair(t)
				// Return newPub but with the OLD device ID
				return newPub, id
			},
			input: func(pubB64, deviceID string) PairingRequestInput {
				return PairingRequestInput{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node",
				}
			},
			wantRequestID: true,
		},
		{
			name: "empty deviceID returns error",
			setup: func(t *testing.T, _ *Store) (string, string) {
				return "key", ""
			},
			input: func(pubB64, deviceID string) PairingRequestInput {
				return PairingRequestInput{DeviceID: "", PublicKey: pubB64}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newTestService(t)
			pubB64, deviceID := tt.setup(t, store)
			input := tt.input(pubB64, deviceID)

			result, err := svc.RequestPairing(input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNilResult && result != nil {
				t.Errorf("expected nil result, got %+v", result)
			}
			if tt.wantRequestID && result == nil {
				t.Error("expected non-nil result with request ID")
			}
			if tt.wantRequestID && result != nil && result.RequestID == "" {
				t.Error("expected non-empty request ID")
			}
		})
	}
}

// --- Approve ---

func TestApprove(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(t *testing.T, store *Store) string // returns requestID
		wantDevice   bool
		wantNil      bool
		wantTokenSet bool
	}{
		{
			name: "approve pending creates paired device",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				store.AddPending(PendingRequest{
					RequestID: "req-1", DeviceID: id, PublicKey: pub,
					Role: "node", Scopes: []string{"scope1"},
					Timestamp: time.Now().UnixMilli(),
				})
				return "req-1"
			},
			wantDevice:   true,
			wantTokenSet: true,
		},
		{
			name: "approve non-existent returns nil",
			setup: func(t *testing.T, _ *Store) string {
				return "missing"
			},
			wantNil: true,
		},
		{
			name: "approve removes from pending",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				store.AddPending(PendingRequest{
					RequestID: "req-2", DeviceID: id, PublicKey: pub,
					Timestamp: time.Now().UnixMilli(),
				})
				return "req-2"
			},
			wantDevice: true,
		},
		{
			name: "approve existing device merges metadata",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				pairDevice(t, store, id, pub, "operator", nil)
				store.AddPending(PendingRequest{
					RequestID: "req-3", DeviceID: id, PublicKey: pub,
					Role: "node", DisplayName: "Updated",
					Timestamp: time.Now().UnixMilli(),
				})
				return "req-3"
			},
			wantDevice: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newTestService(t)
			reqID := tt.setup(t, store)

			result, err := svc.Approve(reqID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
			if tt.wantDevice && result == nil {
				t.Fatal("expected non-nil device")
			}
			if tt.wantTokenSet && result != nil {
				if len(result.Tokens) == 0 {
					t.Error("expected token to be set")
				}
			}

			// Verify removed from pending
			if tt.wantDevice && result != nil {
				if store.GetPendingRequest(reqID) != nil {
					t.Error("pending request should have been removed")
				}
			}
		})
	}
}

// --- Reject ---

func TestReject(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
		wantNil   bool
	}{
		{name: "reject existing pending", requestID: "req-1", wantNil: false},
		{name: "reject non-existent returns nil", requestID: "missing", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newTestService(t)
			if tt.requestID == "req-1" {
				pub, id := makeTestKeypair(t)
				store.AddPending(PendingRequest{
					RequestID: "req-1", DeviceID: id, PublicKey: pub,
					Timestamp: time.Now().UnixMilli(),
				})
			}

			result, err := svc.Reject(tt.requestID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

// --- VerifyDeviceToken ---

func TestVerifyDeviceToken(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, store *Store) (deviceID, token string)
		params func(deviceID, token string) VerifyTokenParams
		want   VerifyTokenResult
	}{
		{
			name: "valid token",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "tok-valid", []string{"scope1"})
				return id, "tok-valid"
			},
			params: func(deviceID, token string) VerifyTokenParams {
				return VerifyTokenParams{DeviceID: deviceID, Token: token, Role: "node", Scopes: []string{"scope1"}}
			},
			want: VerifyTokenResult{OK: true},
		},
		{
			name: "device not paired",
			setup: func(t *testing.T, _ *Store) (string, string) {
				return "no-such-device", "tok"
			},
			params: func(deviceID, token string) VerifyTokenParams {
				return VerifyTokenParams{DeviceID: deviceID, Token: token, Role: "node"}
			},
			want: VerifyTokenResult{OK: false, Reason: "device-not-paired"},
		},
		{
			name: "token missing for role",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDevice(t, store, id, pub, "node", nil)
				return id, "any-tok"
			},
			params: func(deviceID, token string) VerifyTokenParams {
				return VerifyTokenParams{DeviceID: deviceID, Token: token, Role: "node"}
			},
			want: VerifyTokenResult{OK: false, Reason: "token-missing"},
		},
		{
			name: "token revoked",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "tok-revoked", []string{"scope1"})
				store.SetDeviceToken(id, "node", DeviceAuthToken{
					Token: "tok-revoked", Role: "node", Scopes: []string{"scope1"},
					CreatedAtMs: time.Now().UnixMilli(),
					RevokedAtMs: time.Now().UnixMilli(),
				})
				return id, "tok-revoked"
			},
			params: func(deviceID, token string) VerifyTokenParams {
				return VerifyTokenParams{DeviceID: deviceID, Token: token, Role: "node", Scopes: []string{"scope1"}}
			},
			want: VerifyTokenResult{OK: false, Reason: "token-revoked"},
		},
		{
			name: "token mismatch",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "tok-real", []string{"scope1"})
				return id, "tok-wrong"
			},
			params: func(deviceID, token string) VerifyTokenParams {
				return VerifyTokenParams{DeviceID: deviceID, Token: token, Role: "node", Scopes: []string{"scope1"}}
			},
			want: VerifyTokenResult{OK: false, Reason: "token-mismatch"},
		},
		{
			name: "scope mismatch",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "tok-scoped", []string{"scope1"})
				return id, "tok-scoped"
			},
			params: func(deviceID, token string) VerifyTokenParams {
				return VerifyTokenParams{DeviceID: deviceID, Token: token, Role: "node", Scopes: []string{"scope1", "scope2"}}
			},
			want: VerifyTokenResult{OK: false, Reason: "scope-mismatch"},
		},
		{
			name: "updates lastUsedMs on success",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "tok-ts", []string{"scope1"})
				return id, "tok-ts"
			},
			params: func(deviceID, token string) VerifyTokenParams {
				return VerifyTokenParams{DeviceID: deviceID, Token: token, Role: "node", Scopes: []string{"scope1"}}
			},
			want: VerifyTokenResult{OK: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newTestService(t)
			deviceID, token := tt.setup(t, store)
			params := tt.params(deviceID, token)

			got := svc.VerifyDeviceToken(params)
			if got.OK != tt.want.OK {
				t.Errorf("OK = %v, want %v", got.OK, tt.want.OK)
			}
			if got.Reason != tt.want.Reason {
				t.Errorf("Reason = %q, want %q", got.Reason, tt.want.Reason)
			}

			// Check lastUsedMs updated on success
			if tt.name == "updates lastUsedMs on success" && got.OK {
				dev := store.GetPairedDevice(deviceID)
				if dev != nil {
					tok := dev.Tokens["node"]
					if tok.LastUsedMs == 0 {
						t.Error("lastUsedMs should have been updated")
					}
				}
			}
		})
	}
}

// --- EnsureDeviceToken ---

func TestEnsureDeviceToken(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, store *Store) string // returns deviceID
		role       string
		scopes     []string
		wantNil    bool
		wantRotate bool
	}{
		{
			name: "create new token for paired device",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				pairDevice(t, store, id, pub, "node", nil)
				return id
			},
			role: "node", scopes: []string{"scope1"},
			wantNil: false,
		},
		{
			name: "return existing valid token",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "existing-tok", []string{"scope1"})
				return id
			},
			role: "node", scopes: []string{"scope1"},
			wantNil: false, wantRotate: false,
		},
		{
			name: "rotate when scopes expanded",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "old-tok", []string{"scope1"})
				return id
			},
			role: "node", scopes: []string{"scope1", "scope2"},
			wantNil: false, wantRotate: true,
		},
		{
			name: "rotate when token is revoked",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "revoked-tok", []string{"scope1"})
				store.SetDeviceToken(id, "node", DeviceAuthToken{
					Token: "revoked-tok", Role: "node", Scopes: []string{"scope1"},
					CreatedAtMs: time.Now().UnixMilli(),
					RevokedAtMs: time.Now().UnixMilli(),
				})
				return id
			},
			role: "node", scopes: []string{"scope1"},
			wantNil: false, wantRotate: true,
		},
		{
			name: "nil for unpaired device",
			setup: func(t *testing.T, _ *Store) string {
				return "no-such-device"
			},
			role: "node", scopes: []string{"scope1"},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newTestService(t)
			deviceID := tt.setup(t, store)

			result := svc.EnsureDeviceToken(deviceID, tt.role, tt.scopes)

			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
			if !tt.wantNil && result == nil {
				t.Fatal("expected non-nil token")
			}
			if !tt.wantNil && result != nil {
				if result.Token == "" {
					t.Error("expected non-empty token")
				}
				if tt.wantRotate && result.RotatedAtMs == 0 {
					t.Error("expected rotatedAtMs to be set")
				}
			}
		})
	}
}

// --- RevokeDeviceToken ---

func TestRevokeDeviceToken(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, store *Store) string // returns deviceID
		role    string
		wantNil bool
	}{
		{
			name: "revoke existing token",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "tok-to-revoke", []string{"scope1"})
				return id
			},
			role: "node", wantNil: false,
		},
		{
			name: "revoke non-existent device",
			setup: func(t *testing.T, _ *Store) string {
				return "missing"
			},
			role: "node", wantNil: true,
		},
		{
			name: "revoke non-existent role",
			setup: func(t *testing.T, store *Store) string {
				pub, id := makeTestKeypair(t)
				pairDeviceWithToken(t, store, id, pub, "node", "tok", []string{"scope1"})
				return id
			},
			role: "missing", wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newTestService(t)
			deviceID := tt.setup(t, store)

			result := svc.RevokeDeviceToken(deviceID, tt.role)

			if tt.wantNil && result != nil {
				t.Errorf("expected nil, got %+v", result)
			}
			if !tt.wantNil {
				if result == nil {
					t.Fatal("expected non-nil revoked token")
				}
				if result.RevokedAtMs == 0 {
					t.Error("expected revokedAtMs to be set")
				}
				// Verify persisted
				dev := store.GetPairedDevice(deviceID)
				if dev != nil {
					tok := dev.Tokens[tt.role]
					if tok.RevokedAtMs == 0 {
						t.Error("revoked state not persisted")
					}
				}
			}
		})
	}
}

// --- CheckPairingStatus ---

func TestCheckPairingStatus(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, store *Store) (pubB64, deviceID string)
		params func(pubB64, deviceID string) CheckPairingParams
		want   string // expected Status
	}{
		{
			name: "paired device returns paired",
			setup: func(t *testing.T, store *Store) (string, string) {
				pub, id := makeTestKeypair(t)
				pairDevice(t, store, id, pub, "node", []string{"scope1"})
				return pub, id
			},
			params: func(pubB64, deviceID string) CheckPairingParams {
				return CheckPairingParams{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node", Scopes: []string{"scope1"}, IsLocal: false,
				}
			},
			want: "paired",
		},
		{
			name: "unpaired local auto-approves",
			setup: func(t *testing.T, _ *Store) (string, string) {
				return makeTestKeypair(t)
			},
			params: func(pubB64, deviceID string) CheckPairingParams {
				return CheckPairingParams{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node", Scopes: []string{"scope1"}, IsLocal: true,
				}
			},
			want: "auto-approved",
		},
		{
			name: "unpaired remote requires pairing",
			setup: func(t *testing.T, _ *Store) (string, string) {
				return makeTestKeypair(t)
			},
			params: func(pubB64, deviceID string) CheckPairingParams {
				return CheckPairingParams{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node", IsLocal: false,
				}
			},
			want: "pairing-required",
		},
		{
			name: "paired with wrong key requires re-pair",
			setup: func(t *testing.T, store *Store) (string, string) {
				oldPub, id := makeTestKeypair(t)
				pairDevice(t, store, id, oldPub, "node", nil)
				newPub, _ := makeTestKeypair(t)
				return newPub, id
			},
			params: func(pubB64, deviceID string) CheckPairingParams {
				return CheckPairingParams{
					DeviceID: deviceID, PublicKey: pubB64,
					Role: "node", IsLocal: false,
				}
			},
			want: "pairing-required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newTestService(t)
			pubB64, deviceID := tt.setup(t, store)
			params := tt.params(pubB64, deviceID)

			action := svc.CheckPairingStatus(params)
			if action.Status != tt.want {
				t.Errorf("Status = %q, want %q", action.Status, tt.want)
			}
		})
	}
}
