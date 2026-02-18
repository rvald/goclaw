package protocol

import "fmt"

// ServerProtocol is the protocol version this server speaks.
const ServerProtocol = 3

// ---------- connect request params ----------

type ConnectParams struct {
	MinProtocol int              `json:"minProtocol"`
	MaxProtocol int              `json:"maxProtocol"`
	Client      ClientInfo       `json:"client"`
	Role        string           `json:"role,omitempty"`
	Caps        []string         `json:"caps,omitempty"`
	Commands    []string         `json:"commands,omitempty"`
	Permissions map[string]bool  `json:"permissions,omitempty"`
	Auth        *ConnectAuth     `json:"auth,omitempty"`
	Device      *DeviceConnectPayload `json:"device,omitempty"`
}

// DeviceConnectPayload carries cryptographic device identity in the connect request.
type DeviceConnectPayload struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"` // base64url-encoded raw 32-byte Ed25519 public key
	Signature string `json:"signature"` // base64url-encoded Ed25519 signature
	SignedAt  int64  `json:"signedAt"`  // milliseconds since epoch
	Nonce     string `json:"nonce"`     // server-issued challenge nonce
}

// HelloAuthInfo carries auth tokens in the hello-ok response.
type HelloAuthInfo struct {
	DeviceToken string `json:"deviceToken,omitempty"`
}

type ClientInfo struct {
	ID              string `json:"id"`
	DisplayName     string `json:"displayName,omitempty"`
	Version         string `json:"version"`
	Platform        string `json:"platform"`
	DeviceFamily    string `json:"deviceFamily,omitempty"`
	ModelIdentifier string `json:"modelIdentifier,omitempty"`
	Mode            string `json:"mode"`
}

type ConnectAuth struct {
	Token string `json:"token"`
}

// ValidateConnect checks that the server's protocol version falls within
// the client's advertised [MinProtocol, MaxProtocol] range.
func ValidateConnect(params ConnectParams) error {
	if ServerProtocol < params.MinProtocol || ServerProtocol > params.MaxProtocol {
		return &FrameError{
			Code:    "PROTOCOL_MISMATCH",
			Message: fmt.Sprintf("server protocol %d not in client range [%d, %d]", ServerProtocol, params.MinProtocol, params.MaxProtocol),
		}
	}
	return nil
}

// ---------- hello-ok response ----------

type HelloOk struct {
	Type     string     `json:"type"`
	Protocol int        `json:"protocol"`
	Server   ServerInfo `json:"server"`
	Features Features   `json:"features"`
	Snapshot Snapshot   `json:"snapshot"`
	Policy   Policy     `json:"policy"`
}

type ServerInfo struct {
	Version string `json:"version"`
	ConnID  string `json:"connId"`
}

type Features struct {
	Methods []string `json:"methods"`
	Events  []string `json:"events"`
}

type Snapshot struct{}

type Policy struct {
	MaxPayload       int `json:"maxPayload"`
	MaxBufferedBytes int `json:"maxBufferedBytes"`
	TickIntervalMs   int `json:"tickIntervalMs"`
}

// ---------- node invoke ----------

type NodeInvokeRequest struct {
	ID         string `json:"id"`
	NodeID     string `json:"nodeId"`
	Command    string `json:"command"`
	ParamsJSON string `json:"paramsJSON,omitempty"`
}

type NodeInvokeResult struct {
	ID          string      `json:"id"`
	NodeID      string      `json:"nodeId"`
	OK          bool        `json:"ok"`
	PayloadJSON *string     `json:"payloadJSON,omitempty"`
	Error       *ErrorShape `json:"error,omitempty"`
}
