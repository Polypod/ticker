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
            sidebar
                .navigationSplitViewColumnWidth(min: 170, ideal: 190)
        } detail: {
            detail
                .safeAreaInset(edge: .top) { titleBarSpacer }
        }
        .background(Theme.backdrop)
        // System blue fights the monochrome palette.
        .tint(Theme.up.opacity(0.6))
        .preferredColorScheme(.dark)
        .task { client.start() }
    }

    /// The window draws its content full height, so the traffic lights and the
    /// window title land on top of the first row without this.
    private var titleBarSpacer: some View {
        Color.clear.frame(height: 34)
    }

    /// Hand-rolled rather than a `List`: inside a split view the list manages
    /// its own insets (so it ignored the title bar spacer) and paints selection
    /// with the system accent, which is the blue that keeps coming back.
    private var sidebar: some View {
        VStack(alignment: .leading, spacing: 4) {
            // A real laid-out view, not padding: the sidebar container
            // collapses top padding, which is how the traffic lights kept
            // landing on the first row.
            Color.clear.frame(height: 44)

            ForEach(Section.allCases) { item in
                Button {
                    section = item
                } label: {
                    Label(item.rawValue, systemImage: item.icon)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundStyle(section == item ? Theme.primaryText : Theme.secondaryText)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 7)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(
                            RoundedRectangle(cornerRadius: 8)
                                .fill(section == item ? Color.white.opacity(0.12) : .clear)
                        )
                }
                .buttonStyle(.plain)
            }

            Spacer(minLength: 0)
        }
        .padding(.horizontal, 10)
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

    /// A price that moved should be visible without reading the number, the way
    /// the terminal UI flashes the row.
    @State private var flashColor: Color = .clear
    @State private var flashOpacity: Double = 0

    private var variable: Bool { asset.meta.isVariablePrecision }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline, spacing: 16) {
                VStack(alignment: .leading, spacing: 4) {
                    HStack(spacing: 7) {
                        Text(asset.symbol)
                            .font(.system(.title3, design: .monospaced, weight: .semibold))
                            .foregroundStyle(Theme.primaryText)

                        // Open market, same signal the TUI's dot carries.
                        Circle()
                            .fill(asset.exchange.isRegularTradingSession ? Theme.up : Theme.secondaryText)
                            .frame(width: 5, height: 5)
                    }

                    Text(asset.name.isEmpty ? asset.exchange.name : asset.name)
                        .font(.caption)
                        .foregroundStyle(Theme.secondaryText)
                        .lineLimit(1)
                }

                Spacer(minLength: 12)

                VStack(alignment: .trailing, spacing: 3) {
                    Text(asset.quotePrice.price.price(variable))
                        .font(.system(.title3, design: .monospaced))
                        .foregroundStyle(Theme.primaryText)

                    Text(
                        asset.quotePrice.change.signed(variable)
                            + "  "
                            + asset.quotePrice.changePercent.signedPercent
                    )
                    .font(.system(.caption, design: .monospaced))
                    .foregroundStyle(Theme.direction(asset.quotePrice.changePercent))
                }
                .frame(width: 170, alignment: .trailing)

                ProvenanceTag(source: source, age: age)
            }

            StatStrip(items: quoteStats)

            if asset.position.quantity > 0 {
                StatStrip(items: positionStats, tint: Theme.direction(asset.position.totalChange.percent))
            }

            TagStrip(asset: asset)
        }
        .padding(.horizontal, 18)
        .padding(.vertical, 14)
        .background(flashColor.opacity(flashOpacity))
        .panel()
        .onChange(of: asset.quotePrice.price, initial: false) { previous, current in
            guard current != previous else { return }

            flashColor = current > previous ? Theme.up : Theme.down
            flashOpacity = 0.28

            withAnimation(.easeOut(duration: 1.1)) {
                flashOpacity = 0
            }
        }
    }

    private var quoteStats: [Stat] {
        var stats: [Stat] = []

        if asset.quotePrice.pricePrevClose != 0 {
            stats.append(Stat("prev", asset.quotePrice.pricePrevClose.price(variable), width: 120))
        }

        if asset.quotePrice.priceOpen != 0 {
            stats.append(Stat("open", asset.quotePrice.priceOpen.price(variable), width: 120))
        }

        if asset.quotePrice.priceDayHigh != 0, asset.quotePrice.priceDayLow != 0 {
            stats.append(Stat(
                "day",
                asset.quotePrice.priceDayLow.price(variable) + "–" + asset.quotePrice.priceDayHigh.price(variable),
                width: 190
            ))
        }

        if asset.quoteExtended.fiftyTwoWeekHigh != 0, asset.quoteExtended.fiftyTwoWeekLow != 0 {
            stats.append(Stat(
                "52wk",
                asset.quoteExtended.fiftyTwoWeekLow.price(variable)
                    + "–"
                    + asset.quoteExtended.fiftyTwoWeekHigh.price(variable),
                width: 200
            ))
        }

        if asset.quoteExtended.marketCap != 0 {
            stats.append(Stat("mcap", asset.quoteExtended.marketCap.abbreviated, width: 130))
        }

        if asset.quoteExtended.volume != 0 {
            stats.append(Stat("vol", asset.quoteExtended.volume.abbreviated, width: 130))
        }

        return stats
    }

    private var positionStats: [Stat] {
        [
            Stat("qty", asset.position.quantity.price(variable), width: 120),
            Stat("avg", asset.position.unitCost.price(variable), width: 120),
            Stat("value", asset.position.value.price(false), width: 190),
            Stat("weight", asset.position.weight.signedPercent.replacingOccurrences(of: "+", with: ""), width: 130),
            Stat(
                "total",
                asset.position.totalChange.amount.signed(false)
                    + " "
                    + asset.position.totalChange.percent.signedPercent,
                width: 200,
                highlighted: true
            ),
            Stat(
                "day",
                asset.position.dayChange.amount.signed(false)
                    + " "
                    + asset.position.dayChange.percent.signedPercent,
                width: 200,
                highlighted: true
            )
        ]
    }
}

