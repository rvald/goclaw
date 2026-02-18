package gateway

import (
	"testing"

	. "github.com/rvald/goclaw/internal/protocol"
	"github.com/stretchr/testify/assert"
)

func TestAuth_TokenMatch(t *testing.T) {
	cfg := AuthConfig{Mode: "token", Token: "secret-123"}
	provided := &ConnectAuth{Token: "secret-123"}
	result := Authenticate(cfg, provided)
	assert.True(t, result.OK)
	assert.Equal(t, "token", result.Method)
	assert.Empty(t, result.Reason)
}

func TestAuth_TokenMismatch(t *testing.T) {
	cfg := AuthConfig{Mode: "token", Token: "secret-123"}
	provided := &ConnectAuth{Token: "wrong-token"}
	result := Authenticate(cfg, provided)
	assert.False(t, result.OK)
	assert.Equal(t, "token_mismatch", result.Reason)
}

func TestAuth_TokenMissing(t *testing.T) {
	cfg := AuthConfig{Mode: "token", Token: "secret-123"}
	result := Authenticate(cfg, nil)
	assert.False(t, result.OK)
	assert.Equal(t, "token_missing", result.Reason)
}

func TestAuth_TokenEmptyString(t *testing.T) {
	cfg := AuthConfig{Mode: "token", Token: "secret-123"}
	provided := &ConnectAuth{Token: ""}
	result := Authenticate(cfg, provided)
	assert.False(t, result.OK)
	assert.Equal(t, "token_missing", result.Reason)
}

func TestAuth_ModeNone(t *testing.T) {
	cfg := AuthConfig{Mode: "none"}
	result := Authenticate(cfg, nil)
	assert.True(t, result.OK)
	assert.Equal(t, "none", result.Method)
}

func TestAuth_ModeNoneIgnoresToken(t *testing.T) {
	cfg := AuthConfig{Mode: "none"}
	provided := &ConnectAuth{Token: "anything"}
	result := Authenticate(cfg, provided)
	assert.True(t, result.OK)
	assert.Equal(t, "none", result.Method)
}

func TestAuth_ConstantTimeCompare(t *testing.T) {
	cfg := AuthConfig{Mode: "token", Token: "secret-123-correct"}
	r1 := Authenticate(cfg, &ConnectAuth{Token: "secret-123-WRONG!"})
	r2 := Authenticate(cfg, &ConnectAuth{Token: "XXXXXXXXXXXXXXXX!"})
	assert.False(t, r1.OK)
	assert.False(t, r2.OK)
}