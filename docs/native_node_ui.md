# Native Node UI: Dashboard Components

This document specifies the **iOS UI Layer** (SwiftUI) for the Dashboard Native Node feature.

## 1. Views

**File:** `apps/ios/Sources/Dashboard/Views/DashboardGrid.swift`

The main container view that renders the grid of tiles based on `DashboardService` state.

```swift
import SwiftUI

struct DashboardGrid: View {
    @Environment(DashboardService.self) var service
    
    let columns = [
        GridItem(.adaptive(minimum: 150, maximum: 170), spacing: 12)
    ]
    
    var body: some View {
        ScrollView {
            LazyVGrid(columns: columns, spacing: 12) {
                ForEach(service.tiles) { tile in
                    DashboardTileView(tile: tile)
                }
            }
            .padding()
        }
        .background(Color(uiColor: .systemGroupedBackground))
    }
}
```

**File:** `apps/ios/Sources/Dashboard/Views/DashboardTileView.swift`

Renders individual tiles based on their `type`.

```swift
struct DashboardTileView: View {
    let tile: DashboardTile
    
    var body: some View {
        Group {
            switch tile.type {
            case .text:
                TextTileView(tile: tile)
            case .button:
                ButtonTileView(tile: tile)
            }
        }
        .frame(height: tile.size == "large" ? 312 : 150)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .cornerRadius(16)
        .shadow(color: .black.opacity(0.1), radius: 2, x: 0, y: 1)
    }
}
```

## 2. Event Handling

**File:** `apps/ios/Sources/Dashboard/Views/ButtonTileView.swift`

Handles user interaction and communicates back to `NodeAppModel` -> Gateway.

```swift
struct ButtonTileView: View {
    let tile: DashboardTile
    @Environment(NodeAppModel.self) var appModel
    
    var body: some View {
        Button {
            Task {
                await appModel.sendDashboardAction(id: tile.id, action: "tap")
            }
        } label: {
            VStack {
                if tile.isLoading {
                    ProgressView()
                } else {
                    Text(tile.label ?? tile.title)
                        .font(.headline)
                        .foregroundColor(colorForStyle(tile.style))
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .disabled(tile.isLoading)
    }
    
    // ... helper for colors
}
```

## 3. Root Integration

**File:** `apps/ios/Sources/RootCanvas.swift`

The `DashboardGrid` should be displayed when the app is in "Native Dashboard Mode" (triggered perhaps by a setting or specific URL state, or replacing `CanvasContent` entirely for this specific build).

```swift
// In RootCanvas.swift or similar
if appModel.isDashboardMode {
    DashboardGrid()
        .environment(appModel.dashboardService)
} else {
    CanvasContent(...)
}
```
