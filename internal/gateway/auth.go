package gateway

import (
	"crypto/subtle"

	"github.com/rvald/goclaw/internal/protocol"
)

// AuthConfig holds the server-side authentication settings.
type AuthConfig struct {
	Mode  string `json:"mode"`  // "none" or "token"
	Token string `json:"token"` // required when Mode == "token"
}

// AuthResult is the outcome of an authentication attempt.
type AuthResult struct {
	OK     bool   // whether authentication succeeded
	Method string // which auth method was used (e.g. "token", "none")
	Reason string // failure reason, empty on success
}

// Authenticate checks the provided credentials against the server config.
func Authenticate(cfg AuthConfig, provided *protocol.ConnectAuth) AuthResult {
	switch cfg.Mode {

	case "none":
		return AuthResult{OK: true, Method: "none"}

	case "token":
		if provided == nil || provided.Token == "" {
			return AuthResult{OK: false, Method: "token", Reason: "token_missing"}
		}

		if subtle.ConstantTimeCompare([]byte(cfg.Token), []byte(provided.Token)) != 1 {
			return AuthResult{OK: false, Method: "token", Reason: "token_mismatch"}
		}

		return AuthResult{OK: true, Method: "token"}

	default:
		return AuthResult{OK: false, Reason: "unknown_auth_mode"}
	}
}
