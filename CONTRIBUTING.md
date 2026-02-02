# Contributing to EVE Flipper

Thanks for your interest in contributing! This project is maintained by **[@ilyaux](https://github.com/ilyaux)** (in-game: **LunarLiight**).

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/<your-username>/Eve-flipper.git
   cd Eve-flipper
   ```
3. Start the backend:
   ```bash
   go run main.go
   ```
4. Start the frontend:
   ```bash
   cd frontend && npm install && npm run dev
   ```

## Development Workflow

1. Create a feature branch from `main`:
   ```bash
   git checkout -b feature/your-feature
   ```
2. Make your changes
3. Run tests:
   ```bash
   make test
   ```
4. Commit with a clear message:
   ```bash
   git commit -m "Add: short description of what changed"
   ```
5. Push and open a Pull Request

## Code Style

### Go (Backend)

- Follow standard `gofmt` formatting
- Add godoc comments to all exported types and functions
- Keep packages focused: `engine/` for business logic, `esi/` for API calls, `db/` for persistence
- Use constants instead of magic numbers
- Run `go vet ./...` before committing

### TypeScript / React (Frontend)

- Use functional components with hooks
- Keep API calls in `lib/api.ts`, types in `lib/types.ts`
- All user-facing strings go through the `i18n` system (English + Russian)
- Use Tailwind CSS utility classes, follow the EVE-themed color scheme (`eve-*` tokens)

## Project Structure

```
internal/
  api/      HTTP handlers, CORS, NDJSON streaming
  config/   Config and watchlist structs
  db/       SQLite persistence (WAL mode)
  engine/   Scanner, route builder, profit calculations
  esi/      ESI HTTP client with rate limiting and caching
  graph/    Dijkstra, BFS, universe graph
  sde/      Static Data Export downloader and parser

frontend/src/
  components/   React UI components
  lib/          API client, types, i18n, formatting utilities
```

## Adding a New Scan Type

1. Add the scan function to `internal/engine/`
2. Add the API handler to `internal/api/server.go`
3. Add the frontend tab component in `frontend/src/components/`
4. Add i18n keys to `frontend/src/lib/i18n.tsx`
5. Write tests in `*_test.go`

## Reporting Issues

- Use [GitHub Issues](https://github.com/ilyaux/Eve-flipper/issues)
- Include steps to reproduce, expected vs actual behavior
- For EVE-specific issues, mention the system/region and approximate time (for ESI data context)

## Contact

- GitHub: [@ilyaux](https://github.com/ilyaux)
- EVE Online: **LunarLiight**

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
