import Foundation
import Observation

/// Connects to a running `ticker serve`, holds the latest snapshot, and keeps
/// the SSE stream alive.
///
/// It does not launch the daemon. Once the app ships as a real bundle it will
/// spawn the binary it carries and fall back to this path; for now start it
/// yourself with `ticker serve`.
@MainActor
@Observable
final class DaemonClient {
    enum State: Equatable {
        case searching
        case connected
        case failed(String)
    }

    private(set) var state: State = .searching
    private(set) var snapshot: Snapshot?
    /// When the last event arrived, which is what staleness is measured from —
    /// the daemon can be connected but quiet when a market is closed.
    private(set) var lastEvent: Date?

    private var task: Task<Void, Never>?

    func start() {
        guard task == nil else { return }

        task = Task { [weak self] in
            while !Task.isCancelled {
                await self?.connect()

                // The daemon may not be running yet; retry rather than dying.
                try? await Task.sleep(for: .seconds(2))
            }
        }
    }

    func stop() {
        task?.cancel()
        task = nil
    }

    private func connect() async {
        let discovery: Discovery

        do {
            discovery = try Discovery.load()
        } catch {
            state = .failed("No daemon found. Run `ticker serve`.")

            return
        }

        guard let base = URL(string: discovery.url) else {
            state = .failed("Unreadable daemon URL: \(discovery.url)")

            return
        }

        do {
            try await stream(base: base, token: discovery.token)
        } catch is CancellationError {
            return
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    private func stream(base: URL, token: String) async throws {
        var request = URLRequest(url: base.appending(path: "stream"))
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.timeoutInterval = .infinity

        let (bytes, response) = try await URLSession.shared.bytes(for: request)

        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? 0
            throw DaemonError.badStatus(code)
        }

        state = .connected

        for try await line in bytes.lines {
            guard line.hasPrefix("data: ") else { continue }

            let payload = Data(line.dropFirst(6).utf8)

            guard let decoded = try? JSONDecoder.daemon.decode(Snapshot.self, from: payload) else {
                continue
            }

            snapshot = decoded
            lastEvent = Date()
        }
    }
}

enum DaemonError: LocalizedError {
    case badStatus(Int)

    var errorDescription: String? {
        switch self {
        case let .badStatus(code) where code == 401:
            return "Daemon rejected the token in serve.json."
        case let .badStatus(code):
            return "Daemon returned HTTP \(code)."
        }
    }
}
