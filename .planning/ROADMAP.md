# Roadmap

## Backlog

### Phase 999.1: IBKR Position Synchronization Adapter (BACKLOG)

**Goal:** Synchronize current Interactive Brokers positions through an independent, read-only adapter after the core web application is complete.
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] Define the supported IBKR connection method, authentication lifecycle, snapshot semantics, failure handling, and reconciliation with manual lots (promote with `/gsd-review-backlog` when ready)

### Phase 999.2: TestFlight / Notarized Distribution for the Desktop App (BACKLOG)

**Goal:** Distribute the macOS app beyond a local Xcode build once it is worth installing elsewhere.
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] Sandbox the app, sign the bundled `ticker serve` helper with the same team ID and `com.apple.security.inherit`, enable the hardened runtime, notarize, and set up TestFlight (promote with `/gsd-review-backlog` when ready)
