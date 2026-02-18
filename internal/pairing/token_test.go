package pairing

import (
	"encoding/base64"
	"testing"
)

func TestGeneratePairingToken(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T, token string)
	}{
		{
			name: "non-empty",
			check: func(t *testing.T, token string) {
				if token == "" {
					t.Error("expected non-empty token")
				}
			},
		},
		{
			name: "base64url decodable to 32 bytes",
			check: func(t *testing.T, token string) {
				raw, err := base64.RawURLEncoding.DecodeString(token)
				if err != nil {
					t.Fatalf("decode error: %v", err)
				}
				if len(raw) != 32 {
					t.Errorf("got %d bytes, want 32", len(raw))
				}
			},
		},
		{
			name: "unique across 100 calls",
			check: func(t *testing.T, _ string) {
				seen := make(map[string]bool)
				for i := 0; i < 100; i++ {
					tok := GeneratePairingToken()
					if seen[tok] {
						t.Fatalf("duplicate token on call %d", i)
					}
					seen[tok] = true
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := GeneratePairingToken()
			tt.check(t, token)
		})
	}
}

func TestVerifyPairingToken(t *testing.T) {
	tests := []struct {
		name     string
		provided string
		expected string
		want     bool
	}{
		{name: "matching tokens", provided: "abc", expected: "abc", want: true},
		{name: "mismatched tokens", provided: "abc", expected: "def", want: false},
		{name: "empty provided", provided: "", expected: "abc", want: false},
		{name: "empty expected", provided: "abc", expected: "", want: false},
		{name: "both empty", provided: "", expected: "", want: true},
		{name: "different lengths", provided: "short", expected: "muchlongertoken", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyPairingToken(tt.provided, tt.expected)
			if got != tt.want {
				t.Errorf("VerifyPairingToken(%q, %q) = %v, want %v",
					tt.provided, tt.expected, got, tt.want)
			}
		})
	}
}
