# Implementation Plan: Go Gateway Device Pairing

**Goal:** Implement the Device Pairing and Bonjour Discovery features in the Go Gateway (`goclaw`), enabling iOS/Android nodes to discover and pair with the gateway.

## User Review Required

> [!IMPORTANT]
> **Missing Codebase:** The `goclaw` directory is currently empty. This plan assumes initializing a new Go project from scratch. If an existing codebase exists elsewhere, please point to it.

## Proposed Changes

We will build the feature in **Phases**, starting with the foundation (Discovery) and moving to the application logic (Pairing).

### Phase 1: Project Initialization & Structure
*   **Goal:** Set up a runnable CLI skeleton.
*   **Files:**
    *   [NEW] `goclaw/go.mod`: Initialize Go module (`go mod init github.com/openclaw/openclaw/goclaw`).
    *   [NEW] `goclaw/main.go`: Entrypoint.
    *   [NEW] `goclaw/cmd/root.go`: Root CLI command (using `spf13/cobra` is recommended).

### Phase 2: Bonjour Discovery (The "Beacon")
*   **Goal:** Allow iOS apps to see the gateway on the network.
*   **Spec:** `docs/gateway/goclaw_bonjour_spec.md`
*   **Files:**
    *   [NEW] `goclaw/pkg/discovery/discovery.go`: Implementation of mDNS advertiser using `github.com/grandcat/zeroconf`.
    *   [NEW] `goclaw/cmd/debug_discovery.go`: A `debug discovery` command to run the advertiser in isolation for testing.
*   **Dependencies:** `github.com/grandcat/zeroconf`

### Phase 3: Pairing Primitives
*   **Goal:** Manage pending requests and issue tokens.
*   **Spec:** `docs/pairing.md` (Option B: Gateway-owned pairing).
*   **Files:**
    *   [NEW] `goclaw/pkg/pairing/store.go`: In-memory or file-based store for `pending.json` and `paired.json`.
    *   [NEW] `goclaw/pkg/pairing/types.go`: Structs for `PendingRequest`, `PairedNode`.
    *   [NEW] `goclaw/pkg/crypto/token.go`: Token generation logic (JWT or opaque secure random).

### Phase 4: CLI for Pairing
*   **Goal:** Allow the user to approve/reject nodes via CLI.
*   **Files:**
    *   [NEW] `goclaw/cmd/nodes.go`: Parent command.
    *   [NEW] `goclaw/cmd/nodes_pending.go`: List pending.
    *   [NEW] `goclaw/cmd/nodes_approve.go`: Approve request & issue token.

## Verification Plan

### Automated Tests
Run `go test ./...` in the `goclaw` directory.
*   **Discovery:** Unit test `discovery.go` (mocking the zeroconf server if possible, or integration test).

### Manual Verification
1.  **Discovery Test:**
    *   Run `go run ./goclaw/main.go debug discovery`
    *   On macOS, run `dns-sd -B _openclaw-gw._tcp local.` to confirm the service appears.
    *   On iOS, open the OpenClaw app and check if the gateway appears in the list.

2.  **Pairing Test:**
    *   (Requires WebSocket server implementation, which is out of scope for *just* this plan, but needed for end-to-end).
    *   Mock: Create a dummy pending request in `pending.json`.
    *   Run `go run ./goclaw/main.go nodes pending` -> See request.
    *   Run `go run ./goclaw/main.go nodes approve <id>` -> See token generated and moved to `paired.json`.
