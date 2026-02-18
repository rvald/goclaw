// test-pairing.go — Manual test for the device pairing handshake.
//
// Usage:
//   1. Start the gateway:   go run ./cmd/goclaw/ --token secret
//   2. Run this script:     go run ./scripts/test-pairing.go
//
// Flags:
//   -addr     gateway address          (default "ws://127.0.0.1:18789/ws")
//   -token    gateway auth token       (default "secret")
//   -save     save keypair to file     (default "" = ephemeral)
//   -load     load keypair from file   (default "" = generate new)
//
// What it does:
//   1. Generates (or loads) an Ed25519 keypair
//   2. Connects to the gateway via WebSocket
//   3. Reads the connect.challenge event (extracts nonce)
//   4. Signs the auth payload with the private key
//   5. Sends the connect request with the device identity
//   6. Prints the response (hello-ok with deviceToken, or error)

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// ── Helpers ──────────────────────────────────────────────────────────────

var b64 = base64.RawURLEncoding

func deriveDeviceID(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:])
}

func buildPayload(deviceID, clientID, mode, role string, scopes []string, signedAt int64, token, nonce string) string {
	return fmt.Sprintf("v2|%s|%s|%s|%s|%s|%d|%s|%s",
		deviceID, clientID, mode, role,
		strings.Join(scopes, ","), signedAt, token, nonce)
}

func saveKeypair(path string, pub ed25519.PublicKey, priv ed25519.PrivateKey) error {
	data, _ := json.MarshalIndent(map[string]string{
		"publicKey":  b64.EncodeToString(pub),
		"privateKey": b64.EncodeToString(priv),
	}, "", "  ")
	return os.WriteFile(path, data, 0600)
}

func loadKeypair(path string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var kp struct {
		PublicKey  string `json:"publicKey"`
		PrivateKey string `json:"privateKey"`
	}
	if err := json.Unmarshal(data, &kp); err != nil {
		return nil, nil, err
	}
	pub, _ := b64.DecodeString(kp.PublicKey)
	priv, _ := b64.DecodeString(kp.PrivateKey)
	return ed25519.PublicKey(pub), ed25519.PrivateKey(priv), nil
}

// ── Main ─────────────────────────────────────────────────────────────────

