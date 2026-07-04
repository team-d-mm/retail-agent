# Hackathon Win Kit — Design

**Date:** 2026-07-04
**Goal:** The smallest set of features that make `retail-agent` win a GenAI hackathon.
**Business intent (unchanged):** An AI agent that helps family-run grocery shops decide what to reorder, using sales and inventory data.

## Why these features

A GenAI hackathon rewards four things: the GenAI is the star (not a thin wrapper over SQL), a clear problem with real impact, a live demo that runs, and full use of the stack (Google ADK + Gemini + BigQuery + Cloud Run). This design keeps only the work that moves those scores. Everything else is a future slide.

## The 4-item kit

1. **Working ADK multi-agent** — the orchestrator delegates to inventory, demand, and supplier sub-agents, and each sub-agent does real work through function tools (not just Google Search).
2. **Natural-language recommendation with reasoning** — Gemini answers in plain words and says *why* (velocity, lead time, spoilage risk).
3. **Waste / expiry-aware reorder** — the one grocery-smart insight: never recommend more than the shop can sell before it spoils.
4. **Seed BigQuery dataset + one live demo query** — "What should I reorder this week?" runs end to end on stage.

Explicitly out of scope (future): budget-constrained ranking, supplier-batched orders, WhatsApp / receipt-photo ingest, trust-learning loop, day-of-week seasonality.

## Current state (what already exists)

- `internal/agent/agent.go` — orchestrator with three sub-agents wired in. Works.
- `internal/agent/{inventory,demand,supplier}/` — three `llmagent`s, each holding only `geminitool.GoogleSearch`. No real tools.
- `internal/tools/tools.go` — `check_stock` and `get_product_insights` are stubs returning placeholder text / "not implemented".
- `internal/warehouse/warehouse.go` — `GetProducts`, `GetSales`, `GetSuppliers` all return "not implemented".
- `internal/models/models.go` — `Product`, `Sale`, `Supplier`, `ProductInsight`. **No shelf-life field.**

So the scaffold is right; the guts are empty. This design fills the guts.

## Architecture / data flow

```
user asks "What should I reorder this week?"
        │
   retail_agent (orchestrator)  ── delegates ──┐
        │                                      │
        ├─ inventory_agent ── check_stock ──────► warehouse.GetProducts ──► BigQuery.products
        ├─ demand_agent ── get_product_insights ─► warehouse.GetSales ─────► BigQuery.sales
        └─ supplier_agent ── pick_supplier ──────► warehouse.GetSuppliers ─► BigQuery.suppliers
        │
   orchestrator composes plain-language answer with reasoning
```

The function tools are the bridge: each sub-agent calls a tool, the tool calls the warehouse, the warehouse queries BigQuery. The reorder math lives in a pure function so it can be unit-tested without BigQuery or Gemini.

## Components

### 1. `internal/models` — add shelf life
Add `ShelfLifeDays int` to `Product`. Add a `Recommendation` type carrying the numbers plus a short human reason string:
```go
type Recommendation struct {
    Product       Product
    AvgDailySale  float64
    ReorderQty    int
    SpoilageRisk  bool
    Reason        string // plain-language, filled by the reorder logic
}
```

### 2. `internal/warehouse` — real BigQuery client
Replace stubs with `cloud.google.com/go/bigquery`. Read queries:
- `GetProducts` → all products (including `shelf_life_days`).
- `GetSales(productID)` → last 90 days of sales for one product.
- `GetSuppliers` → all suppliers.
- `GetSalesTrend(days)` → total sales quantity per day, for the dashboard trend chart. (Plain aggregation, no agent.)
- `GetTopSellers(limit)` → products ranked by units sold over the window. (Plain aggregation, no agent.)

The last two feed dashboard metrics only. They do **not** touch the multi-agent reorder logic — backend agent design is unchanged.

Config from env: `GCLOUD_PROJECT`, `BIGQUERY_DATASET`. Keep a small interface so tests use a fake:
```go
type Store interface {
    GetProducts(ctx) ([]Product, error)
    GetSales(ctx, productID string) ([]Sale, error)
    GetSuppliers(ctx) ([]Supplier, error)
}
```

### 3. `internal/reorder` (new) — pure reorder logic
One tested function, no I/O:
```
avgDaily   = sum(quantity over last 90 days) / 90
base       = avgDaily * leadTimeDays * 1.5   // safety buffer
want       = base - stockLevel + reorderPt   // classic reorder
sellable   = avgDaily * shelfLifeDays         // most that will sell before spoiling
cap        = max(0, sellable - stockLevel)    // room left before spoilage
qty        = min(want, cap)                    // <-- the expiry-aware step
spoilage   = stockLevel > sellable             // already overstocked to spoil
```
`Reason` is a short template string, e.g. *"Sells ~12/day, 3-day lead time; capped at 40 so it sells before its 5-day shelf life."* Gemini polishes wording; the numbers come from here.

