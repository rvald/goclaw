package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

// testKeypair holds a pre-generated Ed25519 keypair for tests.
type testKeypair struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
	pubB64     string // base64url-encoded public key
}

func newTestKeypair(t *testing.T) testKeypair {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return testKeypair{
		publicKey:  pub,
		privateKey: priv,
		pubB64:     base64.RawURLEncoding.EncodeToString(pub),
	}
}

func signPayload(t *testing.T, priv ed25519.PrivateKey, payload string) string {
	t.Helper()
	sig := ed25519.Sign(priv, []byte(payload))
	return base64.RawURLEncoding.EncodeToString(sig)
}

// --- DeriveDeviceID ---

func TestDeriveDeviceID(t *testing.T) {
	kp := newTestKeypair(t)

	tests := []struct {
		name      string
		publicKey string
		wantEmpty bool
	}{
		{
			name:      "valid 32-byte key",
			publicKey: kp.pubB64,
			wantEmpty: false,
		},
		{
			name:      "empty string",
			publicKey: "",
			wantEmpty: true,
		},
		{
			name:      "invalid base64",
			publicKey: "not-valid-base64!!!",
			wantEmpty: true,
		},
		{
			name:      "wrong length (16 bytes)",
			publicKey: base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveDeviceID(tt.publicKey)
			if tt.wantEmpty && got != "" {
				t.Errorf("expected empty, got %q", got)
			}
			if !tt.wantEmpty {
				if got == "" {
					t.Error("expected non-empty device ID")
				}
				// Should be 64-char hex (SHA-256)
				if len(got) != 64 {
					t.Errorf("expected 64 hex chars, got %d", len(got))
				}
			}
		})
	}

	// Deterministic: same key → same ID
	t.Run("deterministic", func(t *testing.T) {
		id1 := DeriveDeviceID(kp.pubB64)
		id2 := DeriveDeviceID(kp.pubB64)
		if id1 != id2 {
			t.Errorf("not deterministic: %q != %q", id1, id2)
		}
	})
}

// --- BuildAuthPayload ---

func TestBuildAuthPayload(t *testing.T) {
	tests := []struct {
		name   string
		params AuthPayloadParams
		want   string
	}{
		{
			name: "full payload with nonce",
			params: AuthPayloadParams{
				DeviceID:   "abc123",
				ClientID:   "openclaw-ios",
				ClientMode: "ui",
				Role:       "node",
				Scopes:     []string{"operator.admin", "operator.pairing"},
				SignedAtMs: 1700000000000,
				Token:      "tok_xyz",
				Nonce:      "nonce-uuid",
			},
			want: "v2|abc123|openclaw-ios|ui|node|operator.admin,operator.pairing|1700000000000|tok_xyz|nonce-uuid",
		},
		{
			name: "empty token and scopes",
			params: AuthPayloadParams{
				DeviceID:   "abc123",
				ClientID:   "openclaw-ios",
				ClientMode: "ui",
				Role:       "operator",
				Scopes:     nil,
				SignedAtMs: 1700000000000,
				Nonce:      "n",
			},
			want: "v2|abc123|openclaw-ios|ui|operator||1700000000000||n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAuthPayload(tt.params)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// --- VerifySignature ---

func TestVerifySignature(t *testing.T) {
	kp := newTestKeypair(t)
	kp2 := newTestKeypair(t)

	payload := "v2|abc123|openclaw-ios|ui|node|scope1|1700000000000|tok|nonce"
	validSig := signPayload(t, kp.privateKey, payload)

	tests := []struct {
		name      string
		publicKey string
		payload   string
		signature string
		want      bool
	}{
		{
			name:      "valid signature",
			publicKey: kp.pubB64,
			payload:   payload,
			signature: validSig,
			want:      true,
		},
		{
			name:      "wrong payload",
			publicKey: kp.pubB64,
			payload:   "different-payload",
			signature: validSig,
			want:      false,
		},
		{
			name:      "wrong key",
			publicKey: kp2.pubB64,
			payload:   payload,
			signature: validSig,
			want:      false,
		},
		{
			name:      "empty signature",
			publicKey: kp.pubB64,
			payload:   payload,
			signature: "",
			want:      false,
		},
		{
			name: "corrupt signature",
			publicKey: kp.pubB64,
			payload:   payload,
			signature: func() string {
				raw, _ := base64.RawURLEncoding.DecodeString(validSig)
				raw[0] ^= 0xFF // flip first byte
				return base64.RawURLEncoding.EncodeToString(raw)
			}(),
			want: false,
		},
		{
			name:      "empty public key",
			publicKey: "",
			payload:   payload,
			signature: validSig,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifySignature(tt.publicKey, tt.payload, tt.signature)
			if got != tt.want {
				t.Errorf("VerifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- GenerateNonce ---

func TestGenerateNonce(t *testing.T) {
	t.Run("non-empty", func(t *testing.T) {
		n := GenerateNonce()
		if n == "" {
			t.Error("expected non-empty nonce")
		}
	})

	t.Run("UUID format", func(t *testing.T) {
		n := GenerateNonce()
		// UUID v4: 8-4-4-4-12
		parts := strings.Split(n, "-")
		if len(parts) != 5 {
			t.Errorf("expected UUID format (5 parts), got %q", n)
		}
		wantLens := []int{8, 4, 4, 4, 12}
		for i, p := range parts {
			if len(p) != wantLens[i] {
				t.Errorf("part %d: got len %d, want %d (nonce=%q)", i, len(p), wantLens[i], n)
			}
		}
	})

	t.Run("unique across 100 calls", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			n := GenerateNonce()
			if seen[n] {
				t.Fatalf("duplicate nonce on call %d", i)
			}
			seen[n] = true
		}
	})
}

// --- NormalizePublicKey ---

func TestNormalizePublicKey(t *testing.T) {
	kp := newTestKeypair(t)

	tests := []struct {
		name      string
		input     string
		wantEmpty bool
	}{
		{
			name:      "valid key round-trips",
			input:     kp.pubB64,
			wantEmpty: false,
		},
		{
			name:      "empty string",
			input:     "",
			wantEmpty: true,
		},
		{
			name:      "invalid base64",
			input:     "!!!invalid!!!",
			wantEmpty: true,
		},
		{
			name:      "wrong length",
			input:     base64.RawURLEncoding.EncodeToString(make([]byte, 16)),
			wantEmpty: true,
		},
		{
			name: "standard base64 with padding re-encodes",
			input: func() string {
				// Encode with standard (padded) encoding — should still normalize
				return base64.URLEncoding.EncodeToString(kp.publicKey)
			}(),
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizePublicKey(tt.input)
			if tt.wantEmpty && got != "" {
				t.Errorf("expected empty, got %q", got)
			}
			if !tt.wantEmpty {
				if got == "" {
					t.Error("expected non-empty")
				}
				// Re-normalizing should be idempotent
				got2 := NormalizePublicKey(got)
				if got != got2 {
					t.Errorf("not idempotent: %q != %q", got, got2)
				}
			}
		})
	}
}

func TestDeriveDeviceID_MatchesPublicKey(t *testing.T) {
	// Two different keys produce different device IDs
	kp1 := newTestKeypair(t)
	kp2 := newTestKeypair(t)

	id1 := DeriveDeviceID(kp1.pubB64)
	id2 := DeriveDeviceID(kp2.pubB64)

	if id1 == id2 {
		t.Error("different keys produced same device ID")
	}

	// Verify the ID format is hex
	for _, c := range id1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("device ID contains non-hex char: %c (id=%q)", c, id1)
			break
		}
	}

	// Suppress unused import warning
	_ = fmt.Sprintf
}
