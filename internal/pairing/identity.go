package pairing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// Ed25519PublicKeySize is the expected size of a raw Ed25519 public key.
	Ed25519PublicKeySize = 32

	// SignatureSkewMs is the maximum clock skew allowed for signedAt (60 seconds).
	SignatureSkewMs = 60_000
)

// DeviceConnectIdentity is sent by the client in the connect params.
type DeviceConnectIdentity struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"` // base64url-encoded raw 32-byte Ed25519 public key
	Signature string `json:"signature"` // base64url-encoded Ed25519 signature
	SignedAt  int64  `json:"signedAt"`  // milliseconds since epoch
	Nonce     string `json:"nonce"`     // server-issued challenge nonce
}

// AuthPayloadParams holds the fields used to construct the signing payload.
type AuthPayloadParams struct {
	DeviceID   string
	ClientID   string
	ClientMode string
	Role       string
	Scopes     []string
	SignedAtMs int64
	Token      string // gateway auth token (may be empty)
	Nonce      string // challenge nonce
}

// DeriveDeviceID returns SHA-256 hex digest of the raw 32-byte public key.
// The publicKey is base64url-encoded.
// Returns "" if publicKey is invalid.
func DeriveDeviceID(publicKeyBase64Url string) string {
	raw, err := decodePublicKey(publicKeyBase64Url)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// BuildAuthPayload constructs the pipe-delimited signing payload.
// Format: "v2|deviceId|clientId|clientMode|role|scopes|signedAtMs|token|nonce"
// scopes is comma-joined. token defaults to "" if empty.
func BuildAuthPayload(p AuthPayloadParams) string {
	scopes := strings.Join(p.Scopes, ",")
	return fmt.Sprintf("v2|%s|%s|%s|%s|%s|%d|%s|%s",
		p.DeviceID, p.ClientID, p.ClientMode, p.Role,
		scopes, p.SignedAtMs, p.Token, p.Nonce)
}

// VerifySignature verifies an Ed25519 signature against a payload.
// publicKey is base64url-encoded raw 32-byte Ed25519 key.
// signature is base64url-encoded.
// Returns false on any error (bad key, bad sig, wrong length).
func VerifySignature(publicKeyBase64Url string, payload string, signatureBase64Url string) bool {
	pubRaw, err := decodePublicKey(publicKeyBase64Url)
	if err != nil {
		return false
	}

	sig, err := base64.RawURLEncoding.DecodeString(signatureBase64Url)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(ed25519.PublicKey(pubRaw), []byte(payload), sig)
}

// GenerateNonce returns a random UUID v4 string for the connect challenge.
func GenerateNonce() string {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		panic("pairing: crypto/rand failed: " + err.Error())
	}
	// Set version (4) and variant (RFC 4122)
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}

// NormalizePublicKey re-encodes a base64url public key to canonical form.
// Returns "" if invalid.
func NormalizePublicKey(publicKeyBase64Url string) string {
	raw, err := decodePublicKey(publicKeyBase64Url)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// decodePublicKey decodes a base64url-encoded public key and validates its length.
func decodePublicKey(publicKeyBase64Url string) ([]byte, error) {
	if publicKeyBase64Url == "" {
		return nil, fmt.Errorf("empty public key")
	}

	// Try RawURLEncoding first, then fall back to padded URLEncoding
	raw, err := base64.RawURLEncoding.DecodeString(publicKeyBase64Url)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(publicKeyBase64Url)
		if err != nil {
			return nil, fmt.Errorf("invalid base64url: %w", err)
		}
	}

	if len(raw) != Ed25519PublicKeySize {
		return nil, fmt.Errorf("wrong key length: got %d, want %d", len(raw), Ed25519PublicKeySize)
	}

	return raw, nil
}
