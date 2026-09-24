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

    /// The layered backdrop the glass reads against. Glass over a flat fill
    /// looks like a grey rectangle; it needs something to refract.
    static var backdrop: some View {
        LinearGradient(
            colors: [backgroundAccent, background],
            startPoint: .topLeading,
            endPoint: .bottomTrailing
        )
        .ignoresSafeArea()
    }
}

extension View {
    /// One place to change how every panel is treated.
    func panel(cornerRadius: CGFloat = 18) -> some View {
        glassEffect(.regular, in: .rect(cornerRadius: cornerRadius))
    }
}

extension Double {
    var currency: String {
        formatted(.number.precision(.fractionLength(2)))
    }

    var signedPercent: String {
        (self < 0 ? "" : "+") + formatted(.number.precision(.fractionLength(2))) + "%"
    }
}
