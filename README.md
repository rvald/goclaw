# GoClaw

**GoClaw** is a slim, high-performance Go rewrite of the OpenClaw gateway. It serves as a WebSocket command-and-control server for iOS devices ("nodes"), integrating with Discord for remote management.

> ⚡ **Status**: Alpha (Active Development)
> 🧪 **Tests**: 160+ passing (94% coverage in core modules)

---

## 🚀 Features

- **WebSocket Gateway**: Robust connection handling with protocol versioning and keepalives.
- **Secure Device Pairing**:
    - Ed25519 cryptographic identity (no shared secrets).
    - Pairing flow akin to Signal/WhatsApp (scan → sign → connect).
    - Auto-approval for local (loopback) connections.
- **Discord Integration**:
    - Slash commands for device management (`/devices`, `/approve`, `/revoke`).
    - Remote control commands (`/snap`, `/locate`, `/status`, `/notify`).
- **Node Registry**: In-memory session management for connected devices.
- **Zero-Dependency**: Single binary, no external database (uses local JSON state).
- **Observability**:
    - Prometheus Metrics (`/metrics`) for real-time monitoring.
    - Structured Logging (`slog`) with JSON output and automatic rotation.
- **Reliability & Security**:
    - Robust WebSocket handling with timeouts, heartbeats, and read limits.
    - IP-based rate limiting to prevent abuse.
- **Deployment Ready**:
    - Multi-stage Docker build.
    - Systemd service configuration.
- **Graceful Shutdown**: Handles OS signals to cleanly close connections and save state.

---

## 🛠️ Architecture

GoClaw is built with a modular "hexagonal-ish" architecture:

```
internal/
├── protocol/       # Wire format (JSON frames), marshaling
├── gateway/        # WebSocket server, auth, connection lifecycle
├── node/           # Node session registry, invoke request/response
├── pairing/        # Device identity, persistent store, pairing logic
└── discord/        # Discord bot, slash command routing
```

### Key Flows

1.  **Connection**: iOS Node → WebSocket (`/ws`) → Handshake (Protocol + Auth) → Registered.
2.  **Pairing**: Unpaired Device → Connects with `DevicePayload` → Server issues `challenge` → Device signs → Server verifies & stores pending request.
3.  **Command**: Discord User `/snap` → Bot → Gateway Invoker → Node (`camera.snap`) → Result → Discord.

---

## 📦 Installation

### Prerequisites
- Go 1.22+

### Build from Source

```bash
git clone https://github.com/rvald/goclaw.git
cd goclaw
make build
```

This produces a `bin/goclaw` binary.

---

## 🚦 Usage

### Development Mode

Run directly with `go run`:

```bash
# Minimal (Pairing enabled, Discord disabled)
go run ./cmd/goclaw/ --token secret

# Full Stack (with Discord)
export DISCORD_BOT_TOKEN="your_token"
export DISCORD_GUILD_ID="your_guild_id"
go run ./cmd/goclaw/ --token secret
```

### Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--port` | `18789` | Server port |
| `--bind` | `loopback` | Interface to bind (`loopback` or `lan`) |
| `--token` | (none) | Legacy shared secret (fallback auth) |
| `--state-dir` | `$XDG_STATE_HOME/goclaw` | Directory for pairing state |
| `--discord-token` | `$DISCORD_BOT_TOKEN` | Discord bot token |
| `--guild-id` | `$DISCORD_GUILD_ID` | Discord guild ID (for instant commands) |

### Makefile

- `make test`: Run all unit tests.
- `make build`: Compile binary.
- `make run`: Run with default dev settings (requires `.env` for Discord).

---

## � Deployment

### Docker

```bash
docker build -t goclaw .
docker run -d -p 18789:18789 -v goclaw_data:/data goclaw
```

### Systemd

1.  Copy `goclaw.service` to `/etc/systemd/system/`.
2.  `systemctl enable --now goclaw`.

---

## �🔐 Device Pairing

GoClaw uses a trust-on-first-use (TOFU) model with operator approval.

1.  **Device Connects**: Sends public key + signed challenge.
2.  **Server**:
    - If **Localhost**: Auto-approves & pairs.
    - If **Remote**: Rejects with `NOT_PAIRED`, creates pending request.
3.  **Operator**:
    - Sees request via Discord `/devices`.
    - Runs `/approve <request_id>`.
4.  **Device Reconnects**: Authenticated & paired.

---

## 📄 License

Proprietary / Private.