func main() {
	addr := flag.String("addr", "ws://127.0.0.1:18789/ws", "gateway WebSocket address")
	token := flag.String("token", "secret", "gateway auth token")
	savePath := flag.String("save", "", "save keypair to file (reuse across runs)")
	loadPath := flag.String("load", "", "load keypair from file")
	flag.Parse()

	// ── Step 1: Keypair ──────────────────────────────────────────────────

	var pub ed25519.PublicKey
	var priv ed25519.PrivateKey

	if *loadPath != "" {
		var err error
		pub, priv, err = loadKeypair(*loadPath)
		if err != nil {
			log.Fatalf("load keypair: %v", err)
		}
		fmt.Printf("🔑 Loaded keypair from %s\n", *loadPath)
	} else {
		var err error
		pub, priv, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("🔑 Generated new Ed25519 keypair")
	}

	if *savePath != "" {
		if err := saveKeypair(*savePath, pub, priv); err != nil {
			log.Fatalf("save keypair: %v", err)
		}
		fmt.Printf("💾 Saved keypair to %s\n", *savePath)
	}

	deviceID := deriveDeviceID(pub)
	pubB64 := b64.EncodeToString(pub)

	fmt.Printf("   Device ID:  %s…\n", deviceID[:16])
	fmt.Printf("   Public Key: %s…\n", pubB64[:20])
	fmt.Println()

	// ── Step 2: Connect ──────────────────────────────────────────────────

	fmt.Printf("📡 Connecting to %s\n", *addr)
	conn, _, err := websocket.DefaultDialer.Dial(*addr, nil)
	if err != nil {
		log.Fatalf("   ❌ dial failed: %v", err)
	}
	defer conn.Close()
	fmt.Println("   ✅ WebSocket connected")
	fmt.Println()

	// ── Step 3: Read challenge ───────────────────────────────────────────

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("   ❌ read challenge: %v", err)
	}

	// Parse the event frame
	var frame map[string]json.RawMessage
	json.Unmarshal(msg, &frame)

	var event string
	json.Unmarshal(frame["event"], &event)

	if event != "connect.challenge" {
		log.Fatalf("   ❌ expected connect.challenge, got %q", event)
	}

	var challenge struct {
		Nonce string `json:"nonce"`
		Ts    int64  `json:"ts"`
	}
	json.Unmarshal(frame["payload"], &challenge)

	fmt.Println("← connect.challenge")
	fmt.Printf("   Nonce: %s\n", challenge.Nonce)
	fmt.Printf("   Time:  %s\n", time.Unix(challenge.Ts, 0).Format(time.RFC3339))
	fmt.Println()

	// ── Step 4: Sign payload ─────────────────────────────────────────────

	signedAt := time.Now().UnixMilli()
	payload := buildPayload(deviceID, "test-device", "node", "node", nil, signedAt, *token, challenge.Nonce)
	sig := ed25519.Sign(priv, []byte(payload))
	sigB64 := b64.EncodeToString(sig)

	fmt.Println("✍️  Signing payload")
	fmt.Printf("   Payload: %s\n", payload)
	fmt.Printf("   Sig:     %s…\n", sigB64[:30])
	fmt.Println()

	// ── Step 5: Send connect request ─────────────────────────────────────

	connectReq := map[string]any{
		"type":   "req",
		"id":     "pair-1",
		"method": "connect",
		"params": map[string]any{
			"minProtocol": 3,
			"maxProtocol": 3,
			"client": map[string]string{
				"id":          "test-device",
				"displayName": "Test Device",
				"version":     "1.0.0",
				"platform":    "linux",
				"mode":        "node",
			},
			"auth": map[string]string{
				"token": *token,
			},
			"device": map[string]any{
				"id":        deviceID,
				"publicKey": pubB64,
				"signature": sigB64,
				"signedAt":  signedAt,
				"nonce":     challenge.Nonce,
			},
		},
	}

	reqData, _ := json.Marshal(connectReq)
	fmt.Printf("→ connect (device=%s…)\n", deviceID[:12])
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, reqData); err != nil {
		log.Fatalf("   ❌ send connect: %v", err)
	}
	fmt.Println()

	// ── Step 6: Read response ────────────────────────────────────────────

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, respMsg, err := conn.ReadMessage()
	if err != nil {
		log.Fatalf("   ❌ read response: %v", err)
	}

	var resp struct {
		Type   string          `json:"type"`
		ID     string          `json:"id"`
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(respMsg, &resp)

	if resp.OK {
		fmt.Println("← hello-ok")
		fmt.Println("   ✅ Device paired successfully!")

		if resp.Result != nil {
			var result struct {
				Auth struct {
					DeviceToken string `json:"deviceToken"`
				} `json:"auth"`
			}
			json.Unmarshal(resp.Result, &result)
			if result.Auth.DeviceToken != "" {
				fmt.Printf("   🎫 Device Token: %s…\n", result.Auth.DeviceToken[:20])
			}
		}
	} else {
		fmt.Println("← error")
		if resp.Error != nil {
			fmt.Printf("   ❌ Code:    %s\n", resp.Error.Code)
			fmt.Printf("   ❌ Message: %s\n", resp.Error.Message)

			if resp.Error.Code == "NOT_PAIRED" {
				fmt.Println()
				fmt.Println("   ℹ️  This device needs operator approval.")
				fmt.Println("   ℹ️  Use /approve <requestId> in Discord,")
				fmt.Println("   ℹ️  then re-run this script with:")
				fmt.Printf("   ℹ️  go run scripts/test-pairing.go -load <keyfile>\n")
			}
		}
	}

	fmt.Println()
	fmt.Println("📁 State file: ~/.local/state/goclaw/pairing/state.json")
}
