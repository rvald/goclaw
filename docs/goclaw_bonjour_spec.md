# Go Implementation Spec: Bonjour Discovery

**Status:** Draft
**Owner:** Engineering
**Relates to:** `docs/gateway/bonjour.md`

## Overview

This document outlines the technical specification for implementing **Bonjour (mDNS/DNS-SD)** discovery in the Go Gateway (`goclaw`). The goal is to replicate the existing TypeScript behavior (`src/infra/bonjour.ts`) to allow iOS/Android nodes to discover the gateway on the LAN.

## Requirements

1.  **Service Advertisement:** Advertise `_openclaw-gw._tcp` on the local network.
2.  **TXT Records:** Publish a dynamic set of TXT records (the "beacon") containing gateway metadata.
3.  **Lifecycle Management:** Start/Stop advertising without blocking the main process.
4.  **Resiliency:** Automatically handle name conflicts and re-advertise if the service drops (watchdog).
5.  **Configuration:** Support configuration via flags/env vars (comparable to `openclaw.json`).

## Dependencies

We will use a standard Go mDNS library.
*   **Recommendation:** `github.com/hashicorp/mdns`.
*   *Decision:* **`github.com/hashicorp/mdns`** per user request.

## Architecture

### `discovery` Package

A new package `discovery` will encapsulate the logic.

```go
package discovery

import (
    "context"
    "github.com/hashicorp/mdns"
)

// Advertiser manages the mDNS service registration.
type Advertiser struct {
    server *mdns.Server
    cfg    Config
}

// Config holds the parameters for the beacon.
type Config struct {
    InstanceName string // e.g., "MyMac (OpenClaw)"
    Port         int    // Gateway WS port (18789)
    SSHPort      int    // Optional, defaults to 22
    LanHost      string // Hostname.local
    // ... other metadata
}
```

### Logic Flow

1.  **Initialization:**
    *   Sanitize `InstanceName`: Remove special characters, ensure valid DNS label.
    *   Construct `TXT` record map.

2.  **Startup (`Start(ctx)`):**
    *   Initialize `zeroconf.Register`.
    *   This is an asynchronous operation in `zeroconf`.
    *   Log "Advertising..." on success.

3.  **Shutdown (`Stop()`):**
    *   Call `server.Shutdown()` to deregister cleanly.

4.  **Watchdog (Optional/Advanced):**
    *   The TS implementation checks every 60s.
    *   In Go, we can listen to the `zeroconf` object's lifecycle or simply rely on the library's re-announce mechanisms. For v1, we will rely on the library, but expose a `Restart()` method.

## Data Structures (TXT Record)

The `TXT` record MUST mirror the TypeScript implementation:

| Key | Value Example | Condition |
| :--- | :--- | :--- |
| `role` | `gateway` | Always |
| `transport` | `gateway` | Always |
| `gatewayPort` | `18789` | Integer as string |
| `lanHost` | `my-mac.local` | Gateway hostname |
| `displayName` | `MyMac` | Friendly name |
| `sshPort` | `22` | If SSH is enabled |
| `gatewayTls` | `1` | If TLS enabled |
| `gatewayTlsSha256`| `...` | If TLS enabled |

## CLI Integration

The functionality should be exposed via the `goclaw` CLI.

```bash
# Debug mode to just run advertisement
goclaw debug discovery --port 18789 --name "Test Gateway"
```

## Error Handling

*   **Name Conflict:** `zeroconf` usually handles renaming (appending `(2)`). We should log this event if the library exposes it.
*   **Bind Failure:** If port 5353 cannot be bound, log a warning but **do not crash**. The gateway must remain usable via direct IP/SSH even if Bonjour fails.
