// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "TickerApp",
    platforms: [.macOS("26.0")],
    targets: [
        .executableTarget(
            name: "TickerApp",
            path: "Sources/TickerApp"
        )
    ]
)
