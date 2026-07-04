# retail-agent — Project Specification

## Problem Statement

Family-run grocery shops lack data-driven tools to make purchasing decisions. Owners rely on intuition, leading to overstock, stockouts, and missed sales. This project delivers an AI-powered decision-making agent — built with Google ADK in Go — that analyzes inventory and sales data from BigQuery, forecasts demand, and recommends what to reorder and how much.

## MVP Scope (Hackathon Deliverable)

Single feature: **reorder recommendations**. The agent:
- Reads current stock levels and sales history from BigQuery
- Computes average daily sale velocity per product
- Applies a reorder policy (stock below threshold → suggest order quantity)
- Surfaces recommendations via a dashboard UI

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  Dashboard (UI)                  │
│  Displays reorder recommendations to shop owner  │
└────────────────────┬────────────────────────────┘
                     │ HTTP / API call
┌────────────────────▼────────────────────────────┐
│              Cloud Run (Go binary)              │
│  ┌───────────────────────────────────────────┐  │
│  │           retail_agent (ADK)              │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐   │  │
│  │  │inventory │ │  demand  │ │ supplier │   │  │
│  │  │  agent   │ │  agent   │ │  agent   │   │  │
│  │  └──────────┘ └──────────┘ └──────────┘   │  │
│  └───────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────┐  │
│  │        function tools layer               │  │
│  │  check_stock  │  get_product_insights     │  │
│  └──────────┬────────────────────────────────┘  │
└─────────────┼───────────────────────────────────┘
              │ BigQuery queries
┌─────────────▼────────────────────────────────────┐
│              BigQuery                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │ products │ │  sales   │ │suppliers │          │
│  └──────────┘ └──────────┘ └──────────┘          │
└──────────────────────────────────────────────────┘
```

## Data Model (BigQuery)

### `products`

| Column       | Type    | Description                          |
|--------------|---------|--------------------------------------|
| product_id   | STRING  | Primary key                          |
| name         | STRING  | Product name                         |
| category     | STRING  | Grocery category (dairy, produce…)   |
| supplier_id  | STRING  | FK → suppliers                       |
| unit_price   | FLOAT64 | Current unit price                   |
| stock_level  | INT64   | Current units in stock               |
| reorder_pt   | INT64   | When stock dips below this, reorder  |

### `sales`

| Column     | Type      | Description                     |
|------------|-----------|---------------------------------|
| sale_id    | STRING    | Primary key                     |
| product_id | STRING    | FK → products                   |
| quantity   | INT64     | Units sold                      |
| total      | FLOAT64   | Sale total                      |
| date       | DATE      | Sale date                       |

### `suppliers`

| Column          | Type    | Description              |
|-----------------|---------|--------------------------|
| supplier_id     | STRING  | Primary key              |
| name            | STRING  | Supplier name            |
| reliability     | FLOAT64 | 0–1 score                |
| lead_time_days  | INT64   | Avg days to deliver      |
| avg_unit_price  | FLOAT64 | Average price charged    |

## Agent Design

### Orchestrator (`retail_agent`)

- **Model:** `gemini-2.5-flash`
- **Role:** Entry point. Understands the user's request and delegates to the appropriate sub-agent.
- **Tools:** Google Search (for market context), custom tools

### Sub-agents

| Agent            | Responsibility                                                    |
|------------------|------------------------------------------------------------------|
| `inventory_agent`| Check stock levels, flag items below reorder point               |
| `demand_agent`   | Analyze sales history, compute velocity, forecast near-term need |
| `supplier_agent` | Score suppliers by reliability, lead time, and cost              |

### Reorder Logic (MVP)

For each product where `stock_level < reorder_pt`:
1. Get `avg_daily_sales` from last 90 days in BigQuery
2. Forecast demand as `avg_daily_sales × lead_time_days × safety_buffer (1.5)`
3. Recommend order quantity = `forecast - current_stock + reorder_pt`
4. Surface: product name, current stock, reorder point, recommended qty, and top supplier

## API Endpoints

The Cloud Run service exposes HTTP endpoints for the dashboard:

| Method | Path              | Description                              |
|--------|-------------------|------------------------------------------|
| GET    | `/recommendations`| Reorder recommendations (all or `?category=produce`) |
| GET    | `/recommendations/{product_id}` | Recommendation for a single product |
| GET    | `/categories`     | List available product categories        |
| GET    | `/health`         | Health check                             |

The dashboard is the primary user-facing product. It consumes these endpoints to display actionable purchase recommendations grouped by category, with drill-down for individual product details.

## Deployment (Cloud Run)

- Containerised Go binary via `Dockerfile`
- Environment: `GOOGLE_API_KEY`, `GCLOUD_PROJECT`, `BIGQUERY_DATASET`
- Service account with BigQuery Data Viewer + Vertex AI User roles
- Deployed via `gcloud run deploy`

## Project Structure

```
./
├── cmd/retail-agent/main.go         — entrypoint (CLI + HTTP server)
├── internal/
│   ├── agent/                        — orchestrator agent
│   │   ├── inventory/                — inventory analysis sub-agent
│   │   ├── demand/                   — demand forecasting sub-agent
│   │   └── supplier/                 — supplier scoring sub-agent
│   ├── tools/                        — custom function tools
│   ├── warehouse/                    — BigQuery client abstraction
│   └── models/                       — shared domain types
├── docs/specs.md                     — this file
├── .github/workflows/ci.yml          — CI pipeline
├── Dockerfile                        — container image for Cloud Run
├── AGENTS.md                         — agent instructions
├── README.md                         — human-facing README
└── opencode.json                     — OpenCode config
```

## Future (Post-MVP)

- Full supplier scoring with cost optimisation
- Seasonal demand forecasting models
- Multi-shop support
- Alerting (email/SMS notifications for low stock)
- Pricing suggestions based on competitor data
