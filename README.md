<div align="center">
  <img src="assets/logo.svg" width="96" alt="EVE Flipper logo" />

  <h1>EVE Flipper</h1>

  <p>
    Local-first market intelligence, execution planning, and portfolio tooling for EVE Online traders.
  </p>

  <p>
    <a href="https://github.com/ilyaux/Eve-flipper/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/ilyaux/Eve-flipper?label=release"></a>
    <a href="https://github.com/ilyaux/Eve-flipper/releases"><img alt="Downloads" src="https://img.shields.io/github/downloads/ilyaux/Eve-flipper/total"></a>
    <a href="https://go.dev/"><img alt="Go" src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white"></a>
    <a href="https://react.dev/"><img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black"></a>
    <a href="https://www.typescriptlang.org/"><img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white"></a>
    <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/License-MIT-green"></a>
    <a href="https://discord.gg/rnR2bw6XXX"><img alt="Discord" src="https://img.shields.io/badge/Discord-Join%20Server-5865F2?logo=discord&logoColor=white"></a>
  </p>
</div>

## Overview

EVE Flipper helps traders decide whether an opportunity is actually executable, not just mathematically attractive on top-of-book prices. It combines ESI market data, local history, orderbook depth, liquidity scoring, route risk, character data, and paper/live trade workflows in one local application.

The app is built for practical EVE trading:

- Station trading and same-hub flipping.
- Regional hauling and route opportunity scanning.
- Contract arbitrage and liquidation checks.
- Industry build-vs-buy analysis, reactions, invention, and project tracking.
- Portfolio, wallet, PnL, cashflow, active orders, assets, and risk views.
- Paper backtesting, orderbook snapshot recording, and trade journaling.

Everything runs locally. There is no hosted service and no central database.

## Screenshots

| Station Trading | Route Trading | Radius Scanner |
|---|---|---|
| ![Station Trading](assets/screenshot-station.png) | ![Route Trading](assets/screenshot-routes.png) | ![Radius Scan](assets/screenshot-radius.png) |

## Current Public Release

The current public release is `v1.6.1`.

Download it from:

