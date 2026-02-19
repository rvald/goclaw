# Native Node Service: DashboardService

This document specifies the **iOS Service Layer** for the Dashboard Native Node feature. This service manages the state of the native UI and handles business logic for incoming commands.

## 1. Class Structure

**File:** `apps/ios/Sources/Dashboard/DashboardService.swift`

```swift
import Foundation
import Observation
import OpenClawKit

@MainActor
@Observable
final class DashboardService {
    // MARK: - State
    
    /// The current list of tiles to display
    var tiles: [DashboardTile] = []
    
    /// Map of tile ID to tile index for O(1) lookups
    private var tileIndex: [String: Int] = [:]

    // MARK: - API
    
    /// Replaces the entire dashboard layout
    func setLayout(tiles: [DashboardTile]) {
        self.tiles = tiles
        self.rebuildIndex()
    }
    
    /// Updates a single tile's content
    func updateTile(id: String, content: String?, detail: String?, label: String?, style: DashboardTile.Style?, loading: Bool?) throws {
        guard let index = self.tileIndex[id] else {
            throw DashboardError.tileNotFound(id)
        }
        
        // Copy-on-write update
        var tile = self.tiles[index]
        if let content { tile.content = content }
        if let detail { tile.detail = detail }
        if let label { tile.label = label }
        if let style { tile.style = style }
        if let loading { tile.isLoading = loading }
        
        self.tiles[index] = tile
    }
    
    // MARK: - Private
    
    private func rebuildIndex() {
        self.tileIndex = Dictionary(uniqueKeysWithValues: self.tiles.enumerated().map { ($0.element.id, $0.offset) })
    }
}
```

## 2. Models

**File:** `apps/ios/Sources/Dashboard/DashboardModels.swift`

```swift
import Foundation

struct DashboardTile: Identifiable, Codable, Sendable {
    enum TileType: String, Codable, Sendable {
        case text
        case button
    }
    
    enum Style: String, Codable, Sendable {
        case normal
        case success
        case warning
        case danger
    }
    
    let id: String
    let type: TileType
    var title: String
    var size: String // "small", "medium", "large"
    
    // Dynamic Content
    var content: String?
    var detail: String?
    var label: String?
    var style: Style = .normal
    var isLoading: Bool = false
}

enum DashboardError: Error {
    case tileNotFound(String)
}
```

## 3. Integration

This service must be initialized in `NodeAppModel` and exposed to the SwiftUI environment.

**File:** `apps/ios/Sources/Model/NodeAppModel.swift`

```swift
// Add property
let dashboardService = DashboardService()
```
