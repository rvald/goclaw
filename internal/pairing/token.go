package pairing

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
)

// GeneratePairingToken returns a 32-byte cryptographically random token
// encoded as base64url (no padding).
func GeneratePairingToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("pairing: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// VerifyPairingToken performs constant-time comparison of two tokens.
// Returns true if they match. Uses crypto/subtle.ConstantTimeCompare.
func VerifyPairingToken(provided, expected string) bool {
	// ConstantTimeCompare returns 0 for different lengths, which is correct.
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