### 4. `internal/tools` — wire tools to warehouse + reorder
- `check_stock(product_name)` → look up product, return current stock vs reorder point.
- `get_product_insights(product_id)` → run warehouse + `internal/reorder`, return a `Recommendation`.
- `pick_supplier(product_id)` (new small tool for supplier_agent) → best supplier by reliability then lead time.

Give each sub-agent its matching tool in its `Tools:` list (keep `GoogleSearch` too — it's harmless and shows stack breadth).

### 5. Seed data
A `scripts/seed.sql` (or `bq` shell script) that creates the dataset and three tables and inserts ~15 products across a few categories, ~90 days of sales, and a handful of suppliers. Include perishables with short `shelf_life_days` (milk, bread) and staples with long ones (rice), so the expiry cap visibly changes a recommendation on stage.

## Error handling

- Warehouse errors bubble up as tool errors; the agent reports "couldn't read data" rather than crashing.
- Empty sales history → `avgDaily = 0` → recommend 0, reason "no recent sales".
- Missing `shelf_life_days` (0) → treat as non-perishable (skip the cap), so old data still works.

## Testing

- `internal/reorder` — table-driven unit tests: normal restock, spoilage cap kicks in, zero sales, already-overstocked flag. This is the core; test it hard.
- `internal/warehouse` — tests against the `Store` interface with a fake; no live BigQuery in CI.
- `internal/tools` — tool functions return expected shapes given a fake store.
- Agent wiring — smoke test that `agent.New` builds without error.

## Web Application (UI)

The product is a single web application. There is no chat box and no prompt — the user never types a query. They configure credentials once, then press one button.

### User flow
1. **Setup screen** — the user provides three things and nothing else:
   - Google AI Studio API key (drives Gemini).
   - Service Account credential (JSON) for BigQuery access.
   - BigQuery dataset location URL (project + dataset).
2. **Home screen** — a single button: **"See reorder recommendations"**.
3. On click, the app sends the stored credentials to the backend and triggers the multi-agent system with a fixed internal prompt (the user supplies no text). A loading state shows while the agents work.
4. **Dashboard** — when the backend responds, the page shows three panels:
   - **Reorder now** — the list of products to order now (name, current stock, recommended qty, reason). This is the multi-agent reorder output.
   - **Daily sales trend** — a line/bar chart of total sales per day (from `GetSalesTrend`).
   - **Top selling products** — a ranked list (from `GetTopSellers`).

### Backend endpoints (Cloud Run Go binary)
- `POST /run` — body carries the AI Studio key, service account JSON, and dataset URL. Builds a per-request BigQuery store, computes the reorder list deterministically, runs the multi-agent flow for a plain-language narrative, and returns **everything** the dashboard needs in one JSON response: `recommendations`, `narrative`, `sales_trend`, `top_sellers`. This is the only call the button makes.
- `GET /health` — health check.

The two metrics (daily sales trend, top sellers) are plain BigQuery reads folded into the `/run` response rather than separate GET endpoints. Reason: credentials are supplied per-request and cannot ride on a GET, so a single POST that returns the whole dashboard is simpler and keeps the one-button flow intact. The multi-agent reorder logic is unchanged — the metrics are computed alongside it, not by the agents.

### Credential handling
Credentials arrive from the browser per request and are used to build the Gemini and BigQuery clients for that request; they are not persisted server-side. Note: shipping a service-account JSON through a browser is acceptable for a hackathon demo but is not a production auth pattern — flag this as future work, do not build real secret storage now.

### Frontend scope (minimal)
Static single-page frontend (plain HTML/JS or a light framework) served by the same Cloud Run service or a static host. Two views: setup and dashboard. No routing beyond that, no auth, no persistence. Charts via a small charting lib. Keep it thin — the win is the agent + insight, not the UI.

## Demo script (3 min)

1. Show the shop's problem in one line: overstock spoils, stockouts lose sales.
2. On the home screen, press the single button — no typing.
3. Agents delegate (show the sub-agent hand-off in logs/trace); the dashboard fills in.
4. Walk the three panels: reorder-now list, daily sales trend, top sellers.
5. Point at one perishable where the expiry cap lowered the order — the "clever" moment.
6. Close with the impact line: cuts spoilage *and* stockouts.

## Success criteria

- User configures 3 inputs (AI Studio key, service account JSON, dataset URL), then one button press produces the dashboard — no prompt typed.
- Dashboard shows all three panels: reorder-now list, daily sales trend, top sellers.
- Live run returns a real recommendation from seeded BigQuery data.
- At least one recommendation is visibly reduced by the shelf-life cap.
- Each recommendation includes a plain-language reason.
- All three sub-agents are exercised in the `/run` flow.
- `internal/reorder` unit tests pass in CI.
