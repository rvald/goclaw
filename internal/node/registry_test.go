package node

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
    reg := NewRegistry()
    session := &NodeSession{
        NodeID:      "iphone-1",
        ConnID:      "conn-abc",
        DisplayName: "Ricardo's iPhone",
        Platform:    "ios",
        Commands:    []string{"camera.snap", "location.get"},
        sendFunc:    func(event string, payload any) error { return nil },
    }
    err := reg.Register(session)
    require.NoError(t, err)
    got, ok := reg.Get("iphone-1")
    assert.True(t, ok)
    assert.Equal(t, "Ricardo's iPhone", got.DisplayName)
    assert.Equal(t, "conn-abc", got.ConnID)
}

func TestRegistry_GetNotFound(t *testing.T) {
    reg := NewRegistry()
    _, ok := reg.Get("nonexistent")
    assert.False(t, ok)
}

func TestRegistry_Unregister(t *testing.T) {
    reg := NewRegistry()
    session := &NodeSession{
        NodeID: "iphone-1", ConnID: "conn-abc",
        sendFunc: func(event string, payload any) error { return nil },
    }
    reg.Register(session)
    nodeID, ok := reg.Unregister("conn-abc")
    assert.True(t, ok)
    assert.Equal(t, "iphone-1", nodeID)
    _, found := reg.Get("iphone-1")
    assert.False(t, found)
}

func TestRegistry_UnregisterNotFound(t *testing.T) {
    reg := NewRegistry()
    _, ok := reg.Unregister("nonexistent")
    assert.False(t, ok)
}

func TestRegistry_List(t *testing.T) {
    reg := NewRegistry()
    noop := func(event string, payload any) error { return nil }
    reg.Register(&NodeSession{NodeID: "iphone-1", ConnID: "conn-1", sendFunc: noop})
    reg.Register(&NodeSession{NodeID: "ipad-2", ConnID: "conn-2", sendFunc: noop})
    nodes := reg.List()
    assert.Len(t, nodes, 2)
    ids := []string{nodes[0].NodeID, nodes[1].NodeID}
    assert.Contains(t, ids, "iphone-1")
    assert.Contains(t, ids, "ipad-2")
}

func TestRegistry_DuplicateReplaces(t *testing.T) {
    reg := NewRegistry()
    noop := func(event string, payload any) error { return nil }
    reg.Register(&NodeSession{NodeID: "iphone-1", ConnID: "conn-old", sendFunc: noop})
    reg.Register(&NodeSession{NodeID: "iphone-1", ConnID: "conn-new", sendFunc: noop})
    got, ok := reg.Get("iphone-1")
    assert.True(t, ok)
    assert.Equal(t, "conn-new", got.ConnID)
    nodes := reg.List()
    assert.Len(t, nodes, 1) // not 2
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
    reg := NewRegistry()
    noop := func(event string, payload any) error { return nil }
    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(n int) {
            defer wg.Done()
            id := fmt.Sprintf("node-%d", n)
            reg.Register(&NodeSession{NodeID: id, ConnID: id, sendFunc: noop})
            reg.Get(id)
            reg.List()
            reg.Unregister(id)
        }(i)
    }
    wg.Wait()
    // If we get here without a race detector panic, we're good
}

func TestInvoke_HappyPath(t *testing.T) {
    reg := NewRegistry()
    inv := NewInvoker(reg)
    var captured NodeInvokeRequest
    session := &NodeSession{
        NodeID: "iphone-1", ConnID: "conn-1",
        sendFunc: func(event string, payload any) error {
            captured = payload.(NodeInvokeRequest)
            // Simulate iOS responding after a short delay
            go func() {
                time.Sleep(10 * time.Millisecond)
                inv.HandleResult(NodeInvokeResult{
                    ID:          captured.ID,
                    NodeID:      "iphone-1",
                    OK:          true,
                    PayloadJSON: ptrStr(`{"lat":40.7}`),
                })
            }()
            return nil
        },
    }
    reg.Register(session)
    result, err := inv.Invoke(context.Background(), InvokeRequest{
        NodeID:    "iphone-1",
        Command:   "location.get",
        TimeoutMs: 5000,
    })
    require.NoError(t, err)
    assert.True(t, result.OK)
    assert.Equal(t, `{"lat":40.7}`, *result.PayloadJSON)
    assert.Equal(t, "location.get", captured.Command)
}

