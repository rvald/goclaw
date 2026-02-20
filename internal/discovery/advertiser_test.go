package discovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvertiser_StartStop(t *testing.T) {
	// 1. Setup config
	cfg := Config{
		InstanceName: "TestGateway",
		Port:         18789,
		LanHost:      "test-host.local",
		Meta: Metadata{
			Role:        "gateway",
			Transport:   "gateway",
			GatewayPort: "18789",
			DisplayName: "Test Gateway",
		},
	}

	// 2. Create Advertiser
	adv, err := NewAdvertiser(cfg)
	require.NoError(t, err)
	require.NotNil(t, adv)

	// 3. Start (Async)
	err = adv.Start()
	require.NoError(t, err)

	// Allow some time for goroutines to spin up
	time.Sleep(100 * time.Millisecond)

	// 4. Verify (We can't easily verify the network broadcast in a unit test 
	// without a full mDNS client listener, but we can verify internal state 
	// if we expose it, or just ensure no panic/error during lifecycle)
	
	// 5. Stop
	err = adv.Stop()
	require.NoError(t, err)
}

func TestAdvertiser_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "Valid",
			cfg: Config{
				InstanceName: "Valid",
				Port:         8080,
				LanHost:      "host.local",
			},
			wantErr: false,
		},
		{
			name: "Missing Port",
			cfg: Config{
				InstanceName: "NoPort",
				Port:         0,
			},
			wantErr: true,
		},
		{
			name: "Missing Name",
			cfg: Config{
				InstanceName: "",
				Port:         8080,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAdvertiser(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
