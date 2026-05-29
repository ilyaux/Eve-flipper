# Changelog

## Unreleased

- No unreleased changes yet.

## v1.6.5 - 2026-05-29

This is a maintenance release focused on stability fixes after v1.6.4.

### ESI and Private Structures

- Fixed an infinite player-structure name lookup loop when ESI returns `403 Forbidden` for private or inaccessible Upwell structures.
- Added negative caching for inaccessible structure lookups so the app does not retry the same forbidden structure repeatedly.
- Added global cooldown handling for ESI `420/429` structure-name rate limits to prevent request storms across many structure IDs.
- Limited concurrent player-structure name resolution in structure prefetch and system-structure discovery paths.
- Prefer EVERef structure names before authenticated ESI lookup when a public fallback name is already available.
- Applied the same structure lookup suppression to structure detail resolution used by private/corp structure selectors.

### Tests

- Added regression coverage for forbidden structure suppression, global rate-limit suppression, and EVERef fallback behavior.

## v1.6.4 - 2026-05-17

This release expands Eve Flipper into a configurable trading cockpit and adds several community-requested intelligence and diagnostics tools.

### Re-upload Note

- Re-uploaded the v1.6.4 release build on 2026-05-18 to include small bug fixes found after the initial release.
- Fixed light-mode shell styling where the app frame/header could stay black while the rest of the UI used the light palette.
- Fixed PI planet detail decoding when ESI returns route quantities as integer-valued decimals such as `20.0`.
- Included frontend security dependency patches for Vite and PostCSS.
- Migrated frontend tooling to Node.js `24+` and pnpm for more reproducible installs and CI builds.

### Cockpit Engine

- Added the Cockpit Interface settings panel behind the header gear button.
- Added persistent cockpit loadouts for navigation, density, visible panels, quick actions, columns, filters, and startup view.
- Added profile presets for Station Trader, Regional Hauler, Industry Builder, Ledger/Accountant, New Player, and Power User workflows.
- Added per-tab layout settings for Scanner, Regional Trade, Station Trading, Route Builder, Industry, Ledger, and related tools.
- Added import/export support for cockpit profiles as JSON.
- Added shareable cockpit packs and a remote JSON community layout gallery.
- Added built-in workspace templates as local fallback when the remote gallery is unavailable.
- Added role-aware cockpit bindings so different characters can switch to different workspaces.
- Added context/adaptive cockpit hints and quick action configuration.
- Added a compact command palette for fast navigation and common actions.

### Item Intelligence

- Added an Item Intelligence modal with item search, market depth context, history signal, owned stock, active orders, and personal trading context.
- Added reusable item intelligence links from trading tables and top-level navigation.
- Added item-level diagnostics to connect market data with personal assets, orders, and journal history.

### Regional Trade Diagnostics

- Added Regional Trade diagnostic mode for checking rejected or negative opportunities.
- Added clearer visibility into market data status, source/destination prices, margin, and filter rejection reasons.
- Improved nullsec/private-structure troubleshooting by making missing or weak destination data easier to identify.

### Ledger, Watchlist, and Character Data

- Added graph tooltips and clearer date range display for Ledger capital/cashflow/P&L charts.
- Added PI planets tab in the character popup with ESI-backed planet data and MVP production/profit context.
- Added reusable tax profile editor as a shared source for fee/tax configuration across modules.
- Improved Watchlist alert trigger handling and UI flow.

### Industry and Structures

- Improved custom/private structure resolution for Industry where ESI/ACL data is available.
- Added industry structure awareness hooks for corporation/private station workflows.
- Added additional SDE and ESI support for PI and industry-oriented item metadata.

### UI and UX

- Moved PLEX+ out of the main navigation and into the character/profile popup as secondary information.
- Added wider/fullscreen modal support for large workflows.
- Improved light-mode contrast for warning/info states.
- Standardized tabs, filters, and action layout across major workflows.
- Fixed multiple cockpit/layout overflow cases in the header, settings panels, profile presets, workspace gallery, and station trading table tools.

## v1.6.3 - 2026-05-15

This release focuses on wallet history reliability, route responsiveness, DOTLAN navigation, achievement expansion, and removing legacy desktop code.

### Wallet Archive and Ledger Reliability

- Added local wallet transaction and journal archive storage.
- Added incremental wallet sync so future rows are preserved locally once seen.
- Added archive fallback when live ESI wallet calls fail or return rate limit errors.
- Added archive coverage metadata including live rows, archived rows, coverage days, and last sync information.
- Improved Ledger graph clarity so fixed date ticks are not mistaken for missing history.

### Route and DOTLAN

- Added DOTLAN route opening support from route workflows.
- Added route history counters for DOTLAN/navigation-related achievement tracking.
- Changed hauling gank-risk scoring to capped best-effort work with timeouts and partial results so slow zKillboard responses do not block route scans.
- Added tests for route risk timeout behavior.

### Achievements

- Added advanced ledger, audit, DOTLAN, archive, and discipline achievements.
- Added new achievement glyph assets and localized EN/RU achievement text.
- Added classified/hidden achievement handling for unrevealed achievements.

### Backtest and Mission Control

