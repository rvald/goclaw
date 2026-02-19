# Native Node Protocol: Dashboard v1

This document is the **Protocol Specification** for the "Native Dashboard" feature. It defines the JSON commands and events exchanged between the Gateway and the iOS Native Node.

## 1. Capability Definition

The iOS Node must declare this capability in its `connect` handshake (or `NodeAppModel` registration).

*   **Capability Name**: `com.example.native_dashboard`
*   **Version**: `1.0`

## 2. Gateway -> Node Commands

These commands are sent by the Gateway (via `node.invoke`) to control the iOS UI.

### 2.1 `dashboard.set_layout`

Configures the grid of widgets displayed on the screen.

**Params:**
```jsonc
{
  "tiles": [
    {
      "id": "weather_main",
      "type": "text",
      "title": "Weather",
      "size": "medium" // small, medium, large
    },
    {
      "id": "garage_door",
      "type": "button",
      "title": "Garage",
      "label": "Closed",
      "style": "success" // success, warning, danger
    }
  ]
}
```

**Response:** `{ "ok": true }`

### 2.2 `dashboard.update_tile`

Updates the data of a specific tile in real-time.

**Params:**
```jsonc
{
  "id": "weather_main",
  "content": "72°F",
  "detail": "Sunny"
}
```

or for a button:
```jsonc
{
  "id": "garage_door",
  "label": "Opening...",
  "style": "warning",
  "loading": true
}
```

**Response:** `{ "ok": true }`

## 3. Node -> Gateway Events

These events are sent by the iOS Node (via `node.emit` or `agent.request`) when the user interacts with the UI.

### 3.1 `dashboard.action`

Sent when a user taps a tile (e.g., a button).

**Payload:**
```jsonc
{
  "id": "garage_door",
  "action": "tap",
  "ts": 1678900000
}
```

## 4. Error Handling

*   If `dashboard.update_tile` targets a non-existent ID, the node should return error `NOT_FOUND`.
*   If the app is in the background, `dashboard.update_tile` should **succeed** (state updated silently).
