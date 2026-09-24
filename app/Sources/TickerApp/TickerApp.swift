import SwiftUI

@main
struct TickerApp: App {
    var body: some Scene {
        WindowGroup("Ticker") {
            ContentView()
                .frame(minWidth: 420, minHeight: 320)
        }
        .defaultSize(width: 980, height: 640)
        .windowStyle(.hiddenTitleBar)
    }
}