- Improved Paper Backtest result diagnostics and historical snapshot replay handling.
- Improved Mission Control expected-vs-actual and journal integration details.
- Added refinements to station and route execution wording around fill assumptions and quantity constraints.

### Cleanup

- Removed the legacy Tauri shell and vendored Tauri/Rust desktop files.
- Kept the Wails desktop path as the supported desktop runtime.
- Disabled wiki RAG autostart in API tests to keep CI cleanup stable.

## v1.6.2 - 2026-05-09

This release turns the new execution workflow into a full decision-support layer: plan the trade, record it, reconcile the result, and track progress through achievements.

### Mission Control

- Added Trade Execution Autopilot / Mission Control for scanner, route, and station-trading rows.
- Added depth-aware executable quantity, gross spread, net per-unit math, fees/taxes, worst-case PnL, and quantity-reduction diagnostics.
- Added capital constraints including max ISK per trade, wallet reserve, and max item exposure.
- Added station-trading order variants for fast fill, safer spread, and max ISK/hour.
- Added route execution planning with ship profile, cargo capacity, trips, execution minutes, safety delay, and ISK/hour modes.
- Added one-click journal trade creation from execution plans.

### EveLedger and Paper Backtest

- Added EveLedger-style wallet/cashflow dashboard with income/outgoing views, journal categories, trading PnL separation, inventory mark-to-market, and capital curves.
- Improved Paper Backtest diagnostics beyond PnL, including fill assumptions, open MTM controls, instant-flip simulation, and recorded orderbook snapshot replay when local data exists.
- Added clearer expected-vs-actual reconciliation data for planned trades.

### Achievements

- Added the achievement system with persistent SQLite progress, unlock state, seen state, and event tracking.
- Added achievement library inside the character popup with categories, rarity, progress bars, locked/classified states, and EN/RU localization.
- Added animated achievement unlock toasts and reusable badge/icon components.
- Added achievement events for scans, Mission Control, journal creation, reconcile, backtests, route checks, and industry analysis.

### Fixed

- Fixed a Wails desktop startup collision where an already-running local backend on `127.0.0.1:13370` could make a release build talk to the wrong process and display `dev`.
- Wails desktop builds now use a relative API base and proxy API calls through the Wails asset server to the backend instance started by the current desktop process.
- Desktop backend startup now binds the listener before readiness checks, preserving `13370` when available and falling back to a free local port instead of accepting another process as ready.
- Fixed concurrent achievement unlock writes that could return SQLite `database is locked` during bursty UI event tracking.
- Fixed Station Trading empty-state text so it no longer looks like a scan is running before the user starts one.

## v1.6.1 - 2026-05-04

This release focuses on making Eve Flipper less optimistic on paper and more useful for real execution decisions.

### Market Scanning

- Fixed inflated profit reporting after depth and slippage calculations.
- Added stricter handling for partial or broken ESI data so bad pages are less likely to create false opportunities.
- Added execution-aware liquidity, fill-rate, fill-time, and confidence signals.
- Improved target-market restriction handling between frontend and backend.
- Added character-aware enrichment from active orders and assets in trading views.

### Route Trading

- Reworked route execution math toward deeper VWAP-style liquidity instead of only top-of-book pricing.
- Added route execution estimates for cargo trips, travel time, safety delay, ISK/hour, and route mode sorting.
- Added hauling and gank-risk signals including route danger, recent kills, and hot-zone warnings.
- Added courier/collateral risk fields for hauling-oriented route evaluation.

### Paper Backtest and Trade Journal

- Added the Paper Backtest popup with configurable hold/instant flip modes, entry cadence, volume limits, price assumptions, fees, ROI filters, and chart output.
- Added instant-flip simulation for repeated buy-haul-sell opportunities with cooldown control.
- Added orderbook snapshot storage, coverage reporting, cleanup/stats, and recorded snapshot replay support.
- Added Paper/Live Trade Journal foundation with manual entries, scanner-row drafts, live ESI drafts, reconciliation, and suggested status patches.

### Portfolio, Wallet, and Risk

- Improved realized PnL matching so unmatched sells are not treated as zero-cost profit in strict API mode.
- Added portfolio optimizer support for wallet balance, active orders, assets, exposure, and runtime warnings.
- Added wallet/cashflow dashboard foundations for income, outgoing, inventory mark-to-market, and category views.
- Fixed empty transaction handling so P&L shows an empty state instead of an error when ESI returns no transactions.

### Industry

- Improved industry analysis with depth-aware material buying and clearer sell modes.
- Added reaction and invention-oriented analysis inputs.
- Added character-aware industry coverage for owned materials and blueprints.
- Added industry project execution planning, task/job status controls, material rebalancing, blueprint sync, and coverage-aware ledger draft generation.
- Added active industry job sync from ESI into the character industry workflow.

### Updates and Release Safety

- Auto-update now requires SHA256 checksum verification before replacing the local binary.
- GitHub release workflow now publishes `SHA256SUMS.txt` for release assets.
- Added tests for checksum selection and parsing.

### Known Limits

- Historical orderbook replay only becomes meaningful after enough local snapshots have been recorded. ESI does not provide old orderbook depth retroactively.
- Route execution planning now includes core time/risk/cargo fields, but full ship-specific navigation remains an area for future tuning.
