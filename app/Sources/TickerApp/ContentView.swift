import SwiftUI

enum Section: String, CaseIterable, Identifiable {
    case watchlist = "Watchlist"
    case positions = "Positions"

    var id: String { rawValue }

    var icon: String {
        switch self {
        case .watchlist: return "chart.line.uptrend.xyaxis"
        case .positions: return "briefcase"
        }
    }
}

struct ContentView: View {
    @State private var client = DaemonClient()
    @State private var section: Section = .watchlist

    var body: some View {
        NavigationSplitView {
            List(Section.allCases, selection: $section) { item in
                Label(item.rawValue, systemImage: item.icon)
                    .tag(item)
            }
            .navigationSplitViewColumnWidth(min: 170, ideal: 190)
            .scrollContentBackground(.hidden)
        } detail: {
            detail
        }
        .background(Theme.backdrop)
        .preferredColorScheme(.dark)
        .task { client.start() }
    }

    @ViewBuilder
    private var detail: some View {
        ZStack {
            Theme.backdrop

            switch client.state {
            case .searching:
                StatusView(title: "Looking for the daemon", detail: "Start it with `ticker serve`.")
            case let .failed(message):
                StatusView(title: "Not connected", detail: message)
            case .connected:
                if let snapshot = client.snapshot {
                    quotes(snapshot)
                } else {
                    StatusView(title: "Connected", detail: "Waiting for the first snapshot.")
                }
            }
        }
    }

    private func quotes(_ snapshot: Snapshot) -> some View {
        VStack(spacing: 14) {
            GlassEffectContainer(spacing: 14) {
                ScrollView {
                    LazyVStack(spacing: 10) {
                        ForEach(rows(snapshot)) { asset in
                            AssetRow(
                                asset: asset,
                                source: snapshot.sources[asset.symbol] ?? "unknown",
                                age: client.lastEvent.map { Date().timeIntervalSince($0) } ?? 0
                            )
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 14)
                }
            }

            SummaryBar(summary: snapshot.summary)
                .padding(.horizontal, 16)
                .padding(.bottom, 14)
        }
    }

    private func rows(_ snapshot: Snapshot) -> [Asset] {
        switch section {
        case .watchlist:
            return snapshot.assets
        case .positions:
            return snapshot.assets.filter { $0.position.quantity > 0 }
        }
    }
}

struct AssetRow: View {
    let asset: Asset
    let source: String
    let age: TimeInterval

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 16) {
            VStack(alignment: .leading, spacing: 3) {
                Text(asset.symbol)
                    .font(.system(.title3, design: .monospaced, weight: .semibold))
                    .foregroundStyle(Theme.primaryText)

                Text(asset.name.isEmpty ? asset.exchange.name : asset.name)
                    .font(.caption)
                    .foregroundStyle(Theme.secondaryText)
                    .lineLimit(1)
            }

            Spacer(minLength: 12)

            VStack(alignment: .trailing, spacing: 3) {
                Text(asset.quotePrice.price.currency)
                    .font(.system(.title3, design: .monospaced))
                    .foregroundStyle(Theme.primaryText)

                Text(asset.quotePrice.changePercent.signedPercent)
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(Theme.direction(asset.quotePrice.changePercent))
            }

            ProvenanceTag(source: source, age: age)
        }
        .padding(.horizontal, 18)
        .padding(.vertical, 14)
        .panel()
    }
}

/// Where the number came from and how old it is. A Tiingo stream tick and a
/// three-minute-old IBKR price look identical otherwise.
struct ProvenanceTag: View {
    let source: String
    let age: TimeInterval

    private var isStale: Bool { age > 60 }

    var body: some View {
        VStack(alignment: .trailing, spacing: 3) {
            Text(source.uppercased())
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(Theme.secondaryText)

            HStack(spacing: 4) {
                Circle()
                    .fill(isStale ? Theme.down.opacity(0.7) : Theme.up.opacity(0.7))
                    .frame(width: 5, height: 5)

                Text(isStale ? "\(Int(age))s" : "live")
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(Theme.secondaryText)
            }
        }
        .frame(width: 58, alignment: .trailing)
    }
}

struct SummaryBar: View {
    let summary: PositionSummary

    var body: some View {
        HStack(spacing: 22) {
            metric("Value", summary.value.currency, Theme.primaryText)
            metric("Day", summary.dayChange.percent.signedPercent, Theme.direction(summary.dayChange.percent))
            metric("Total", summary.totalChange.percent.signedPercent, Theme.direction(summary.totalChange.percent))
        }
        .padding(.horizontal, 22)
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .panel(cornerRadius: 22)
    }

    private func metric(_ label: String, _ value: String, _ tint: Color) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label.uppercased())
                .font(.system(size: 9, design: .monospaced))
                .foregroundStyle(Theme.secondaryText)

            Text(value)
                .font(.system(.body, design: .monospaced, weight: .medium))
                .foregroundStyle(tint)
        }
    }
}

struct StatusView: View {
    let title: String
    let detail: String

    var body: some View {
        VStack(spacing: 8) {
            Text(title)
                .font(.headline)
                .foregroundStyle(Theme.primaryText)

            Text(detail)
                .font(.system(.caption, design: .monospaced))
                .foregroundStyle(Theme.secondaryText)
        }
        .padding(28)
        .panel(cornerRadius: 22)
    }
}