- [GitHub Releases](https://github.com/ilyaux/Eve-flipper/releases/latest)

Release packages are published as two runtime families:

| Runtime | Assets | Use When |
|---|---|---|
| Desktop app | `eve-flipper-desktop-windows-amd64.exe`, `eve-flipper-desktop-linux-*`, `eve-flipper-desktop-darwin-*` | You want the normal app window with the embedded backend. Recommended for most users. |
| Web/server binary | `eve-flipper-web-windows-amd64.exe`, `eve-flipper-web-linux-*`, `eve-flipper-web-darwin-*` | You want to run the local backend and open the UI in a browser. |
| Checksums | `SHA256SUMS.txt` | Used by the updater and for manual release verification. |

## Main Modules

| Module | What It Does |
|---|---|
| Flipper (Radius) | Finds local buy/sell opportunities around a source system using depth-aware profit, slippage, liquidity, and fillability math. |
| Station Trading | Same-station market scanner for hub trading with advanced filters, active order context, history, and paper trade actions. |
| Regional Trade | Cross-region scanner for hauling and market spread discovery. |
| Route | Builds route opportunities with execution estimates, cargo trips, travel time, ISK/hour, liquidity, and gank-risk signals. |
| Contract Arbitrage | Evaluates contracts, courier risk, collateral issues, liquidation assumptions, and suspicious pricing. |
| Paper Backtest | Simulates hold and instant-flip strategies with configurable entry cadence, volume limits, price assumptions, ROI filters, fees, and equity charts. |
| Trade Journal | Tracks manual and scanner-created paper/live trade records, live drafts from ESI, reconciliation, and suggested status updates. |
| Portfolio and Risk | Calculates wallet, assets, active orders, exposure, PnL, optimizer diagnostics, and inventory-aware capital usage. |
| Industry | Performs build-vs-buy analysis, material depth checks, sell-mode comparison, reactions, invention, project planning, blueprints, jobs, and ledger coverage. |
| Wallet/Cashflow | Provides EveLedger-style foundations for income, outgoing, inventory mark-to-market, category views, and capital tracking. |
| PLEX+ | Tracks PLEX-oriented market analytics and profitability dashboards. |
| War/Demand Tracker | Surfaces region activity, demand hot zones, and opportunity context. |

## Execution-Aware Trading

The project intentionally avoids the most common market-tool trap: treating the first buy/sell order as if the whole position can trade there.

Current execution logic includes:

- VWAP-style depth walking for market scanner and route calculations.
- Slippage and safe quantity calculation.
- Real profit fields after depth and fees.
- Liquidity, fill-rate, fill-time, turnover, and confidence signals.
- Daily volume and history-aware filters.
- Active character orders/assets context where available.
- Route cargo trips, execution minutes, safety delay, and ISK/hour.
- Gank-risk and hot-zone indicators for hauling.
- Courier and collateral risk signals.

## Backtesting and Orderbook Replay

The Paper Backtest module supports two practical modes:

- Hold mode: buy, hold for a configured period, then exit using historical assumptions.
- Instant flip mode: simulate repeated buy-haul-sell cycles when opportunities appear again after cooldown.

The app can also record orderbook snapshots locally and replay recorded coverage. ESI does not provide historical orderbook depth retroactively, so real historical orderbook replay becomes useful only after you have accumulated your own snapshots.

## Character-Aware Workflows

EVE SSO is optional, but login unlocks deeper workflows:

- Character wallet, transactions, journal, orders, assets, location, skills, and blueprints.
- Structure market access where your character has access.
- Live trade journal drafts and reconciliation.
- Portfolio optimizer using wallet, inventory, and active orders.
- Industry coverage against owned materials and BPO/BPCs.
- Active industry job sync.
- EVE UI actions such as open market and set waypoint.

## Quick Start

### Desktop Release

1. Download the desktop asset for your OS from [latest release](https://github.com/ilyaux/Eve-flipper/releases/latest).
2. Run the binary.
3. Add a character if you want SSO-backed features, or use public-market scanners without login.

### Web/Server Release

Windows:

```powershell
.\eve-flipper-web-windows-amd64.exe
```

Linux/macOS:

```bash
chmod +x ./eve-flipper-web-linux-amd64
./eve-flipper-web-linux-amd64
```

Then open:

```text
http://127.0.0.1:13370
```

### Build From Source

Prerequisites:

- [mise](https://mise.jdx.dev/)

```bash
git clone https://github.com/ilyaux/Eve-flipper.git
cd Eve-flipper
mise install
mise run setup
mise run build
mise run run
```

## Developer Commands

Help:

```bash
mise run help
```

Start backend and frontend together:

```bash
mise run dev
```

Start only the backend:

```bash
mise run dev:backend
```

Start only the frontend:

```bash
mise run dev:frontend
```

Frontend builds:

```bash
mise run frontend
mise run frontend-wails
```

Production builds:

```bash
mise run build
mise run run
mise run wails
mise run wails-run
mise run cross
```

Formatting, linting, and verification:

```bash
mise run format
mise run format:check
mise run vet
mise run lint
mise run check
```

## Runtime Configuration

The web/server binary listens on localhost by default:

```bash
./eve-flipper-web-linux-amd64 --host 127.0.0.1 --port 13370
```

| Flag | Default | Description |
|---|---:|---|
| `--host` | `127.0.0.1` | Bind address. Use `0.0.0.0` only if you know how to secure the host. |
| `--port` | `13370` | HTTP port for the local web UI and API. |

Desktop builds start their own local backend internally. If `13370` is already busy, the desktop app can use a free local port and route API calls through the Wails asset server.

## EVE SSO Setup for Source Builds

Official release builds are configured through GitHub release secrets. For local source builds, create `.env` in the repository root:

```env
ESI_CLIENT_ID=your-client-id
ESI_CLIENT_SECRET=your-client-secret
ESI_CALLBACK_URL=http://localhost:13370/api/auth/callback
```

Do not commit `.env`.

Useful scopes include market orders, wallet, assets, skills, blueprints, industry jobs, structures, corporation data, and EVE UI actions. The app requests the scopes required by its character-aware modules.

## Data and Privacy

- SQLite stores local config, history, snapshots, journal records, projects, and cached state.
- ESI tokens are stored locally.
- Public market scans can run without EVE login.
- No project-operated cloud backend receives your trading data.

## Release Safety

- Release binaries are built by GitHub Actions from tags.
- Release assets include `SHA256SUMS.txt`.
- The auto-updater verifies the downloaded asset checksum before replacing the local binary.
- Windows release builds include version metadata and app icon resources.

## Known Limits

- ESI does not provide old orderbook depth. Historical orderbook replay requires locally recorded snapshots.
- Market data can move between scan and execution. Always check volume, fees, taxes, standings, skills, and order depth before committing serious ISK.
- Gank-risk and route scoring are decision support, not safety guarantees.
- Industry and portfolio tools are models. They improve discipline, but they do not replace manual review of jobs, orders, assets, and market conditions.

## Tests

```bash
mise run test
mise run test:wails
mise run frontend
mise run frontend-wails
```

Notes:

- The backend has a standard colocated Go test suite under `internal/**` using `*_test.go` files.
- The frontend currently has no dedicated test suite. The main frontend quality gates today are build, formatting, and lint checks.

## Documentation and Community

- [Project wiki](https://github.com/ilyaux/Eve-flipper/wiki)
- [Getting Started](https://github.com/ilyaux/Eve-flipper/wiki/Getting-Started)
- [API Reference](https://github.com/ilyaux/Eve-flipper/wiki/API-Reference)
- [Discord](https://discord.gg/rnR2bw6XXX)

## Contributing

Issues, pull requests, bug reports, screenshots, and trading workflow feedback are welcome. For larger changes, describe the trading scenario and the expected behavior clearly so it can be tested against real EVE market conditions.

## License

MIT License. See [LICENSE](LICENSE).

## Disclaimer

EVE Flipper is an independent third-party project and is not affiliated with CCP Games. EVE Online and related trademarks are property of CCP hf.
