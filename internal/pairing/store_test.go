package pairing

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// --- Helpers ---

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func makePending(requestID, deviceID string, ts int64) PendingRequest {
	return PendingRequest{
		RequestID: requestID,
		DeviceID:  deviceID,
		PublicKey: "test-key-" + deviceID,
		Timestamp: ts,
	}
}

func makePaired(deviceID string, approvedAt int64) PairedDevice {
	return PairedDevice{
		DeviceID:     deviceID,
		PublicKey:    "test-key-" + deviceID,
		CreatedAtMs:  approvedAt - 1000,
		ApprovedAtMs: approvedAt,
	}
}

// --- AddPending + GetPending ---

func TestStoreAddAndGetPending(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *Store)
		queryID string
		wantNil bool
	}{
		{
			name: "get existing pending",
			setup: func(s *Store) {
				s.AddPending(makePending("req-1", "dev-1", 1000))
			},
			queryID: "req-1",
			wantNil: false,
		},
		{
			name:    "get non-existent",
			setup:   func(s *Store) {},
			queryID: "missing",
			wantNil: true,
		},
		{
			name: "duplicate requestID overwrites",
			setup: func(s *Store) {
				s.AddPending(makePending("req-1", "dev-1", 1000))
				s.AddPending(PendingRequest{
					RequestID:   "req-1",
					DeviceID:    "dev-2",
					PublicKey:   "key-2",
					DisplayName: "updated",
					Timestamp:   2000,
				})
			},
			queryID: "req-1",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			tt.setup(s)
			got := s.GetPendingRequest(tt.queryID)
			if tt.wantNil && got != nil {
				t.Errorf("expected nil, got %+v", got)
			}
			if !tt.wantNil && got == nil {
				t.Error("expected non-nil result")
			}
			// Verify overwrite
			if tt.name == "duplicate requestID overwrites" && got != nil {
				if got.DeviceID != "dev-2" {
					t.Errorf("overwrite failed: got deviceID=%q, want dev-2", got.DeviceID)
				}
			}
		})
	}
}

// --- SetPaired + GetPaired ---

func TestStoreSetAndGetPaired(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		wantNil  bool
	}{
		{name: "get existing paired", deviceID: "dev-1", wantNil: false},
		{name: "get non-existent", deviceID: "missing", wantNil: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			s.SetPaired(makePaired("dev-1", 5000))

			got := s.GetPairedDevice(tt.deviceID)
			if tt.wantNil && got != nil {
				t.Errorf("expected nil, got %+v", got)
			}
			if !tt.wantNil && got == nil {
				t.Error("expected non-nil result")
			}
		})
	}
}

// --- ListPending ---

func TestStoreListPending(t *testing.T) {
	s := newTestStore(t)
	s.AddPending(makePending("req-1", "dev-1", 1000))
	s.AddPending(makePending("req-2", "dev-2", 3000))
	s.AddPending(makePending("req-3", "dev-3", 2000))

	list := s.ListPending()
	if len(list) != 3 {
		t.Fatalf("got %d pending, want 3", len(list))
	}

	// Should be sorted by timestamp descending
	if list[0].RequestID != "req-2" {
		t.Errorf("first should be req-2 (ts=3000), got %s", list[0].RequestID)
	}
	if list[1].RequestID != "req-3" {
		t.Errorf("second should be req-3 (ts=2000), got %s", list[1].RequestID)
	}
	if list[2].RequestID != "req-1" {
		t.Errorf("third should be req-1 (ts=1000), got %s", list[2].RequestID)
	}
}

// --- ListPaired ---

func TestStoreListPaired(t *testing.T) {
	s := newTestStore(t)
	s.SetPaired(makePaired("dev-1", 1000))
	s.SetPaired(makePaired("dev-2", 3000))
	s.SetPaired(makePaired("dev-3", 2000))

	list := s.ListPaired()
	if len(list) != 3 {
		t.Fatalf("got %d paired, want 3", len(list))
	}

	// Should be sorted by approvedAt descending
	if list[0].DeviceID != "dev-2" {
		t.Errorf("first should be dev-2 (approved=3000), got %s", list[0].DeviceID)
	}
	if list[1].DeviceID != "dev-3" {
		t.Errorf("second should be dev-3 (approved=2000), got %s", list[1].DeviceID)
	}
	if list[2].DeviceID != "dev-1" {
		t.Errorf("third should be dev-1 (approved=1000), got %s", list[2].DeviceID)
	}
}

// --- PruneExpiredPending ---

