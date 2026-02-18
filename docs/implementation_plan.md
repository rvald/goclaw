# GoClaw — Implementation Status

> Slim Go rewrite of the OpenClaw gateway. WebSocket server for iOS node control with Discord bot integration.

**160+ tests passing** across 6 packages.

---

## Module Map

```
cmd/goclaw/
  main.go              ← CLI entrypoint, config, signal handling

internal/
  protocol/            ← Wire format (frames, marshal, connect handshake)
  gateway/             ← WebSocket server, connection lifecycle, auth, device pairing handshake
  node/                ← Node registry, invoke request/response
  discord/             ← Discord bot, slash command routing, pairing commands
  pairing/             ← Device pairing: identity, store, service, tokens
```

---

## Completed Modules

### 1. Protocol — `internal/protocol/`

Wire format for all WebSocket communication. **39 tests.**

- [frames.go](internal/protocol/frames.go) — `ParseFrame()`, `RequestFrame`, `ResponseFrame`, `EventFrame`, `FrameError`
- [marshal.go](internal/protocol/marshal.go) — `MarshalRequest()`, `MarshalResponse()`, `MarshalEvent()` with input validation
- [connect.go](internal/protocol/connect.go) — `ConnectParams`, `ClientInfo`, `ConnectAuth`, `HelloOk`, `ValidateConnect()`, `NodeInvokeRequest`, `NodeInvokeResult`, `DeviceConnectPayload`, `HelloAuthInfo`

---

### 2. Gateway — `internal/gateway/`

WebSocket server, connection state machine, authentication. **24 tests.**

- [auth.go](internal/gateway/auth.go) — `AuthConfig`, `Authenticate()` with `crypto/subtle` constant-time token comparison
- [conn.go](internal/gateway/conn.go) — `Conn` state machine (`connecting` → `authenticated` → `closed`), `WebSocket` / `ConnHandler` interfaces, `SendEvent()` with write mutex, `WithPairing()` for device identity verification
- [server.go](internal/gateway/server.go) — HTTP server with `/ws` upgrade and `/health` endpoint, `Bind` support (`loopback` / `lan`), graceful shutdown, `isLoopback()` for auto-approve detection
- [gateway.go](internal/gateway/gateway.go) — Top-level orchestrator, `ConnHandler` implementation (node registration, invoke result routing, disconnect cleanup), tick keepalive, shutdown broadcast, `PairingSvc()` accessor

**Connection lifecycle:**
1. Server upgrades HTTP → WebSocket
2. `Conn.Run()` sends `connect.challenge` event (with nonce)
3. Client sends `connect` request with auth token + optional `device` payload
4. Server validates protocol version + authenticates
5. If device payload present: verify signature → nonce → device ID → pairing status
6. On success → `OnAuthenticated` registers node in registry
7. Read loop routes subsequent `node.invoke.result` requests to invoker
8. On disconnect → `OnDisconnected` unregisters node, cancels pending invokes

---

### 3. Node — `internal/node/`

Node session registry and invoke request/response lifecycle. **14 tests.**

- [registry.go](internal/node/registry.go) — `NodeSession`, `Registry` (thread-safe dual-index by nodeID and connID), `NewNodeSession()` constructor, duplicate nodeIDs replace old sessions
- [invoker.go](internal/node/invoker.go) — `Invoker`, `InvokeRequest`/`InvokeResult`, `HandleResult()`, `CancelPendingForNode()` for node disconnect cleanup

**Invoke flow:**
1. `Invoke()` generates unique ID, registers result channel, sends event via `sendFunc`
2. Node receives `node.invoke.request` event, processes, sends back `node.invoke.result`
3. `HandleResult()` delivers to waiting channel
4. `Invoke()` returns result (or timeout/context cancellation/node disconnect error)

---

### 4. Discord — `internal/discord/`

Discord bot and command routing. **10 tests.**

- [types.go](internal/discord/types.go) — `Invoker` / `NodeRegistry` / `PairingService` / `PairingStore` interfaces, type aliases
- [bot.go](internal/discord/bot.go) — `Bot` with `Start()` / `Stop()` lifecycle, `RegisterCommands()`, `SetRouter()`, `BotConfig` with `Token` + `GuildID`
- [router.go](internal/discord/router.go) — `CommandRouter` with 9 handlers:
  - `HandleSnap()` — `camera.snap` → base64 decode → image data
  - `HandleLocate()` — `location.get` → lat/lon + Google Maps link
  - `HandleStatus()` — `device.status` → battery/thermal/storage/network
  - `HandleNodes()` — lists connected devices
  - `HandleNotify()` — `system.notify` → push notification
  - `HandleDevices()` — lists paired devices + pending requests
  - `HandleApprove()` — approves a pending pairing request
  - `HandleReject()` — rejects a pending pairing request
  - `HandleRevoke()` — revokes a paired device's token

---

### 5. CLI Entrypoint — `cmd/goclaw/`

- [main.go](cmd/goclaw/main.go) — Config from flags/env, validation (LAN guardrail, port range), signal-based graceful shutdown, startup banner, optional Discord bot, pairing store/service init
- [Makefile](Makefile) — `make test`, `make build`, `make run`

---

### 6. Device Pairing — `internal/pairing/`

Cryptographic device identity and challenge-response pairing. **67 tests.**

- [token.go](internal/pairing/token.go) — 32-byte token generation from `crypto/rand`, constant-time verification
- [identity.go](internal/pairing/identity.go) — Ed25519 signature verification, SHA-256 device ID derivation, pipe-delimited payload construction, UUID v4 nonce generation
- [store.go](internal/pairing/store.go) — Persistent JSON state (pending requests, paired devices, tokens), atomic writes, mutex-protected, 5-minute TTL pruning
- [service.go](internal/pairing/service.go) — Orchestration: `RequestPairing`, `Approve`, `Reject`, `CheckPairingStatus`, `VerifyDeviceToken`, `EnsureDeviceToken`, `RevokeDeviceToken`

Full details: [device_pairing_impl.md](docs/device_pairing_impl.md)

---

## What We Cut from OpenClaw

| Cut | Why |
|-----|-----|
| Config file loading | Flags + env vars only |
| Plugin system | No plugins |
| Agent/chat system | No AI agent |
| Canvas / browser control | Desktop/web automation |
| Gmail / cron / skills | Feature-specific hooks |
| Tailscale exposure | Standard networking |
| Config hot-reload | Restart to apply changes |
| PID lock / SIGUSR1 restart | systemd handles this |
| In-process respawn | Kill + restart is cleaner |

---

**Migration:** Keep `--token` as a fallback for development; device pairing runs alongside token auth, not replacing it.

---

## Verification

```bash
make build                                    # compiles to bin/goclaw
./bin/goclaw --help                           # shows all flags
./bin/goclaw --token secret                   # starts server, Ctrl+C to stop
./bin/goclaw --bind lan                       # refused (no token)
./bin/goclaw --port 99999                     # refused (bad port)
make test                                     # 160+ tests, 6 packages
make run                                      # full stack with Discord
```
