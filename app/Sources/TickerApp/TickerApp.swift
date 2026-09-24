import SwiftUI

@main
struct TickerApp: App {
    var body: some Scene {
        WindowGroup("Ticker") {
            ContentView()
                .frame(minWidth: 420, minHeight: 320)
        }
        // 16:9, matching the concept's proportions rather than a square window.
        .defaultSize(width: 1280, height: 720)
        .windowStyle(.hiddenTitleBar)
    }
}
