import SwiftUI

/// Dark-only palette. The reference concept is monochrome with one accent per
/// direction, so colour carries meaning here and nothing else does.
enum Theme {
    static let background = Color(red: 0.04, green: 0.045, blue: 0.055)
    static let backgroundAccent = Color(red: 0.08, green: 0.09, blue: 0.12)
    static let primaryText = Color.white.opacity(0.92)
    static let secondaryText = Color.white.opacity(0.45)
    static let up = Color(red: 0.42, green: 0.85, blue: 0.62)
    static let down = Color(red: 0.95, green: 0.45, blue: 0.45)

    static func direction(_ value: Double) -> Color {
        value < 0 ? down : up
    }

    /// Soft colour behind the panels. Glass refracts what is under it, so a
    /// flat dark fill renders as a grey rectangle — these glows are what make
    /// the effect read as glass at all.
    static var backdrop: some View {
        ZStack {
            LinearGradient(
                colors: [backgroundAccent, background],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )

            glow(Color(red: 0.25, green: 0.55, blue: 0.75), size: 760)
                .offset(x: -240, y: -260)

            glow(Color(red: 0.45, green: 0.30, blue: 0.70), size: 680)
                .offset(x: 300, y: 180)

            glow(up, size: 460)
                .offset(x: 120, y: -320)
        }
        .ignoresSafeArea()
    }

    private static func glow(_ color: Color, size: CGFloat) -> some View {
        Circle()
            .fill(
                RadialGradient(
                    colors: [color.opacity(0.38), color.opacity(0.0)],
                    center: .center,
                    startRadius: 0,
                    endRadius: size / 2
                )
            )
            .frame(width: size, height: size)
            .blur(radius: 70)
    }
}

extension View {
    /// One place to change how every panel is treated.
    func panel(cornerRadius: CGFloat = 18) -> some View {
        glassEffect(.regular, in: .rect(cornerRadius: cornerRadius))
    }
}

extension Double {
    /// Mirrors the terminal UI's variable precision: small prices need more
    /// decimals to say anything at all.
    func price(_ variable: Bool) -> String {
        let digits = variable && abs(self) < 10 ? 4 : 2

        return formatted(.number.precision(.fractionLength(digits)))
    }

    func signed(_ variable: Bool) -> String {
        (self < 0 ? "" : "+") + price(variable)
    }

    var signedPercent: String {
        (self < 0 ? "" : "+") + formatted(.number.precision(.fractionLength(2))) + "%"
    }

    /// Market cap and volume are unreadable in full.
    var abbreviated: String {
        let units: [(Double, String)] = [(1e12, "T"), (1e9, "B"), (1e6, "M"), (1e3, "K")]

        for (scale, suffix) in units where abs(self) >= scale {
            return (self / scale).formatted(.number.precision(.fractionLength(2))) + suffix
        }

        return formatted(.number.precision(.fractionLength(0)))
    }
}
