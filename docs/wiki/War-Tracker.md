# War Tracker

**War Tracker** helps you find trading opportunities in regions with high PvP activity. It uses [zkillboard](https://zkillboard.com) kill data to identify "hot" regions (wars, conflicts, elevated activity) and shows what items are in demand there compared to Jita prices.

## What it does

- **Hot zones** — Regions ranked by kill activity (kills/day, ISK destroyed, active players). Status: **War** 🔥, **Conflict** ⚠️, **Elevated** 📈, **Normal** ✅.
- **Demand data** — Uses zkillboard to estimate which ships, modules, and ammo are being lost (and thus needed) in each region.
- **Region opportunities** — For a selected region, shows trade opportunities: Jita price vs region price, profit per unit, profit %, and estimated daily demand (ships, modules, ammo).
- **Target region** — You can set a region as the "target" for [Region Arbitrage](/ilyaux/Eve-flipper/wiki/Region-Arbitrage) from the War Tracker popup.

## How to use

1. Open the **War Tracker** tab.
2. Wait for data to load (or click **Refresh Data** to fetch fresh data from zkillboard). Data may be cached for performance.
3. Browse **War Zones**, **Conflicts**, **Elevated**, and **Normal** activity regions. Each card shows:
   - Region name, kills/day, hot score (vs average)
   - Active players, ISK destroyed
   - Top lost ship types
4. **Click a region** to open the detail popup:
   - **Ships** — Popular lost ships and Jita vs region price, profit %, daily volume.
   - **Modules** — Common PvP modules (tank, tackle, propulsion, etc.) with same metrics.
   - **Ammo** — Common ammo types in demand.
5. Use **"Open Region Arbitrage"** to jump to the Region Scan tab with this region as the target.

## Tips

- War and conflict zones often have higher sell prices for meta/PvP items; buying in Jita and selling there can be profitable.
- **Daily profit** is an estimate based on kill volume and prices; use it as a guide, not a guarantee.
- Combine with [Route Trading](/ilyaux/Eve-flipper/wiki/Route-Trading) to plan multi-hop hauls into war zones.

## Data source

- Kill and loss data: **zkillboard** (EVE killboards).
- Market prices: **ESI** (Jita and target region).

---

[← Back to Home](/ilyaux/Eve-flipper/wiki)
