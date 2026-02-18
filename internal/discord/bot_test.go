package discord

import (
	"context"
	"fmt"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrStr(s string) *string { return &s }

// Mocks
type MockInvoker struct {
    InvokeFn func(ctx context.Context, req InvokeRequest) (InvokeResult, error)
}
func (m *MockInvoker) Invoke(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
    return m.InvokeFn(ctx, req)
}

type MockRegistry struct {
    nodes []*NodeSession
}

func (m *MockRegistry) List() []*NodeSession { 
	return m.nodes 
}

func (m *MockRegistry) Get(id string) (*NodeSession, bool) {
    for _, n := range m.nodes {
        if n.NodeID == id {
            return n, true
        }
    }
    return nil, false
}

// Tests

func TestBot_EmptyTokenErrors(t *testing.T) {
    _, err := NewBot(BotConfig{Token: ""})
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "token")
}

func TestBot_CommandConversion(t *testing.T) {
    cmds := []SlashCommand{
        {
            Name:        "snap",
            Description: "Take a photo from the iOS device camera",
            Options: []*discordgo.ApplicationCommandOption{
                {
                    Type:        discordgo.ApplicationCommandOptionString,
                    Name:        "facing",
                    Description: "Camera facing direction",
                    Choices: []*discordgo.ApplicationCommandOptionChoice{
                        {Name: "Back", Value: "back"},
                        {Name: "Front", Value: "front"},
                    },
                },
            },
        },
    }
    // Convert to discordgo format
    appCmds := toApplicationCommands(cmds)
    require.Len(t, appCmds, 1)
    assert.Equal(t, "snap", appCmds[0].Name)
    require.Len(t, appCmds[0].Options, 1)
    assert.Equal(t, "facing", appCmds[0].Options[0].Name)
    assert.Len(t, appCmds[0].Options[0].Choices, 2)
}

func TestHandler_Snap_Success(t *testing.T) {
    invoker := &MockInvoker{
        InvokeFn: func(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
            assert.Equal(t, "camera.snap", req.Command)
            return InvokeResult{
                OK:          true,
                PayloadJSON: ptrStr(`{"imageBase64":"iVBORw0KGgo=","format":"png","width":1920,"height":1080}`),
            }, nil
        },
    }
    registry := &MockRegistry{
        nodes: []*NodeSession{{NodeID: "iphone-1", DisplayName: "Ricardo's iPhone"}},
    }
    router := NewCommandRouter(invoker, registry)
    resp := router.HandleSnap(context.Background(), "iphone-1", "back", 80)
    assert.True(t, resp.OK)
    assert.Contains(t, resp.Message, "Ricardo's iPhone")
    assert.NotEmpty(t, resp.ImageData) // decoded base64
}

func TestHandler_Snap_NodeOffline(t *testing.T) {
    invoker := &MockInvoker{
        InvokeFn: func(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
            return InvokeResult{}, fmt.Errorf("node not connected")
        },
    }
    registry := &MockRegistry{nodes: nil}
    router := NewCommandRouter(invoker, registry)
    resp := router.HandleSnap(context.Background(), "", "back", 80)
    assert.False(t, resp.OK)
    assert.Contains(t, resp.Message, "No iOS device connected")
}

func TestHandler_Locate_Success(t *testing.T) {
    invoker := &MockInvoker{
        InvokeFn: func(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
            assert.Equal(t, "location.get", req.Command)
            return InvokeResult{
                OK:          true,
                PayloadJSON: ptrStr(`{"latitude":40.7128,"longitude":-74.0060,"altitude":10.5,"accuracy":5.0}`),
            }, nil
        },
    }
    registry := &MockRegistry{
        nodes: []*NodeSession{{NodeID: "iphone-1"}},
    }
    router := NewCommandRouter(invoker, registry)
    resp := router.HandleLocate(context.Background(), "iphone-1")
    assert.True(t, resp.OK)
    assert.Contains(t, resp.Message, "40.7128")
    assert.Contains(t, resp.Message, "-74.0060")
    assert.Contains(t, resp.Message, "google.com/maps")
}

func TestHandler_Status_Success(t *testing.T) {
    invoker := &MockInvoker{
        InvokeFn: func(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
            assert.Equal(t, "device.status", req.Command)
            return InvokeResult{
                OK: true,
                PayloadJSON: ptrStr(`{
                    "battery": {"level": 0.85, "state": "charging"},
                    "thermal": {"state": "nominal"},
                    "storage": {"totalBytes": 256000000000, "availableBytes": 128000000000},
                    "network": {"type": "wifi", "ssid": "HomeWifi"}
                }`),
            }, nil
        },
    }
    registry := &MockRegistry{
        nodes: []*NodeSession{{NodeID: "iphone-1"}},
    }
    router := NewCommandRouter(invoker, registry)
    resp := router.HandleStatus(context.Background(), "iphone-1")
    assert.True(t, resp.OK)
    assert.Contains(t, resp.Message, "85%")       // battery formatted
    assert.Contains(t, resp.Message, "charging")
    assert.Contains(t, resp.Message, "nominal")
    assert.Contains(t, resp.Message, "wifi")
}

func TestHandler_Nodes_Empty(t *testing.T) {
    registry := &MockRegistry{nodes: nil}
    router := NewCommandRouter(nil, registry) // no invoker needed
    resp := router.HandleNodes()
    assert.Contains(t, resp.Message, "No nodes connected")
}

func TestHandler_Nodes_Connected(t *testing.T) {
    registry := &MockRegistry{
        nodes: []*NodeSession{
            {NodeID: "iphone-1", DisplayName: "Ricardo's iPhone", Platform: "ios", Version: "1.2.0"},
            {NodeID: "ipad-2", DisplayName: "Office iPad", Platform: "ios", Version: "1.1.0"},
        },
    }
    router := NewCommandRouter(nil, registry)
    resp := router.HandleNodes()
    assert.Contains(t, resp.Message, "Ricardo's iPhone")
    assert.Contains(t, resp.Message, "Office iPad")
    assert.Contains(t, resp.Message, "2") // 2 devices
}

func TestHandler_InvokeTimeout(t *testing.T) {
    invoker := &MockInvoker{
        InvokeFn: func(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
            return InvokeResult{OK: false}, fmt.Errorf("timeout after 30000ms")
        },
    }
    registry := &MockRegistry{
        nodes: []*NodeSession{{NodeID: "iphone-1"}},
    }
    router := NewCommandRouter(invoker, registry)
    resp := router.HandleSnap(context.Background(), "iphone-1", "back", 80)
    assert.False(t, resp.OK)
    assert.Contains(t, resp.Message, "timed out")
}

func TestHandler_Notify_Success(t *testing.T) {
    invoker := &MockInvoker{
        InvokeFn: func(ctx context.Context, req InvokeRequest) (InvokeResult, error) {
            assert.Equal(t, "system.notify", req.Command)
            return InvokeResult{OK: true}, nil
        },
    }
    registry := &MockRegistry{
        nodes: []*NodeSession{{NodeID: "iphone-1", DisplayName: "Ricardo's iPhone"}},
    }
    router := NewCommandRouter(invoker, registry)
    resp := router.HandleNotify(context.Background(), "iphone-1", "Hello", "Testing notification")
    assert.True(t, resp.OK)
    assert.Contains(t, resp.Message, "sent")
}