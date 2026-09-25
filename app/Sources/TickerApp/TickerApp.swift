import SwiftUI

@main
struct TickerApp: App {
    var body: some Scene {
        WindowGroup("Ticker") {
            ContentView()
                // The stat columns need ~1100pt before they collide; below this the
                // window is not usable rather than merely cramped.
                .frame(minWidth: 1120, minHeight: 560)
        }
        // 16:9, matching the concept's proportions rather than a square window.
        .defaultSize(width: 1440, height: 820)
    }
}
