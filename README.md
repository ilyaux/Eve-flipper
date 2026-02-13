# EVE Flipper

EVE Flipper is a local-first market analysis platform for EVE Online traders.  
It combines real-time ESI data, historical market behavior, and execution-aware math to surface actionable opportunities across station trading, regional arbitrage, contracts, routes, industry, and PLEX.

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)
[![Release](https://img.shields.io/github/v/release/ilyaux/Eve-flipper)](https://github.com/ilyaux/Eve-flipper/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/ilyaux/Eve-flipper/total)](https://github.com/ilyaux/Eve-flipper/releases)
[![Last Commit](https://img.shields.io/github/last-commit/ilyaux/Eve-flipper)](https://github.com/ilyaux/Eve-flipper/commits/master)
[![Discord](https://img.shields.io/badge/Discord-Join%20Server-5865F2?logo=discord&logoColor=white)](https://discord.com/invite/Z9pXSGcJZE)

## Core Capabilities

### Trading Scanners
- `Radius Scan`: local buy/sell opportunities within jump constraints.
- `Region Arbitrage`: cross-region spreads and hauling candidates.
- `Route Trading`: multi-hop route search with cross-region support.
- `Station Trading`: same-station opportunities with liquidity and risk metrics.
- `Contract Scanner`: contract arbitrage in two modes:
  - `Instant liquidation` (buy now, liquidate now)
  - `Horizon mode` (expected profit with hold days and confidence target)

### Execution and Risk
- `Execution Plan`: order-book walk simulation (expected price, slippage, fillability).
- Correct partial-fill accounting (`total_isk` reflects fillable quantity when full fill is impossible).
- Scam/risk signals for trade quality filtering.

### PLEX and Industry
- `PLEX Dashboard`: arbitrage paths, SP-farm math, depth, indicators, cross-hub comparison.
- Hardened PLEX backend flow: in-flight request deduplication and stale-cache fallback during ESI instability.
- `Industry Chain Optimizer`: buy-vs-build decomposition with material tree and system-aware costs.

### Character and Portfolio
- EVE SSO integration for wallet/orders/transactions/structures.
- Portfolio analytics and optimization modules.
- Undercut monitoring and station-level context.

## Screenshots

| Station Trading | Route Trading | Radius Scan |
|---|---|---|
| ![Station Trading](assets/screenshot-station.png) | ![Route Trading](assets/screenshot-routes.png) | ![Radius Scan](assets/screenshot-radius.png) |

## Architecture

- Backend: `Go` (`net/http`), SQLite persistence, ESI client with caching/rate-limiting.
- Frontend: `React + TypeScript + Vite`.
- Distribution model: single backend binary with embedded frontend assets.
- Default runtime: local bind (`127.0.0.1:13370`).

## Quick Start

### Option 1: Release binaries

Download the latest build from:
- https://github.com/ilyaux/Eve-flipper/releases

Run the binary and open:
- `http://127.0.0.1:13370`

### Option 2: Build from source

Prerequisites:
- Go `1.25+`
- Node.js `20+`
- npm

```bash
git clone https://github.com/ilyaux/Eve-flipper.git
cd Eve-flipper
npm -C frontend install
npm -C frontend run build
go build -o build/eve-flipper .
./build/eve-flipper
```

Windows PowerShell helpers:

```powershell
.\make.ps1 build
.\make.ps1 run
```

Unix Make targets:

```bash
make build
make run
```

## Runtime Flags

```bash
./eve-flipper --host 127.0.0.1 --port 13370
```

| Flag | Default | Description |
|------|---------|-------------|
| `--host` | `127.0.0.1` | Bind address (`0.0.0.0` for LAN/remote access) |
| `--port` | `13370` | HTTP port |

## Local SSO Setup (for source builds)

SSO is disabled unless credentials are provided.

Create `.env` in repo root:

```env
ESI_CLIENT_ID=your-client-id
ESI_CLIENT_SECRET=your-client-secret
ESI_CALLBACK_URL=http://localhost:13370/api/auth/callback
```

Do not commit `.env`.

## Development Workflow

Backend:

```bash
go run .
```

Frontend dev server:

```bash
npm -C frontend install
npm -C frontend run dev
```

Tests:

```bash
go test ./...
```

Production frontend build check:

```bash
npm -C frontend run build
```

## Documentation

- Project wiki: https://github.com/ilyaux/Eve-flipper/wiki
- Getting Started: https://github.com/ilyaux/Eve-flipper/wiki/Getting-Started
- API Reference: https://github.com/ilyaux/Eve-flipper/wiki/API-Reference
- Station Trading: https://github.com/ilyaux/Eve-flipper/wiki/Station-Trading
- Contract Scanner: https://github.com/ilyaux/Eve-flipper/wiki/Contract-Scanner
- Execution Plan: https://github.com/ilyaux/Eve-flipper/wiki/Execution-Plan
- PLEX Dashboard: https://github.com/ilyaux/Eve-flipper/wiki/PLEX-Dashboard

## Security Notes

- By default, the server listens only on localhost.
- ESI credentials are never required for non-SSO features.
- If exposed beyond localhost (`--host 0.0.0.0`), use your own network hardening (firewall/reverse proxy/TLS).

## Contributing

See:
- `CONTRIBUTING.md`

## License

MIT License. See `LICENSE`.

## Disclaimer

EVE Flipper is an independent third-party project and is not affiliated with CCP Games.  
EVE Online and related trademarks are property of CCP hf.
