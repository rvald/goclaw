package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalRequest(t *testing.T) {
	t.Run("valid request with params", func(t *testing.T) {
		params := map[string]any{"minProtocol": 3}
		data, err := MarshalRequest("req-1", "connect", params)
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "req", raw["type"])
		assert.Equal(t, "req-1", raw["id"])
		assert.Equal(t, "connect", raw["method"])
		p, ok := raw["params"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(3), p["minProtocol"])
	})

	t.Run("nil params are omitted", func(t *testing.T) {
		data, err := MarshalRequest("req-2", "node.list", nil)
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "req", raw["type"])
		assert.Equal(t, "req-2", raw["id"])
		assert.Equal(t, "node.list", raw["method"])
		assert.Nil(t, raw["params"])
	})

	t.Run("missing id returns error", func(t *testing.T) {
		_, err := MarshalRequest("", "connect", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=id")
	})

	t.Run("missing method returns error", func(t *testing.T) {
		_, err := MarshalRequest("req-1", "", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=method")
	})

	t.Run("unmarshalable params returns error", func(t *testing.T) {
		_, err := MarshalRequest("req-1", "connect", make(chan int))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "INVALID_JSON")
	})

	t.Run("round trip through ParseFrame", func(t *testing.T) {
		params := map[string]any{"minProtocol": float64(3)}
		data, err := MarshalRequest("req-1", "connect", params)
		require.NoError(t, err)

		frame, err := ParseFrame(data)
		require.NoError(t, err)
		req, ok := frame.(*RequestFrame)
		require.True(t, ok, "expected *RequestFrame")
		assert.Equal(t, FrameTypeReq, req.Type)
		assert.Equal(t, "req-1", req.ID)
		assert.Equal(t, "connect", req.Method)
		assert.NotNil(t, req.Params)
	})
}

func TestMarshalResponse(t *testing.T) {
	t.Run("successful response with payload", func(t *testing.T) {
		payload := map[string]any{"protocol": 3}
		data, err := MarshalResponse("req-1", true, payload, nil)
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "res", raw["type"])
		assert.Equal(t, "req-1", raw["id"])
		assert.Equal(t, true, raw["ok"])
		assert.NotNil(t, raw["payload"])
		assert.Nil(t, raw["error"])
	})

	t.Run("failed response with error shape", func(t *testing.T) {
		errShape := &ErrorShape{Code: "UNAUTHORIZED", Message: "bad token"}
		data, err := MarshalResponse("req-1", false, nil, errShape)
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "res", raw["type"])
		assert.Equal(t, "req-1", raw["id"])
		assert.Equal(t, false, raw["ok"])
		assert.Nil(t, raw["payload"])
		errObj, ok := raw["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "UNAUTHORIZED", errObj["code"])
		assert.Equal(t, "bad token", errObj["message"])
	})

	t.Run("failed response with retryable flag", func(t *testing.T) {
		retryable := true
		errShape := &ErrorShape{Code: "RATE_LIMITED", Message: "slow down", Retryable: &retryable}
		data, err := MarshalResponse("req-1", false, nil, errShape)
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		errObj, ok := raw["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "RATE_LIMITED", errObj["code"])
		assert.Equal(t, true, errObj["retryable"])
	})

	t.Run("missing id returns error", func(t *testing.T) {
		_, err := MarshalResponse("", true, nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=id")
	})

	t.Run("round trip through ParseFrame", func(t *testing.T) {
		payload := map[string]any{"protocol": float64(3)}
		data, err := MarshalResponse("req-1", true, payload, nil)
		require.NoError(t, err)

		frame, err := ParseFrame(data)
		require.NoError(t, err)
		res, ok := frame.(*ResponseFrame)
		require.True(t, ok, "expected *ResponseFrame")
		assert.Equal(t, FrameTypeRes, res.Type)
		assert.Equal(t, "req-1", res.ID)
		assert.True(t, res.OK)
		assert.NotNil(t, res.Payload)
	})
}

func TestMarshalEvent(t *testing.T) {
	t.Run("event with payload", func(t *testing.T) {
		payload := map[string]any{"nonce": "abc", "ts": 1700000000}
		data, err := MarshalEvent("connect.challenge", payload)
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "event", raw["type"])
		assert.Equal(t, "connect.challenge", raw["event"])
		assert.Nil(t, raw["id"])
		assert.NotNil(t, raw["payload"])
	})

	t.Run("event without payload", func(t *testing.T) {
		data, err := MarshalEvent("tick", nil)
		require.NoError(t, err)

		var raw map[string]any
		require.NoError(t, json.Unmarshal(data, &raw))
		assert.Equal(t, "event", raw["type"])
		assert.Equal(t, "tick", raw["event"])
		assert.Nil(t, raw["payload"])
	})

	t.Run("missing event returns error", func(t *testing.T) {
		_, err := MarshalEvent("", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=event")
	})

	t.Run("round trip through ParseFrame", func(t *testing.T) {
		payload := map[string]any{"nonce": "abc"}
		data, err := MarshalEvent("connect.challenge", payload)
		require.NoError(t, err)

		frame, err := ParseFrame(data)
		require.NoError(t, err)
		evt, ok := frame.(*EventFrame)
		require.True(t, ok, "expected *EventFrame")
		assert.Equal(t, FrameTypeEvent, evt.Type)
		assert.Equal(t, "connect.challenge", evt.Event)
		assert.NotNil(t, evt.Payload)
	})
}
