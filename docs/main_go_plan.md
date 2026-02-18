# `main.go` — Entry Point

> CLI entrypoint for the Go gateway. Composition root that parses config, wires modules, and manages lifecycle.

**Status: ✅ Implemented** — [main.go](cmd/goclaw/main.go)

---

## Architecture

```
main() → parseConfig() → validateConfig() → run()
                                                ├── pairing.NewStore()  → Store (persistent state)
                                                ├── pairing.NewService() → Service (orchestration)
                                                ├── gateway.New()    → Server + Registry + Invoker + PairingSvc
                                                ├── discord.NewBot() → Bot (optional, with pairing commands)
                                                └── signal.NotifyContext() → graceful shutdown
```

## Configuration

Defined in [main.go `Config` struct](cmd/goclaw/main.go#L21-L29):

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--port` | `GOCLAW_PORT` | `18789` | WebSocket server port |
| `--bind` | `GOCLAW_BIND` | `loopback` | `loopback` (127.0.0.1) or `lan` (0.0.0.0) |
| `--token` | `GOCLAW_TOKEN` | _(empty)_ | Auth token for node connections |
| `--discord-token` | `DISCORD_BOT_TOKEN` | _(empty)_ | Discord bot token (optional) |
| `--guild-id` | `DISCORD_GUILD_ID` | _(empty)_ | Discord guild for slash commands |
| `--tick` | — | `15s` | Keepalive tick interval |
| `--state-dir` | `XDG_STATE_HOME` | `~/.local/state/goclaw` | Persistent state (pairing, etc.) |

**Priority:** CLI flags → environment variables → defaults

## Validation

Defined in [main.go `validateConfig()`](cmd/goclaw/main.go#L46-L55):

- **Port range** — must be 1–65535
- **Bind mode** — must be `"loopback"` or `"lan"`
- **LAN guardrail** — `--bind lan` without `--token` is refused (prevents unauthenticated exposure)

## Signal Handling

Defined in [main.go `run()`](cmd/goclaw/main.go#L57-L119):

- `SIGINT` / `SIGTERM` → cancel context → broadcast `"shutdown"` event → drain connections → exit
- **Shutdown timeout** — 5 seconds max, then force exit
- Discord bot stopped before server shutdown

## Startup Banner

```
goclaw v0.1.0
  ws://127.0.0.1:18789  auth=token  bind=loopback
  discord: disabled  pairing: enabled
  state: ~/.local/state/goclaw
  health: http://127.0.0.1:18789/health
```

---

## What We Cut from OpenClaw

24+ subsystems removed — no config files, no plugin system, no agent/chat, no canvas/browser control, no cron, no tailscale, no hot-reload, no PID lock, no SIGUSR1 restart. Full list in [implementation_plan.md](docs/implementation_plan.md).
