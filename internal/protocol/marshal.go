package protocol

import (
	"encoding/json"
	"fmt"
)

// MarshalRequest builds a JSON-encoded request frame.
func MarshalRequest(id, method string, params any) ([]byte, error) {
	if id == "" {
		return nil, &FrameError{Code: "MISSING_FIELD", Field: "id", Message: "request frame missing required \"id\" field"}
	}
	if method == "" {
		return nil, &FrameError{Code: "MISSING_FIELD", Field: "method", Message: "request frame missing required \"method\" field"}
	}

	frame := RequestFrame{
		Type:   FrameTypeReq,
		ID:     id,
		Method: method,
	}

	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, &FrameError{Code: "INVALID_JSON", Message: fmt.Sprintf("failed to marshal request params: %v", err)}
		}
		frame.Params = raw
	}

	return json.Marshal(frame)
}

// MarshalResponse builds a JSON-encoded response frame.
func MarshalResponse(id string, ok bool, payload any, errShape *ErrorShape) ([]byte, error) {
	if id == "" {
		return nil, &FrameError{Code: "MISSING_FIELD", Field: "id", Message: "response frame missing required \"id\" field"}
	}

	frame := ResponseFrame{
		Type:  FrameTypeRes,
		ID:    id,
		OK:    ok,
		Error: errShape,
	}

	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, &FrameError{Code: "INVALID_JSON", Message: fmt.Sprintf("failed to marshal response payload: %v", err)}
		}
		frame.Payload = raw
	}

	return json.Marshal(frame)
}

// MarshalEvent builds a JSON-encoded event frame.
func MarshalEvent(event string, payload any) ([]byte, error) {
	if event == "" {
		return nil, &FrameError{Code: "MISSING_FIELD", Field: "event", Message: "event frame missing required \"event\" field"}
	}

	frame := EventFrame{
		Type:  FrameTypeEvent,
		Event: event,
	}

	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, &FrameError{Code: "INVALID_JSON", Message: fmt.Sprintf("failed to marshal event payload: %v", err)}
		}
		frame.Payload = raw
	}

	return json.Marshal(frame)
}