struct Stat: Identifiable {
    let id = UUID()
    let label: String
    let value: String
    let width: CGFloat
    let highlighted: Bool

    init(_ label: String, _ value: String, width: CGFloat = 130, highlighted: Bool = false) {
        self.label = label
        self.value = value
        self.width = width
        self.highlighted = highlighted
    }
}

struct StatStrip: View {
    let items: [Stat]
    var tint: Color = Theme.primaryText

    var body: some View {
        HStack(spacing: 8) {
            ForEach(items) { item in
                HStack(spacing: 5) {
                    Text(item.label.uppercased())
                        .font(.system(size: 9, design: .monospaced))
                        .foregroundStyle(Theme.secondaryText)

                    Text(item.value)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(item.highlighted ? tint : Theme.primaryText.opacity(0.75))

                    Spacer(minLength: 0)
                }
                .frame(width: item.width, alignment: .leading)
            }

            Spacer(minLength: 0)
        }
    }
}

/// Currency, quote delay and exchange — the terminal UI's `--show-tags`.
struct TagStrip: View {
    let asset: Asset

    private var tags: [String] {
        var values: [String] = []

        if !asset.currency.fromCurrencyCode.isEmpty {
            let converted = asset.currency.toCurrencyCode

            values.append(
                converted.isEmpty || converted == asset.currency.fromCurrencyCode
                    ? asset.currency.fromCurrencyCode
                    : "\(asset.currency.fromCurrencyCode) → \(converted)"
            )
        }

        if !asset.exchange.delayText.isEmpty {
            values.append(asset.exchange.delayText)
        } else if asset.exchange.delay > 0 {
            values.append("delayed \(Int(asset.exchange.delay))m")
        } else {
            values.append("real-time")
        }

        if !asset.exchange.name.isEmpty {
            values.append(asset.exchange.name)
        }

        return values
    }

    var body: some View {
        HStack(spacing: 6) {
            ForEach(tags, id: \.self) { tag in
                Text(tag)
                    .font(.system(size: 9, design: .monospaced))
                    .foregroundStyle(Theme.secondaryText)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(
                        RoundedRectangle(cornerRadius: 4)
                            .fill(Color.white.opacity(0.06))
                    )
            }

            Spacer(minLength: 0)
        }
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
            metric("Value", summary.value.price(false), Theme.primaryText)
            metric("Cost", summary.cost.price(false), Theme.primaryText.opacity(0.7))
            metric(
                "Day",
                summary.dayChange.amount.signed(false) + "  " + summary.dayChange.percent.signedPercent,
                Theme.direction(summary.dayChange.percent)
            )
            metric(
                "Total",
                summary.totalChange.amount.signed(false) + "  " + summary.totalChange.percent.signedPercent,
                Theme.direction(summary.totalChange.percent)
            )
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
