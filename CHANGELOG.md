# Changelog

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
