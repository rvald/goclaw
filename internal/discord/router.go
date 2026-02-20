package discord

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// CommandResponse is the result returned by command handlers.
type CommandResponse struct {
	OK        bool
	Message   string
	ImageData []byte // decoded image bytes, if applicable
}

// CommandRouter dispatches slash commands to the appropriate handler.
type CommandRouter struct {
	invoker  Invoker
	registry NodeRegistry
	pairing  PairingService // optional — nil when pairing is not enabled
	store    PairingStore   // optional — nil when pairing is not enabled
}

// NewCommandRouter creates a router backed by the given invoker and registry.
func NewCommandRouter(invoker Invoker, registry NodeRegistry) *CommandRouter {
	return &CommandRouter{invoker: invoker, registry: registry}
}

// WithPairing attaches pairing service and store to the router.
func (r *CommandRouter) WithPairing(svc PairingService, store PairingStore) {
	r.pairing = svc
	r.store = store
}

// Commands returns the slash command definitions for Discord registration.
func (r *CommandRouter) Commands() []SlashCommand {
	cmds := []SlashCommand{
		{
			Name:        "snap",
			Description: "Take a camera snapshot from a connected device",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "node", Description: "Node ID (optional)"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "facing", Description: "Camera facing: front or back",
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Front", Value: "front"},
						{Name: "Back", Value: "back"},
					},
				},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "quality", Description: "JPEG quality 1-100"},
			},
		},
		{
			Name:        "locate",
			Description: "Get the current location of a device",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "node", Description: "Node ID (optional)"},
			},
		},
		{
			Name:        "status",
			Description: "Get device status (battery, thermal, storage, network)",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "node", Description: "Node ID (optional)"},
			},
		},
		{
			Name:        "nodes",
			Description: "List all connected nodes",
		},
		{
			Name:        "notify",
			Description: "Send a push notification to a device",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "title", Description: "Notification title", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "body", Description: "Notification body", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "node", Description: "Node ID (optional)"},
			},
		},
	}

	// Add pairing commands only when pairing is enabled
	if r.pairing != nil {
		cmds = append(cmds,
			SlashCommand{
				Name:        "devices",
				Description: "List all paired and pending devices",
			},
			SlashCommand{
				Name:        "approve",
				Description: "Approve a pending device pairing request",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: "request", Description: "Request ID to approve", Required: true},
				},
			},
			SlashCommand{
				Name:        "reject",
				Description: "Reject a pending device pairing request",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: "request", Description: "Request ID to reject", Required: true},
				},
			},
			SlashCommand{
				Name:        "revoke",
				Description: "Revoke a paired device's access token",
				Options: []*discordgo.ApplicationCommandOption{
					{Type: discordgo.ApplicationCommandOptionString, Name: "device", Description: "Device ID to revoke", Required: true},
					{Type: discordgo.ApplicationCommandOptionString, Name: "role", Description: "Role to revoke (default: node)"},
				},
			},
		)
	}

	return cmds
}

// resolveNode picks a node by ID, or the first available if nodeID is empty.
func (r *CommandRouter) resolveNode(nodeID string) (*NodeSession, error) {
	if nodeID != "" {
		n, ok := r.registry.Get(nodeID)
		if !ok {
			return nil, fmt.Errorf("node %q not connected", nodeID)
		}
		return n, nil
	}
	nodes := r.registry.List()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes available")
	}
	return nodes[0], nil
}

