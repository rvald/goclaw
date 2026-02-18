package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// FrameError carries structured context for observability.
type FrameError struct {
	Code    string // e.g. "INVALID_JSON", "MISSING_FIELD", "UNKNOWN_TYPE"
	Field   string // which field was the problem, if applicable
	Message string // human-readable detail
}

func (e *FrameError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("frame error [%s]: %s (field=%s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("frame error [%s]: %s", e.Code, e.Message)
}

// Frame types
type FrameType string

const (
    FrameTypeReq   FrameType = "req"
    FrameTypeRes   FrameType = "res"
    FrameTypeEvent FrameType = "event"
)

type RawFrame struct {
    Type FrameType `json:"type"`
}

type RequestFrame struct {
	Type   FrameType 		`json:"type"`
	ID     string    		`json:"id"`
	Method string    		`json:"method"`
	Params json.RawMessage  `json:"params,omitempty"`
}

type ResponseFrame struct {
	Type    FrameType       `json:"type"`
	ID      string          `json:"id"`
    OK      bool            `json:"ok"`
    Payload json.RawMessage `json:"payload,omitempty"`
    Error   *ErrorShape     `json:"error,omitempty"`
}

type EventFrame struct {
 	Type    FrameType       `json:"type"`
    Event   string          `json:"event"`
    Payload json.RawMessage `json:"payload,omitempty"`
    Seq     *int            `json:"seq,omitempty"`
}

type ErrorShape struct {
    Code       string `json:"code"`
    Message    string `json:"message"`
    Retryable  *bool  `json:"retryable,omitempty"`
}

// ParseFrame — discriminated union decode
func ParseFrame(data []byte) (any, error) {

	var raw RawFrame
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &FrameError{Code: "INVALID_JSON", Message: fmt.Sprintf("invalid frame JSON: %v", err)}
	}

	if raw.Type == "" {
		return nil, &FrameError{Code: "MISSING_FIELD", Field: "type", Message: "frame missing required \"type\" field"}
	}

	switch raw.Type {

	case FrameTypeReq:
		var req RequestFrame
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, &FrameError{Code: "INVALID_JSON", Message: fmt.Sprintf("invalid request frame JSON: %v", err)}
		}

		if req.ID == "" {
			return nil, &FrameError{Code: "MISSING_FIELD", Field: "id", Message: "request frame missing required \"id\" field"}
		}
		if req.Method == "" {
			return nil, &FrameError{Code: "MISSING_FIELD", Field: "method", Message: "request frame missing required \"method\" field"}
		}
		if bytes.Equal(req.Params, []byte("null")) {
			req.Params = nil
		}
		return &req, nil

	case FrameTypeRes:
		var res ResponseFrame
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, &FrameError{Code: "INVALID_JSON", Message: fmt.Sprintf("invalid response frame JSON: %v", err)}
		}

		if res.ID == "" {
			return nil, &FrameError{Code: "MISSING_FIELD", Field: "id", Message: "response frame missing required \"id\" field"}
		}

		return &res, nil

	case FrameTypeEvent:
		var evt EventFrame
		if err := json.Unmarshal(data, &evt); err != nil {
			return nil, &FrameError{Code: "INVALID_JSON", Message: fmt.Sprintf("invalid event frame JSON: %v", err)}
		}

		if evt.Event == "" {
			return nil, &FrameError{Code: "MISSING_FIELD", Field: "event", Message: "event frame missing required \"event\" field"}
		}
		return &evt, nil

	default:
		return nil, &FrameError{Code: "UNKNOWN_TYPE", Message: fmt.Sprintf("unknown frame type: %q", raw.Type)}
	}
}