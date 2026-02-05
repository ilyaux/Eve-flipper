# Execution Plan

The **Execution Plan** (also called *Execution Plan Calculator* or *Plan ispolneniya*) answers: *"If I buy or sell this quantity right now, at what price will I actually fill, and will the order book have enough volume?"*

It simulates "walking the book": it takes the current order book (sell orders for buying, buy orders for selling), fills your quantity level by level, and computes the **expected average price**, **slippage**, and whether you **can fill** the full amount.

## What you see

| Field | Meaning |
|-------|--------|
| **Best price** | Top of book — the best single price (lowest ask for buy, highest bid for sell). |
| **Expected price** | Volume-weighted average fill price if you market-order the full quantity. |
| **Slippage %** | How much worse than best: `(expected - best) / best × 100`. 0% means your whole order fits at the best price. |
| **Total ISK** | Total cost (buy) or revenue (sell) at the expected price. |
| **Can fill** | Whether the book has enough volume for your quantity. |
| **Book depth** | Total volume in the book (all levels). |
| **Volume at best price** | Volume available at the first (best) price level. If your quantity ≤ this, slippage is 0%. |

Profit in the calculator uses **sales tax** when you set it (e.g. in Route Planner or Station Trading): revenue from selling is reduced by the tax before subtracting buy cost.

## Where it appears

- **Route Planner** — In route details, the 📊 button opens the Execution Plan for that hop (buy/sell for the chosen quantity; sales tax from scan params is applied).
- **Station Trading** — The 📊 button in the table opens the **Station Execution Calculator**: limit orders at bid/ask, broker fee and sales tax, fill curve, and (when history is available) impact estimates (λ, η, n*).
- **Industry** — From the shopping list, 📊 opens the Execution Plan for that material (quantity from the list; sales tax from the Industry "After broker" setting).
- **Radius / Region scan** — Execution Plan can be opened from result rows to check fill and slippage for a given item and quantity.

## Why slippage is 0%

If your quantity is small relative to the first level of the order book, the whole order fills at the **best price**, so **expected price = best price** and **slippage = 0%**. The **Volume at best price** field shows how much is available at that first level — if your quantity is less than or equal to it, 0% slippage is correct.

## Advanced: impact from history

When market history is available (e.g. in the Station Execution Calculator), the plan can show:

- **Kyle's λ** — Linear price impact (ΔP ∝ λ×Q).
- **η (sqrt impact)** — Price move for large volume (ΔP ∝ η×√Q).
- **n* (TWAP)** — Suggested number of orders to split volume over time to reduce market impact.

These help you judge how much a large order might move the price and whether to slice it.

## Summary

- **Execution Plan** = order-book simulation: expected price, slippage, can-fill, and (in station view) broker/tax and optional impact metrics.
- Use it to check **realistic fill price** and **profit after fees** before committing to a trade or route.

---

[← Back to Home](/ilyaux/Eve-flipper/wiki)