func ptrStr(s string) *string { return &s }

func TestInvoke_Timeout(t *testing.T) {
    reg := NewRegistry()
    inv := NewInvoker(reg)
    session := &NodeSession{
        NodeID: "iphone-1", ConnID: "conn-1",
        sendFunc: func(event string, payload any) error {
            // Don't respond — simulate iOS being unresponsive
            return nil
        },
    }
    reg.Register(session)
    result, err := inv.Invoke(context.Background(), InvokeRequest{
        NodeID:    "iphone-1",
        Command:   "camera.snap",
        TimeoutMs: 100, // very short timeout for test speed
    })
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
    assert.False(t, result.OK)
}

func TestInvoke_NodeNotConnected(t *testing.T) {
    reg := NewRegistry()
    inv := NewInvoker(reg)
    result, err := inv.Invoke(context.Background(), InvokeRequest{
        NodeID:    "nonexistent",
        Command:   "camera.snap",
        TimeoutMs: 1000,
    })
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "not connected")
    assert.False(t, result.OK)
}

func TestInvoke_NodeDisconnects(t *testing.T) {
    reg := NewRegistry()
    inv := NewInvoker(reg)
    session := &NodeSession{
        NodeID: "iphone-1", ConnID: "conn-1",
        sendFunc: func(event string, payload any) error {
            // Simulate iOS disconnecting 50ms after receiving the command
            go func() {
                time.Sleep(50 * time.Millisecond)
                reg.Unregister("conn-1")
                inv.CancelPendingForNode("iphone-1") // key method
            }()
            return nil
        },
    }
    reg.Register(session)
    result, err := inv.Invoke(context.Background(), InvokeRequest{
        NodeID:    "iphone-1",
        Command:   "camera.snap",
        TimeoutMs: 5000, // long timeout — should NOT wait this long
    })
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "disconnected")
    assert.False(t, result.OK)
}

func TestInvoke_ContextCancelled(t *testing.T) {
    reg := NewRegistry()
    inv := NewInvoker(reg)
    session := &NodeSession{
        NodeID: "iphone-1", ConnID: "conn-1",
        sendFunc: func(event string, payload any) error { return nil },
    }
    reg.Register(session)
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // cancel immediately
    _, err := inv.Invoke(ctx, InvokeRequest{
        NodeID:    "iphone-1",
        Command:   "camera.snap",
        TimeoutMs: 5000,
    })
    assert.Error(t, err)
}

func TestInvoke_ConcurrentInvokes(t *testing.T) {
    reg := NewRegistry()
    inv := NewInvoker(reg)
    session := &NodeSession{
        NodeID: "iphone-1", ConnID: "conn-1",
        sendFunc: func(event string, payload any) error {
            req := payload.(NodeInvokeRequest)
            go func() {
                time.Sleep(10 * time.Millisecond)
                inv.HandleResult(NodeInvokeResult{
                    ID: req.ID, NodeID: "iphone-1",
                    OK:          true,
                    PayloadJSON: ptrStr(fmt.Sprintf(`{"cmd":"%s"}`, req.Command)),
                })
            }()
            return nil
        },
    }
    reg.Register(session)
    var wg sync.WaitGroup
    results := make([]InvokeResult, 2)
    commands := []string{"camera.snap", "location.get"}
    for i, cmd := range commands {
        wg.Add(1)
        go func(idx int, command string) {
            defer wg.Done()
            r, err := inv.Invoke(context.Background(), InvokeRequest{
                NodeID: "iphone-1", Command: command, TimeoutMs: 5000,
            })
            require.NoError(t, err)
            results[idx] = r
        }(i, cmd)
    }
    wg.Wait()
    assert.True(t, results[0].OK)
    assert.True(t, results[1].OK)
    assert.Contains(t, *results[0].PayloadJSON, "camera.snap")
    assert.Contains(t, *results[1].PayloadJSON, "location.get")
}

func TestHandleResult_UnknownID(t *testing.T) {
    reg := NewRegistry()
    inv := NewInvoker(reg)
    ok := inv.HandleResult(NodeInvokeResult{
        ID: "nonexistent", NodeID: "iphone-1", OK: true,
    })
    assert.False(t, ok) // no pending invoke with that ID
}