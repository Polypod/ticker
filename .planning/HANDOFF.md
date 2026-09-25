# Handoff — macOS app on top of `ticker`

Last updated 2026-09-25. Everything below is on `master`, working tree clean.

## What we're building

A macOS SwiftUI client for this ticker, styled after a dark glassmorphism
concept, fed by the existing Go monitor rather than a Swift rewrite. An iPhone
web UI is *designed for* but not built.

## Decisions already settled (don't relitigate)

| Decision | Choice |
|---|---|
| Audience | Personal, local Xcode build. TestFlight is backlog 999.2 |
| Platform | macOS first; one multiplatform target later |
| OS floor | Current OS, Liquid Glass, dark-only |
| Architecture | Go daemon (`ticker serve`) + thin Swift client |
| Why not a rewrite | IBKR, the AI brief and three monitors all stay in Go |
| Why in this repo | Everything is under `internal/`; no other module can import it |
| Transport | HTTP on loopback, SSE for pushes, bearer token from day one |
| Daemon lifecycle | App will spawn the bundled binary; today you start it yourself |
| Keys | Keychain, injected as env vars when spawning. Never plaintext on disk |
| Watchlist storage | `~/.ticker.yaml`, shared with the CLI. Comment loss accepted |
| Quote sources | Tiingo (streaming) primary, IB `MarketPrice` fallback (~3 min, not realtime) |
| Yahoo | Out for quotes, kept for FX rates only |
| No Tiingo key | Degraded mode: IB positions only, watchlist greyed. No Yahoo fallback |
| IB positions | Read-only live mirror. Writing lots to yaml is backlog 999.1 |
| Provenance | Always visible per row — mixing a stream tick with a 3-min price silently is how you lose money |

## What exists

**`ticker serve`** (`internal/server/`, `cmd/root.go`)

- `GET /quotes` — assets, position summary, per-symbol source
- `GET /stream` — SSE, full snapshot on connect and on every update
- Bearer token, also accepted as `?token=` because `EventSource` can't set headers
- URL + token written to `serve.json` in the state directory, 0600, removed on shutdown
- Each stream client has a latest-wins slot, so a slow reader coalesces instead of blocking the monitor
- Tests: `internal/server/server_test.go` (auth, payload, SSE push)

**`app/`** — SwiftPM executable, not an `.xcodeproj`. `swift run`, or open `Package.swift` in Xcode.

- `DaemonClient` — reads `serve.json`, holds the SSE stream, retries every 2s
- `ContentView` — sidebar + rows matching the TUI's data density, flash on price change
- `Theme` — dark palette, one `panel()` modifier, backdrop glows

## Run it

```sh
ticker serve --address 127.0.0.1:8899 --token devtoken   # terminal 1
cd app && swift run                                       # terminal 2
```

## Open items, roughly in priority order

1. **Formatting is mediocre.** Fixed column widths are a blunt instrument. Needs
   a real pass: alignment, density, and whether the stat strips should drop
   columns progressively (vol → mcap → 52wk) instead of forcing a 1120pt minimum
   window width.
2. **Long lists untested.** The rows are already in a `ScrollView { LazyVStack }`,
   so they scroll; nobody has run it with more than three symbols, so row height,
   the pinned summary bar and scroll performance are unverified.
3. **Numbers follow system locale** (`2 792,80`) while the TUI prints `2792.80`.
   Decide which one the app should match.
4. **No mutations.** `POST` endpoints for watchlist editing were deliberately
   deferred until the Swift settings screen needs them.
5. **Settings screen** — API key entry into Keychain, source picker per ticker.
6. **Daemon spawning** — needs a real `.app` bundle.
7. **AI brief and IBKR panels** — daemon serves `ctx.Groups[0]` only today.

## Note for whoever picks this up

Screen capture and the accessibility API are denied to the agent's terminal, so
the assistant cannot see the app it is editing. Every visual change this session
was verified by the user sending a screenshot. If you're continuing with an
agent, either grant Screen Recording to the terminal or expect that loop.
