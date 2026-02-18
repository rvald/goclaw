package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectParams_DecodeReal(t *testing.T) {
    // This mirrors what the iOS app actually sends
    raw := `{
        "minProtocol": 3,
        "maxProtocol": 3,
        "client": {
            "id": "iphone-abc123",
            "displayName": "Ricardo's iPhone",
            "version": "1.2.0",
            "platform": "ios",
            "deviceFamily": "iPhone",
            "modelIdentifier": "iPhone16,1",
            "mode": "node"
        },
        "role": "node",
        "caps": ["camera", "location", "screen"],
        "commands": ["camera.snap", "camera.list", "location.get", "device.status", "device.info"],
        "permissions": {"camera.capture": true, "location.access": true},
        "auth": {"token": "my-secret-token"}
    }`
    var params ConnectParams
    err := json.Unmarshal([]byte(raw), &params)
    require.NoError(t, err)
    assert.Equal(t, 3, params.MinProtocol)
    assert.Equal(t, 3, params.MaxProtocol)
    assert.Equal(t, "iphone-abc123", params.Client.ID)
    assert.Equal(t, "Ricardo's iPhone", params.Client.DisplayName)
    assert.Equal(t, "ios", params.Client.Platform)
    assert.Equal(t, "node", params.Client.Mode)
    assert.Equal(t, "node", params.Role)
    assert.Contains(t, params.Caps, "camera")
    assert.Contains(t, params.Commands, "camera.snap")
    assert.Equal(t, true, params.Permissions["camera.capture"])
    assert.Equal(t, "my-secret-token", params.Auth.Token)
}

func TestConnectParams_MinimalNode(t *testing.T) {
    raw := `{
        "minProtocol": 3,
        "maxProtocol": 3,
        "client": {"id": "node-1", "version": "1.0", "platform": "ios", "mode": "node"}
    }`
    var params ConnectParams
    err := json.Unmarshal([]byte(raw), &params)
    require.NoError(t, err)
    assert.Equal(t, "node-1", params.Client.ID)
    assert.Empty(t, params.Caps)
    assert.Empty(t, params.Commands)
    assert.Nil(t, params.Auth)
    assert.Empty(t, params.Role)
}

func TestValidateConnect_ProtocolOK(t *testing.T) {
    params := ConnectParams{MinProtocol: 2, MaxProtocol: 4}
    err := ValidateConnect(params)
    assert.NoError(t, err) // 3 is within [2, 4]
}

func TestValidateConnect_ProtocolTooLow(t *testing.T) {
    params := ConnectParams{MinProtocol: 1, MaxProtocol: 2}
    err := ValidateConnect(params)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "protocol")
}

func TestValidateConnect_ProtocolTooHigh(t *testing.T) {
    params := ConnectParams{MinProtocol: 99, MaxProtocol: 100}
    err := ValidateConnect(params)
    assert.Error(t, err)
}

func TestHelloOk_Encode(t *testing.T) {
    hello := HelloOk{
        Type:     "hello-ok",
        Protocol: 3,
        Server: ServerInfo{
            Version: "0.1.0",
            ConnID:  "conn-abc",
        },
        Features: Features{
            Methods: []string{"connect", "node.invoke", "node.invoke.result", "node.list"},
            Events:  []string{"connect.challenge", "node.invoke.request", "tick"},
        },
        Snapshot: Snapshot{},
        Policy: Policy{
            MaxPayload:      1048576,
            MaxBufferedBytes: 4194304,
            TickIntervalMs:   15000,
        },
    }
    data, err := json.Marshal(hello)
    require.NoError(t, err)
    var raw map[string]any
    require.NoError(t, json.Unmarshal(data, &raw))
    assert.Equal(t, "hello-ok", raw["type"])
    assert.Equal(t, float64(3), raw["protocol"])
    server := raw["server"].(map[string]any)
    assert.Equal(t, "0.1.0", server["version"])
    assert.Equal(t, "conn-abc", server["connId"])
    policy := raw["policy"].(map[string]any)
    assert.Equal(t, float64(15000), policy["tickIntervalMs"])
}

func TestNodeInvokeRequest_Encode(t *testing.T) {
    req := NodeInvokeRequest{
        ID:         "invoke-123",
        NodeID:     "iphone-1",
        Command:    "camera.snap",
        ParamsJSON: `{"facing":"back","maxWidth":1920}`,
    }
    data, err := json.Marshal(req)
    require.NoError(t, err)
    var raw map[string]any
    require.NoError(t, json.Unmarshal(data, &raw))
    assert.Equal(t, "invoke-123", raw["id"])
    assert.Equal(t, "iphone-1", raw["nodeId"])
    assert.Equal(t, "camera.snap", raw["command"])
    assert.Equal(t, `{"facing":"back","maxWidth":1920}`, raw["paramsJSON"])
}

func TestNodeInvokeResult_Decode(t *testing.T) {
    raw := `{
        "id": "invoke-123",
        "nodeId": "iphone-1",
        "ok": true,
        "payloadJSON": "{\"lat\":40.7128,\"lon\":-74.0060}"
    }`
    var result NodeInvokeResult
    err := json.Unmarshal([]byte(raw), &result)
    require.NoError(t, err)
    assert.Equal(t, "invoke-123", result.ID)
    assert.Equal(t, "iphone-1", result.NodeID)
    assert.True(t, result.OK)
    require.NotNil(t, result.PayloadJSON)
    assert.Contains(t, *result.PayloadJSON, "40.7128")
}

func TestNodeInvokeResult_DecodeError(t *testing.T) {
    raw := `{
        "id": "invoke-456",
        "nodeId": "iphone-1",
        "ok": false,
        "error": {"code": "UNAVAILABLE", "message": "camera permission denied"}
    }`
    var result NodeInvokeResult
    err := json.Unmarshal([]byte(raw), &result)
    require.NoError(t, err)
    assert.False(t, result.OK)
    require.NotNil(t, result.Error)
    assert.Equal(t, "UNAVAILABLE", result.Error.Code)
    assert.Equal(t, "camera permission denied", result.Error.Message)
}
