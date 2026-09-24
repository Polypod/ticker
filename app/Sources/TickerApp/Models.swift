import Foundation

/// Payloads from `ticker serve`. Field names match the Go domain types, which
/// carry no json tags, so the keys are the Go identifiers.
struct Snapshot: Decodable {
    var assets: [Asset]
    var summary: PositionSummary
    var sources: [String: String]
    var updatedAt: Date
}

struct Asset: Decodable, Identifiable {
    var id: String { symbol }

    let name: String
    let symbol: String
    let quotePrice: QuotePrice
    let position: Position
    let exchange: Exchange

    enum CodingKeys: String, CodingKey {
        case name = "Name"
        case symbol = "Symbol"
        case quotePrice = "QuotePrice"
        case position = "Position"
        case exchange = "Exchange"
    }
}

struct QuotePrice: Decodable {
    let price: Double
    let change: Double
    let changePercent: Double

    enum CodingKeys: String, CodingKey {
        case price = "Price"
        case change = "Change"
        case changePercent = "ChangePercent"
    }
}

struct Position: Decodable {
    let value: Double
    let cost: Double
    let quantity: Double
    let weight: Double
    let totalChange: PositionChange
    let dayChange: PositionChange

    enum CodingKeys: String, CodingKey {
        case value = "Value"
        case cost = "Cost"
        case quantity = "Quantity"
        case weight = "Weight"
        case totalChange = "TotalChange"
        case dayChange = "DayChange"
    }
}

struct PositionChange: Decodable {
    let amount: Double
    let percent: Double

    enum CodingKeys: String, CodingKey {
        case amount = "Amount"
        case percent = "Percent"
    }
}

struct PositionSummary: Decodable {
    let value: Double
    let cost: Double
    let totalChange: PositionChange
    let dayChange: PositionChange

    enum CodingKeys: String, CodingKey {
        case value = "Value"
        case cost = "Cost"
        case totalChange = "TotalChange"
        case dayChange = "DayChange"
    }
}

struct Exchange: Decodable {
    let name: String
    let isActive: Bool

    enum CodingKeys: String, CodingKey {
        case name = "Name"
        case isActive = "IsActive"
    }
}

/// Written by `ticker serve` so a client can find a running daemon.
struct Discovery: Decodable {
    let url: String
    let token: String
    let pid: Int

    static var path: URL {
        URL.applicationSupportDirectory.appending(path: "ticker/serve.json")
    }

    static func load() throws -> Discovery {
        let data = try Data(contentsOf: path)

        return try JSONDecoder().decode(Discovery.self, from: data)
    }
}

extension JSONDecoder {
    /// Go marshals `time.Time` as RFC 3339 with fractional seconds.
    static var daemon: JSONDecoder {
        let decoder = JSONDecoder()

        // ISO8601FormatStyle is Sendable; ISO8601DateFormatter is not, and this
        // closure is.
        decoder.dateDecodingStrategy = .custom { decoder in
            let text = try decoder.singleValueContainer().decode(String.self)

            if let date = try? Date.ISO8601FormatStyle(includingFractionalSeconds: true).parse(text) {
                return date
            }

            // Whole-second timestamps omit the fraction entirely.
            if let date = try? Date.ISO8601FormatStyle().parse(text) {
                return date
            }

            throw DecodingError.dataCorrupted(
                .init(codingPath: decoder.codingPath, debugDescription: "unrecognized date \(text)")
            )
        }

        return decoder
    }
}