func TestStorePruneExpiredPending(t *testing.T) {
	tests := []struct {
		name       string
		pendingAge int64 // ms ago from "now"
		now        int64
		wantPruned int
		wantRemain int
	}{
		{
			name:       "prune expired (6 min old)",
			pendingAge: 6 * 60 * 1000,
			now:        10_000_000,
			wantPruned: 1,
			wantRemain: 0,
		},
		{
			name:       "keep fresh (1 min old)",
			pendingAge: 1 * 60 * 1000,
			now:        10_000_000,
			wantPruned: 0,
			wantRemain: 1,
		},
		{
			name:       "boundary at 5 min (exactly TTL age is kept)",
			pendingAge: 5 * 60 * 1000,
			now:        10_000_000,
			wantPruned: 0,
			wantRemain: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ts := tt.now - tt.pendingAge
			s.AddPending(makePending("req-1", "dev-1", ts))

			pruned := s.PruneExpiredPending(tt.now)
			if pruned != tt.wantPruned {
				t.Errorf("pruned %d, want %d", pruned, tt.wantPruned)
			}
			remaining := len(s.ListPending())
			if remaining != tt.wantRemain {
				t.Errorf("remaining %d, want %d", remaining, tt.wantRemain)
			}
		})
	}

	// Mixed case: 2 expired, 1 fresh
	t.Run("mixed fresh and expired", func(t *testing.T) {
		s := newTestStore(t)
		now := int64(10_000_000)
		s.AddPending(makePending("old-1", "dev-1", now-6*60*1000)) // expired
		s.AddPending(makePending("old-2", "dev-2", now-7*60*1000)) // expired
		s.AddPending(makePending("new-1", "dev-3", now-1*60*1000)) // fresh

		pruned := s.PruneExpiredPending(now)
		if pruned != 2 {
			t.Errorf("pruned %d, want 2", pruned)
		}
		remaining := s.ListPending()
		if len(remaining) != 1 {
			t.Errorf("remaining %d, want 1", len(remaining))
		}
		if remaining[0].RequestID != "new-1" {
			t.Errorf("remaining should be new-1, got %s", remaining[0].RequestID)
		}
	})
}

// --- Persistence ---

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()

	// Create store, add data
	s1, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	s1.AddPending(makePending("req-1", "dev-1", 1000))
	s1.SetPaired(makePaired("dev-2", 2000))

	// Create new store from same directory — should load state
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore reload: %v", err)
	}

	if got := s2.GetPendingRequest("req-1"); got == nil {
		t.Error("pending req-1 not loaded from disk")
	}
	if got := s2.GetPairedDevice("dev-2"); got == nil {
		t.Error("paired dev-2 not loaded from disk")
	}
}

// --- Atomic Write ---

func TestStoreAtomicWrite(t *testing.T) {
	s := newTestStore(t)
	s.AddPending(makePending("req-1", "dev-1", 1000))
	s.SetPaired(makePaired("dev-1", 2000))

	// Check file permissions
	pendingPath := filepath.Join(s.stateDir, "pending.json")
	pairedPath := filepath.Join(s.stateDir, "paired.json")

	for _, path := range []string{pendingPath, pairedPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("%s has perm %o, want 0600", path, perm)
		}
	}
}

// --- RemovePending ---

func TestStoreRemovePending(t *testing.T) {
	s := newTestStore(t)
	s.AddPending(makePending("req-1", "dev-1", 1000))

	// Remove existing
	got := s.RemovePending("req-1")
	if got == nil {
		t.Error("expected non-nil on remove existing")
	}
	if s.GetPendingRequest("req-1") != nil {
		t.Error("req-1 still present after remove")
	}

	// Remove non-existent
	got = s.RemovePending("missing")
	if got != nil {
		t.Error("expected nil on remove non-existent")
	}
}

// --- SetDeviceToken ---

func TestStoreSetDeviceToken(t *testing.T) {
	s := newTestStore(t)
	s.SetPaired(makePaired("dev-1", 1000))

	// Set token on paired device
	token := DeviceAuthToken{
		Token:       "tok-123",
		Role:        "node",
		Scopes:      []string{"scope1"},
		CreatedAtMs: 2000,
	}
	err := s.SetDeviceToken("dev-1", "node", token)
	if err != nil {
		t.Fatalf("SetDeviceToken: %v", err)
	}

	device := s.GetPairedDevice("dev-1")
	if device == nil {
		t.Fatal("device not found after SetDeviceToken")
	}
	if device.Tokens == nil {
		t.Fatal("tokens map is nil")
	}
	tok, ok := device.Tokens["node"]
	if !ok {
		t.Fatal("token for role 'node' not found")
	}
	if tok.Token != "tok-123" {
		t.Errorf("token = %q, want tok-123", tok.Token)
	}

	// Set on non-existent device should error
	err = s.SetDeviceToken("missing", "node", token)
	if err == nil {
		t.Error("expected error for non-existent device")
	}
}

// --- Concurrency ---

func TestStoreConcurrency(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup

	// 10 goroutines writing and reading concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reqID := "req-" + time.Now().Format("150405.000") + "-" + string(rune('A'+i))
			devID := "dev-" + string(rune('A'+i))
			s.AddPending(makePending(reqID, devID, int64(i*1000)))
			s.ListPending()
			s.GetPendingRequest(reqID)
		}(i)
	}

	wg.Wait()
	// If we get here without panicking, concurrency is safe
}

// --- UpdateDeviceMetadata ---

func TestStoreUpdateDeviceMetadata(t *testing.T) {
	s := newTestStore(t)
	s.SetPaired(makePaired("dev-1", 1000))

	name := "My iPhone"
	platform := "ios"
	err := s.UpdateDeviceMetadata("dev-1", DeviceMetadataPatch{
		DisplayName: &name,
		Platform:    &platform,
	})
	if err != nil {
		t.Fatalf("UpdateDeviceMetadata: %v", err)
	}

	device := s.GetPairedDevice("dev-1")
	if device.DisplayName != "My iPhone" {
		t.Errorf("DisplayName = %q, want 'My iPhone'", device.DisplayName)
	}
	if device.Platform != "ios" {
		t.Errorf("Platform = %q, want 'ios'", device.Platform)
	}

	// Non-existent device should error
	err = s.UpdateDeviceMetadata("missing", DeviceMetadataPatch{DisplayName: &name})
	if err == nil {
		t.Error("expected error for non-existent device")
	}
}