// HandleSnap requests a camera snapshot from the target node.
func (r *CommandRouter) HandleSnap(ctx context.Context, nodeID, facing string, quality int) CommandResponse {
	node, err := r.resolveNode(nodeID)
	if err != nil {
		return CommandResponse{OK: false, Message: "📱 No iOS device connected"}
	}

	result, err := r.invoker.Invoke(ctx, InvokeRequest{
		NodeID:    node.NodeID,
		Command:   "camera.snap",
		TimeoutMs: 30000,
	})
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return CommandResponse{OK: false, Message: "⏱️ Camera request timed out"}
		}
		return CommandResponse{OK: false, Message: fmt.Sprintf("❌ Error: %s", err.Error())}
	}

	if !result.OK {
		return CommandResponse{OK: false, Message: r.invokeErrorMessage(result, "❌ Camera snap failed")}
	}
	if result.PayloadJSON == nil {
		return CommandResponse{OK: false, Message: "❌ Camera snap missing payload"}
	}

	var payload struct {
		ImageBase64 string `json:"imageBase64"`
		Base64      string `json:"base64"`
		Format      string `json:"format"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
	}
	if err := json.Unmarshal([]byte(*result.PayloadJSON), &payload); err != nil {
		return CommandResponse{OK: false, Message: fmt.Sprintf("❌ Camera snap decode failed: %v", err)}
	}
	raw := payload.ImageBase64
	if raw == "" {
		raw = payload.Base64
	}
	if raw == "" {
		return CommandResponse{OK: false, Message: "❌ Camera snap payload missing image data"}
	}

	imageData, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return CommandResponse{OK: false, Message: fmt.Sprintf("❌ Camera snap decode failed: %v", err)}
	}

	return CommandResponse{
		OK:        true,
		Message:   fmt.Sprintf("📸 Photo from %s (%dx%d %s)", node.DisplayName, payload.Width, payload.Height, payload.Format),
		ImageData: imageData,
	}
}

// HandleLocate requests the device location.
func (r *CommandRouter) HandleLocate(ctx context.Context, nodeID string) CommandResponse {
	node, err := r.resolveNode(nodeID)
	if err != nil {
		return CommandResponse{OK: false, Message: "📱 No iOS device connected"}
	}

	result, err := r.invoker.Invoke(ctx, InvokeRequest{
		NodeID:    node.NodeID,
		Command:   "location.get",
		TimeoutMs: 15000,
	})
	if err != nil {
		return CommandResponse{OK: false, Message: fmt.Sprintf("❌ Error: %s", err.Error())}
	}
	if !result.OK {
		return CommandResponse{OK: false, Message: r.invokeErrorMessage(result, "❌ Location request failed")}
	}
	if result.PayloadJSON == nil {
		return CommandResponse{OK: false, Message: "❌ Location missing payload"}
	}

	var loc struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Altitude  float64 `json:"altitude"`
		Accuracy  float64 `json:"accuracy"`
	}
	if err := json.Unmarshal([]byte(*result.PayloadJSON), &loc); err != nil {
		return CommandResponse{OK: false, Message: fmt.Sprintf("❌ Location decode failed: %v", err)}
	}

	mapURL := fmt.Sprintf("https://google.com/maps?q=%f,%f", loc.Latitude, loc.Longitude)
	msg := fmt.Sprintf("📍 Location: %f, %f (±%.0fm, alt %.1fm)\n%s",
		loc.Latitude, loc.Longitude, loc.Accuracy, loc.Altitude, mapURL)

	return CommandResponse{OK: true, Message: msg}
}

// HandleStatus requests device status (battery, thermal, storage, network).
func (r *CommandRouter) HandleStatus(ctx context.Context, nodeID string) CommandResponse {
	node, err := r.resolveNode(nodeID)
	if err != nil {
		return CommandResponse{OK: false, Message: "📱 No iOS device connected"}
	}

	result, err := r.invoker.Invoke(ctx, InvokeRequest{
		NodeID:    node.NodeID,
		Command:   "device.status",
		TimeoutMs: 10000,
	})
	if err != nil {
		return CommandResponse{OK: false, Message: fmt.Sprintf("❌ Error: %s", err.Error())}
	}
	if !result.OK {
		return CommandResponse{OK: false, Message: r.invokeErrorMessage(result, "❌ Device status failed")}
	}
	if result.PayloadJSON == nil {
		return CommandResponse{OK: false, Message: "❌ Device status missing payload"}
	}

	var status struct {
		Battery struct {
			Level float64 `json:"level"`
			State string  `json:"state"`
		} `json:"battery"`
		Thermal struct {
			State string `json:"state"`
		} `json:"thermal"`
		Storage struct {
			TotalBytes     int64 `json:"totalBytes"`
			AvailableBytes int64 `json:"availableBytes"`
		} `json:"storage"`
		Network struct {
			Type string `json:"type"`
		SSID string `json:"ssid"`
		} `json:"network"`
	}
	if err := json.Unmarshal([]byte(*result.PayloadJSON), &status); err != nil {
		return CommandResponse{OK: false, Message: fmt.Sprintf("❌ Status decode failed: %v", err)}
	}

	batteryPct := int(status.Battery.Level * 100)
	msg := fmt.Sprintf("🔋 Battery: %d%% (%s)\n🌡️ Thermal: %s\n📶 Network: %s\n💾 Storage: %.0f GB / %.0f GB",
		batteryPct,
		status.Battery.State,
		status.Thermal.State,
		status.Network.Type,
		float64(status.Storage.AvailableBytes)/1e9,
		float64(status.Storage.TotalBytes)/1e9,
	)

	return CommandResponse{OK: true, Message: msg}
}

// HandleNodes lists all connected nodes.
func (r *CommandRouter) HandleNodes() CommandResponse {
	nodes := r.registry.List()
	if len(nodes) == 0 {
		return CommandResponse{Message: "No nodes connected"}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📱 %d device(s) connected:\n", len(nodes)))
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("• %s (%s %s) — %s\n", n.DisplayName, n.Platform, n.Version, n.NodeID))
	}
	return CommandResponse{OK: true, Message: sb.String()}
}

// HandleNotify sends a push notification to the target node.
func (r *CommandRouter) HandleNotify(ctx context.Context, nodeID, title, body string) CommandResponse {
	nd, err := r.resolveNode(nodeID)
	if err != nil {
		return CommandResponse{Message: fmt.Sprintf("❌ %s", err)}
	}

	result, err := r.invoker.Invoke(ctx, InvokeRequest{
		NodeID:    nd.NodeID,
		Command:   "system.notify",
		TimeoutMs: 10000,
	})
	if err != nil {
		return CommandResponse{Message: fmt.Sprintf("❌ invoke error: %v", err)}
	}
	if !result.OK {
		return CommandResponse{Message: r.invokeErrorMessage(result, "❌ Notification failed")}
	}

	return CommandResponse{OK: true, Message: fmt.Sprintf("✅ Notification sent to **%s**", nd.DisplayName)}
}

func (r *CommandRouter) invokeErrorMessage(result InvokeResult, fallback string) string {
	if result.Error != nil && result.Error.Message != "" {
		return fmt.Sprintf("❌ %s", result.Error.Message)
	}
	return fallback
}

// --- Device Pairing Handlers ---

// HandleDevices lists all paired and pending devices.
func (r *CommandRouter) HandleDevices() CommandResponse {
	if r.store == nil {
		return CommandResponse{Message: "❌ Device pairing is not enabled"}
	}

	paired := r.store.ListPaired()
	pending := r.store.ListPending()

	if len(paired) == 0 && len(pending) == 0 {
		return CommandResponse{OK: true, Message: "No devices found."}
	}

	var sb strings.Builder

	if len(paired) > 0 {
		sb.WriteString(fmt.Sprintf("**Paired Devices** (%d)\n", len(paired)))
		for _, d := range paired {
			name := d.DisplayName
			if name == "" {
				name = d.DeviceID[:12] + "…"
			}
			sb.WriteString(fmt.Sprintf("• `%s` — %s (%s)\n", d.DeviceID[:12], name, d.Platform))
		}
	}

	if len(pending) > 0 {
		if len(paired) > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("**Pending Requests** (%d)\n", len(pending)))
		for _, p := range pending {
			name := p.DisplayName
			if name == "" {
				name = p.DeviceID[:12] + "…"
			}
			sb.WriteString(fmt.Sprintf("• `%s` — %s (request: `%s`)\n", p.DeviceID[:12], name, p.RequestID[:8]))
		}
	}

	return CommandResponse{OK: true, Message: sb.String()}
}

// HandleApprove approves a pending device pairing request.
func (r *CommandRouter) HandleApprove(requestID string) CommandResponse {
	if r.pairing == nil {
		return CommandResponse{Message: "❌ Device pairing is not enabled"}
	}
	if requestID == "" {
		return CommandResponse{Message: "❌ Request ID is required"}
	}

	device, err := r.pairing.Approve(requestID)
	if err != nil {
		return CommandResponse{Message: fmt.Sprintf("❌ Approve failed: %v", err)}
	}
	if device == nil {
		return CommandResponse{Message: fmt.Sprintf("❌ No pending request found for `%s`", requestID)}
	}

	name := device.DisplayName
	if name == "" {
		name = device.DeviceID[:12] + "…"
	}
	return CommandResponse{OK: true, Message: fmt.Sprintf("✅ Approved device **%s** (`%s`)", name, device.DeviceID[:12])}
}

// HandleReject rejects a pending device pairing request.
func (r *CommandRouter) HandleReject(requestID string) CommandResponse {
	if r.pairing == nil {
		return CommandResponse{Message: "❌ Device pairing is not enabled"}
	}
	if requestID == "" {
		return CommandResponse{Message: "❌ Request ID is required"}
	}

	rejected, err := r.pairing.Reject(requestID)
	if err != nil {
		return CommandResponse{Message: fmt.Sprintf("❌ Reject failed: %v", err)}
	}
	if rejected == nil {
		return CommandResponse{Message: fmt.Sprintf("❌ No pending request found for `%s`", requestID)}
	}

	name := rejected.DisplayName
	if name == "" {
		name = rejected.DeviceID[:12] + "…"
	}
	return CommandResponse{OK: true, Message: fmt.Sprintf("🚫 Rejected device **%s** (`%s`)", name, rejected.DeviceID[:12])}
}

// HandleRevoke revokes a paired device's access token.
func (r *CommandRouter) HandleRevoke(deviceID, role string) CommandResponse {
	if r.pairing == nil {
		return CommandResponse{Message: "❌ Device pairing is not enabled"}
	}
	if deviceID == "" {
		return CommandResponse{Message: "❌ Device ID is required"}
	}
	if role == "" {
		role = "node"
	}

	tok := r.pairing.RevokeDeviceToken(deviceID, role)
	if tok == nil {
		return CommandResponse{Message: fmt.Sprintf("❌ No token found for device `%s` role `%s`", deviceID[:min(12, len(deviceID))], role)}
	}

	return CommandResponse{OK: true, Message: fmt.Sprintf("🔒 Revoked token for device `%s` role `%s`", deviceID[:min(12, len(deviceID))], role)}
}
