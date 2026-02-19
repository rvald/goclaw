# Native Node Verification Plan

This document outlines how to verify the end-to-end functionality of the Native Node Dashboard.

## 1. Unit Tests (iOS)

**File:** `apps/ios/Tests/DashboardTests.swift`

*   **Service State**: Verify `DashboardService.updateTile` correctly modifies the `tiles` array.
*   **Error Handling**: Verify `updateTile` throws `tileNotFound` for invalid IDs.
*   **Decoding**: Verify `DashboardTile` correctly decodes from the JSON definition in `native-node-protocol.md`.

## 2. Manual Verification (Gateway Integration)

Since this involves a real device and a Gateway, manual verification is required.

### 2.1 Setup
1.  Run a local Gateway: `openclaw gateway`
2.  Run the iOS App in Simulator or Device.
3.  Connect iOS App to Gateway (manual connect to `127.0.0.1:18789`).

### 2.2 Test: Layout Push
Use the CLI or `curl` (via an agent script) to push a layout.

```bash
# Verify the node declares the capability
openclaw nodes list --json | jq '.[] | select(.caps | contains(["com.example.native_dashboard"]))'
```

**Script `test_layout.py`:**
```python
import openclaw
# ... connect to gateway ...
await gateway.node.invoke(
    node_id="<iOS_DEVICE_ID>",
    command="dashboard.set_layout",
    params={
        "tiles": [{"id": "t1", "type": "button", "title": "Test"}]
    }
)
```
*   **Verification**: Ensure the "Test" button appears on the iOS screen.

### 2.3 Test: Real-time Update
**Script `test_update.py`:**
```python
await gateway.node.invoke(
    node_id="<iOS_DEVICE_ID>",
    command="dashboard.update_tile",
    params={"id": "t1", "label": "Updated!", "style": "success"}
)
```
*   **Verification**: Ensure the button text changes to "Updated!" and turns green immediately.

### 2.4 Test: Node -> Gateway Event
1.  Tap the "Test" button on iOS.
2.  **Verification**: Check Gateway logs or Agent listener for the `dashboard.action` event.
    ```
    Element: dashboard.action
    Payload: { "id": "t1", "action": "tap", ... }
    ```

## 3. Crash/Stability
*   **Background Test**: Put app in background, send 100 updates. Bring to foreground. Ensure state is correct and app did not crash.
