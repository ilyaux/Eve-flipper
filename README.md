<p align="center">
  <h1 align="center">EVE Flipper</h1>
  <p align="center">
    Real-time market arbitrage scanner for EVE Online
  </p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go" />
  <img src="https://img.shields.io/badge/React-18-61DAFB?logo=react" />
  <img src="https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript" />
  <img src="https://img.shields.io/badge/SQLite-WAL-003B57?logo=sqlite" />
  <img src="https://img.shields.io/badge/license-MIT-green" />
</p>

---

EVE Flipper finds profitable station-trading and hauling opportunities by analyzing real-time market data from the [EVE Swagger Interface (ESI)](https://esi.evetech.net/ui/). It supports radius-based flips, cross-region arbitrage, public contract analysis, and multi-hop trade route optimization.

Ships as a **single executable** &mdash; frontend is embedded into the Go binary. No installer, no external dependencies at runtime.

## Features

- **Radius Scan** &mdash; find buy-low / sell-high flips within a configurable jump radius
- **Region Scan** &mdash; cross-region arbitrage across entire regions
- **Contract Scanner** &mdash; evaluate public item-exchange contracts vs market value, with scam detection
- **Route Builder** &mdash; multi-hop trade routes via beam search (configurable hops, profit-per-jump ranking)
- **Watchlist** &mdash; track favorite items with custom margin alerts
- **Persistent Storage** &mdash; SQLite (WAL mode) for config, watchlist, scan history, and station cache
- **Live Progress** &mdash; NDJSON streaming for real-time scan feedback
- **Multi-language UI** &mdash; English / Russian
- **Single Binary** &mdash; frontend embedded via `go:embed`, one file to run everything

## Architecture

```
┌──────────────────────────────────────────┐
│           Single Binary (go:embed)       │
│                                          │
│  ┌────────────────────────────────────┐  │
│  │  Embedded React SPA (frontend/dist)│  │
│  └──────────────┬─────────────────────┘  │
│                 │ /api/* → API handler   │
│                 │ /*     → static files  │
│  ┌──────────────▼─────────────────────┐  │
│  │  Go HTTP Server (:13370)           │  │
│  │  ┌──────────┐  ┌────────────────┐  │  │
│  │  │ Scanner  │  │ Route Builder  │  │  │
│  │  └────┬─────┘  └───────┬────────┘  │  │
│  │       │                │           │  │
│  │  ┌────▼────────────────▼────────┐  │  │
│  │  │  ESI Client (rate-limited)   │  │  │
│  │  └──────────────┬───────────────┘  │  │
│  │  ┌──────────────▼───────────────┐  │  │
│  │  │  SQLite (WAL) + SDE Cache    │  │  │
│  │  └──────────────────────────────┘  │  │
│  └────────────────────────────────────┘  │
└──────────────────────────────────────────┘
```

## Screenshots

> Add screenshots to `assets/` and uncomment below:
>
> ```
> ![Radius Scan](assets/screenshot-radius.png)
> ![Route Builder](assets/screenshot-routes.png)
> ```

## Download

Grab the latest release for your platform from the [Releases](https://github.com/ilyaux/Eve-flipper/releases) page:

| Platform | Binary |
|----------|--------|
| Windows (x64) | `eve-flipper-windows-amd64.exe` |
| Linux (x64) | `eve-flipper-linux-amd64` |
| Linux (ARM64) | `eve-flipper-linux-arm64` |
| macOS (Intel) | `eve-flipper-darwin-amd64` |
| macOS (Apple Silicon) | `eve-flipper-darwin-arm64` |

Run the binary and open [http://127.0.0.1:13370](http://127.0.0.1:13370) in your browser. No installer needed.

## Building from Source

### Prerequisites

| Tool | Version |
|------|---------|
| [Go](https://go.dev/dl/) | 1.25+ |
| [Node.js](https://nodejs.org/) | 20+ |

No CGO required &mdash; SQLite uses [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go).

### Clone

```bash
git clone https://github.com/ilyaux/Eve-flipper.git
cd Eve-flipper
```

### Build (single binary with embedded frontend)

**Windows (PowerShell):**

```powershell
.\make.ps1 build       # frontend + backend → build/eve-flipper.exe
.\make.ps1 run         # build and run immediately
.\make.ps1 test        # run Go tests
.\make.ps1 cross       # cross-compile for all platforms
.\make.ps1 clean       # remove build artifacts
```

**Linux / macOS:**

```bash
make build       # frontend + backend → build/eve-flipper
make run         # build and run immediately
make test        # run Go tests
make cross       # cross-compile for all platforms
make clean       # remove build artifacts
```

Output goes to `build/`. Each binary is a standalone single-file executable with the frontend embedded inside.

### Development Mode (hot-reload)

For frontend development with hot-reload, run the backend and frontend separately:

```bash
# Terminal 1: backend
go run main.go

# Terminal 2: frontend (dev server with hot-reload)
cd frontend
npm install
VITE_API_URL=http://127.0.0.1:13370 npm run dev
```

Open [http://localhost:1420](http://localhost:1420) for the dev server.

## Configuration

### Port

The server listens on `127.0.0.1:13370` by default:

```bash
eve-flipper --port 8080
```

### Frontend API URL (dev mode only)

When running the frontend dev server separately, set the backend URL:

```bash
VITE_API_URL=http://127.0.0.1:13370 npm run dev
```

Or create `frontend/.env`:

```env
VITE_API_URL=http://127.0.0.1:13370
```

This is not needed for production builds &mdash; the frontend is embedded and served from the same origin.

### SQLite Database

All data is stored in `flipper.db` in the working directory. The database uses WAL mode for concurrent reads during scans. On first run, if a legacy `config.json` exists, it is automatically migrated to SQLite and renamed to `config.json.bak`.

## Project Structure

```
Eve-flipper/
├── main.go                   # Entry point, embeds frontend, serves API + SPA
├── Makefile                  # Build tasks (Linux/macOS)
├── make.ps1                  # Build tasks (Windows PowerShell)
├── internal/
│   ├── api/                  # HTTP handlers, CORS, NDJSON streaming
│   ├── config/               # Config & watchlist structs
│   ├── db/                   # SQLite persistence layer
│   ├── engine/               # Scanner, route builder, profit math
│   ├── esi/                  # ESI HTTP client, rate limiting, caching
│   ├── graph/                # Dijkstra, BFS, universe topology
│   └── sde/                  # SDE downloader & JSONL parser
├── frontend/
│   ├── src/
│   │   ├── components/       # React UI components
│   │   ├── lib/              # API client, types, i18n, formatting
│   │   └── App.tsx           # Root component with tab layout
│   ├── dist/                 # Built frontend (embedded into binary)
│   └── vite.config.ts
├── data/                     # SDE cache (auto-downloaded at first run)
└── flipper.db                # SQLite database (auto-created)
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/status` | Backend health & readiness |
| `GET` | `/api/config` | Current configuration |
| `POST` | `/api/config` | Update configuration |
| `GET` | `/api/systems/autocomplete?q=` | System name autocomplete |
| `POST` | `/api/scan` | Radius flip scan (NDJSON stream) |
| `POST` | `/api/scan/multi-region` | Cross-region scan (NDJSON stream) |
| `POST` | `/api/scan/contracts` | Contract arbitrage scan (NDJSON stream) |
| `POST` | `/api/route/find` | Multi-hop route search (NDJSON stream) |
| `GET` | `/api/watchlist` | Get watchlist items |
| `POST` | `/api/watchlist` | Add item to watchlist |
| `PUT` | `/api/watchlist/{typeID}` | Update alert threshold |
| `DELETE` | `/api/watchlist/{typeID}` | Remove from watchlist |
| `GET` | `/api/scan/history` | Recent scan history |

## Testing

```bash
go test ./...
```

## Releases

Releases are automated via GitHub Actions. To create a new release:

```bash
git tag v1.0.0
git push --tags
```

This triggers the [release workflow](.github/workflows/release.yml), which cross-compiles binaries for all platforms and publishes them on the [Releases](https://github.com/ilyaux/Eve-flipper/releases) page.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

[MIT](LICENSE)

## Disclaimer

EVE Flipper is a third-party tool and is not affiliated with or endorsed by CCP Games. EVE Online and all related trademarks are property of CCP hf. Market data is sourced from the public [EVE Swagger Interface](https://esi.evetech.net/).
