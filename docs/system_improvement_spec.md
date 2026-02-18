# OpenClaw Gateway (Go) - Improvement Specification

## 1. Executive Summary
This document outlines the architectural changes implemented to upgrade the **Go Gateway** with production-grade reliability, observability, and security features.

**Scope:**
1.  **Observability**: Structured JSON logging and Prometheus metrics for real-time monitoring.
2.  **Reliability**: Robust WebSocket connection handling with resource limits and heartbeats.
3.  **Security**: IP-based rate limiting to prevent abuse.
4.  **Deployment**: Containerization (Docker) and Service management (Systemd).

## 2. Technical Requirements

### 2.1 Observability
**Goal**: Provide full visibility into server health, client connections, and error rates.

*   **REQ-O1 (Metrics Endpoint)**: The gateway MUST expose a `GET /metrics` endpoint compatible with Prometheus.
    *   **Metric**: `goclaw_connected_clients` (Gauge) - Active WS connections.
    *   **Metric**: `goclaw_messages_total` (Counter) - Message throughput (in/out).
    *   **Metric**: `goclaw_errors_total` (Counter) - Error rates by type.
*   **REQ-O2 (Structured Logging)**:
    *   Logs MUST be structured (JSON) for production ingestion.
    *   Logs MUST support dual output: JSON to file (for aggregation) and Text to console (for dev).
    *   **Rotation**: Logs MUST verify rotation settings (Max 10MB, Keep 3).

### 2.2 Reliability (WebSocket Safety)
**Goal**: Prevent resource exhaustion (OOM, file descriptors) and detect zombie connections.

*   **REQ-R1 (Message Limits)**: The server MUST enforce a strict read limit (default **512KB**) per message. Connections exceeding this limit MUST be closed with `CloseMessageTooBig`.
*   **REQ-R2 (Heartbeats)**:
    *   Server MUST send `PING` frames periodically (default **54s**).
    *   Server MUST expect `PONG` frames within a deadline (default **60s**).
    *   Connections failing to respond MUST be forcibly closed.
*   **REQ-R3 (Read Deadlines)**: All WebSocket reads MUST have a deadline derived from the heartbeat interval.

### 2.3 Security (Rate Limiting)
**Goal**: Mitigate DoS and brute-force attacks on the handshake endpoint.

*   **REQ-S1 (Connection Rate Limiting)**:
    *   The `/ws` endpoint MUST enforce a rate limit per Source IP.
    *   **Algorithm**: Token Bucket.
    *   **Limits**: 5 requests/second (Rate), 10 requests (Burst).
    *   Rejected connections MUST receive HTTP 429 (Too Many Requests).

### 2.4 Deployment & Operations
**Goal**: Standardize deployment artifacts for Linux environments.

*   **REQ-D1 (Containerization)**:
    *   Provide a multi-stage `Dockerfile`.
    *   Base image: `gcr.io/distroless/static` (Secure, non-root).
    *   Combined with static Go binary (`CGO_ENABLED=0`).
*   **REQ-D2 (Service Management)**:
    *   Provide a `systemd` unit file (`goclaw.service`).
    *   Enable security hardening: `NoNewPrivileges`, `ProtectSystem`, `PrivateTmp`.
    *   Auto-restart on failure.

## 3. Implementation Status
All requirements defined above have been implemented and verified via Test-Driven Development (TDD).

-   **Codebase**: `internal/gateway/`, `internal/logger/`, `cmd/goclaw/`
-   **Tests**: `internal/gateway/*_test.go`
-   **Artifacts**: `Dockerfile`, `goclaw.service`
