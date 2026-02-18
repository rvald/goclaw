package protocol

import (
  "testing"
  "github.com/stretchr/testify/assert"
  "github.com/stretchr/testify/require"
)

func TestParseFrame(t *testing.T) {
	t.Run("unmarshal valid request frame", func(t *testing.T) {
		input := []byte(`{"type":"req","id":"abc-123","method":"connect","params":{"minProtocol":3}}`)
		frame, err := ParseFrame(input)
		require.NoError(t, err)
		req, ok := frame.(*RequestFrame)
		require.True(t, ok, "expected *RequestFrame")
		assert.Equal(t, FrameTypeReq, req.Type)
		assert.Equal(t, "abc-123", req.ID)
		assert.Equal(t, "connect", req.Method)
		assert.NotNil(t, req.Params) // params should be captured as raw JSON
	})

	t.Run("unmarshal valid response frame", func(t *testing.T) {
		input := []byte(`{"type":"res","id":"abc-123","ok":true,"payload":{"protocol":3}}`)
		frame, err := ParseFrame(input)
		require.NoError(t, err)
		res, ok := frame.(*ResponseFrame)
		require.True(t, ok, "expected *ResponseFrame")
		assert.Equal(t, FrameTypeRes, res.Type)
		assert.Equal(t, "abc-123", res.ID)
		assert.True(t, res.OK)
		assert.NotNil(t, res.Payload)
		assert.Nil(t, res.Error)
	})

	t.Run("unmarshal valid event frame", func(t *testing.T) {
		input := []byte(`{"type":"event","event":"connect.challenge","payload":{"nonce":"xyz","ts":1700000000}}`)
		frame, err := ParseFrame(input)
		require.NoError(t, err)
		evt, ok := frame.(*EventFrame)
		require.True(t, ok, "expected *EventFrame")
		assert.Equal(t, FrameTypeEvent, evt.Type)
		assert.Equal(t, "connect.challenge", evt.Event)
		assert.NotNil(t, evt.Payload)
		assert.Nil(t, evt.Seq) // seq is optional, not present here
	})

	t.Run("should return error for bad json", func(t *testing.T) {
		input := []byte(`{broken json!!!`)
		frame, err := ParseFrame(input)
		assert.Error(t, err)
		assert.Nil(t, frame)
		assert.Contains(t, err.Error(), "INVALID_JSON")
	})

	t.Run("should return error for empty bytes", func(t *testing.T) {
		frame, err := ParseFrame([]byte{})
		assert.Error(t, err)
		assert.Nil(t, frame)
		assert.Contains(t, err.Error(), "INVALID_JSON")
	})

	t.Run("should return error for unknown types", func(t *testing.T) {
		input := []byte(`{"type":"wat","id":"abc"}`)
		frame, err := ParseFrame(input)
		assert.Error(t, err)
		assert.Nil(t, frame)
		assert.Contains(t, err.Error(), "unknown frame type")
	})

	t.Run("should return error for json without type key", func(t *testing.T) {
		input := []byte(`{"id":"abc","method":"connect"}`)
		frame, err := ParseFrame(input)
		assert.Error(t, err)
		assert.Nil(t, frame)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=type")
	})

	t.Run("should return error for json without id key", func(t *testing.T) {
		input := []byte(`{"type":"req","method":"connect"}`)
		frame, err := ParseFrame(input)
		assert.Error(t, err)
		assert.Nil(t, frame)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=id")
	})

	t.Run("should return error for event json without event key", func(t *testing.T) {
		input := []byte(`{"type":"event","payload":{"foo":"bar"}}`)
		frame, err := ParseFrame(input)
		assert.Error(t, err)
		assert.Nil(t, frame)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=event")
	})

	t.Run("should return error for request json missing method key", func(t *testing.T) {
		input := []byte(`{"type":"req","id":"abc-123"}`)
		frame, err := ParseFrame(input)
		assert.Error(t, err)
		assert.Nil(t, frame)
		assert.Contains(t, err.Error(), "MISSING_FIELD")
		assert.Contains(t, err.Error(), "field=method")
	})

	t.Run("should handle null params", func(t *testing.T) {
		input := []byte(`{"type":"req","id":"abc","method":"node.list","params":null}`)
		frame, err := ParseFrame(input)
		require.NoError(t, err)
		req := frame.(*RequestFrame)
		assert.Equal(t, "node.list", req.Method)
		assert.Nil(t, req.Params) // null should become nil, not empty bytes
	})

	t.Run("should ignore extra params", func(t *testing.T) {
		input := []byte(`{"type":"req","id":"abc","method":"connect","params":{},"futureField":"hello","anotherOne":42}`)
		frame, err := ParseFrame(input)
		require.NoError(t, err)
		req := frame.(*RequestFrame)
		assert.Equal(t, "abc", req.ID)
		assert.Equal(t, "connect", req.Method)
	})

	t.Run("should parse event seq param", func(t *testing.T) {
		input := []byte(`{"type":"event","event":"tick","payload":{"ts":123},"seq":42}`)
		frame, err := ParseFrame(input)
		require.NoError(t, err)
		evt := frame.(*EventFrame)
		assert.Equal(t, "tick", evt.Event)
		require.NotNil(t, evt.Seq)
		assert.Equal(t, 42, *evt.Seq)
	})

	t.Run("should return error for a failed response", func(t *testing.T) {
		input := []byte(`{"type":"res","id":"abc","ok":false,"error":{"code":"UNAUTHORIZED","message":"token mismatch"}}`)
		frame, err := ParseFrame(input)
		require.NoError(t, err)
		res := frame.(*ResponseFrame)
		assert.False(t, res.OK)
		assert.Nil(t, res.Payload)
		require.NotNil(t, res.Error)
		assert.Equal(t, "UNAUTHORIZED", res.Error.Code)
		assert.Equal(t, "token mismatch", res.Error.Message)
	})
}
