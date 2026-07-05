# retail-agent

Retail decision-making agent built with Google ADK in Go. Helps family-run shop owners with product purchasing decisions. Hackathon project for Hack2Skill + Google Cloud AI Decision Making Platform.

## Stack

- **Language:** Go 1.26
- **AI:** Google ADK (Agent Development Kit), multi-agent architecture, Gemini (`gemini-2.5-flash`)
- **Infra:** Cloud Run (Go binary), BigQuery
- **Web:** Go HTTP server + static single-page dashboard (Chart.js)

## Design

- Orchestrator agent (`retail_agent`) delegates to sub-agents (inventory analysis, demand forecasting, supplier scoring)
- Sub-agents live in separate packages under `internal/agent/`; each holds its own function tool
- Custom function tools in `internal/tools/` (`check_stock`, `get_product_insights`, `pick_supplier`), bound to a `warehouse.Store`
- Expiry-aware reorder math is a pure, unit-tested function in `internal/reorder/` (caps orders so perishables sell before spoiling); the deterministic result is the source of truth, the LLM adds a plain-language narrative
- `internal/warehouse/` is a `Store` interface with a BigQuery impl (`BQStore`) and an in-memory `FakeStore` for tests
- `internal/server/` exposes `POST /run` (button → agents + reads) and `GET /health`; `internal/agentrun/` runs the orchestrator programmatically (`Run` returns the narrative, `RunTrace` also returns the tool-call trajectory)
- The product is a one-button web app (`web/`): the owner stores 3 credentials once in browser `localStorage`, presses a button, gets a dashboard. Credentials are sent per request and never persisted server-side
- Shared types in `internal/models/`
- Agent instructions live in `internal/instructions/` as separate markdown files per agent, embedded at compile time via `go:embed`
- Evaluation/judge framework in `internal/eval/`

## Commands

| Action | Command |
|--------|---------|
| Run CLI agent | `source .env && go run ./cmd/retail-agent` |
| Run Web UI (ADK launcher) | `source .env && go run ./cmd/retail-agent web api webui` |
| Run Web App (button UI) | `go run ./cmd/retail-agent serve-web` then open http://localhost:8080 |
| Seed demo BigQuery data | `bq query --use_legacy_sql=false < scripts/seed.sql` |
| Build all | `go build ./...` |
| Test all | `go test ./...` |
| Run agent-response eval (live) | `source .env && go test ./internal/eval/ -v` |
| Run vet (lint) | `go vet ./...` |
| Tidy deps | `go mod tidy` |
| Full CI check | `go build ./... && go vet ./... && go test ./...` |
| Build container | `docker build -t retail-agent .` |
| Deploy to Cloud Run | `gcloud run deploy retail-agent --source . ...` (see README) |

## Structure

- `cmd/retail-agent/main.go` — entrypoint (`serve-web` product server + ADK launcher CLI)
- `internal/agent/` — orchestrator agent (wires sub-agents), takes `(ctx, apiKey, store)`
- `internal/agent/inventory/` — inventory analysis sub-agent (`check_stock`)
- `internal/agent/demand/` — demand forecasting sub-agent (`get_product_insights`)
- `internal/agent/supplier/` — supplier scoring sub-agent (`pick_supplier`)
- `internal/instructions/` — per-agent instruction markdown files, embedded via `go:embed`
- `internal/tools/` — custom function tools (store-bound constructors)
- `internal/reorder/` — pure, tested expiry-aware reorder math (`Recommend`, `RecommendAll`)
- `internal/warehouse/` — `Store` interface, `BQStore` (BigQuery), `FakeStore` (tests), dataset URL parser
- `internal/agentrun/` — runs the orchestrator programmatically (`Run`, `RunTrace`)
- `internal/server/` — HTTP handlers (`POST /run`, `GET /health`)
- `internal/eval/` — agent-response evaluation (deterministic test cases; no LLM-as-judge)
- `internal/models/` — shared domain types
- `web/` — setup + dashboard frontend (`index.html`, `app.js`, `style.css`)
- `scripts/seed.sql` — demo BigQuery data
- `docs/` — specs and plans (`docs/superpowers/`)
- `.github/workflows/ci.yml` — CI (build + vet + test)
- `go.mod` / `go.sum` — Go module (module path: `github.com/team-d-mm/retail-agent`)

## Setup

1. Get a Gemini API key at https://aistudio.google.com/app/apikey
2. Copy `.env.example` to `.env` and add your key: `export GOOGLE_API_KEY="..."`
3. (For BigQuery) set `GCLOUD_PROJECT` and `BIGQUERY_DATASET`, authenticate via ADC or service account

## Notes for agents

- Tests that call `agent.New`, `agentrun.Run`/`RunTrace`, or the live eval require `GOOGLE_API_KEY`; they skip when absent
- `functiontool.New` is the ADK API for wrapping Go functions as tools
- Sub-agents are wired via `llmagent.Config.SubAgents` field
- `agent.New` and the sub-agent `New` funcs return `(Agent, error)` — construction errors propagate (no `log.Fatalf` on the request path)
- Agent instructions live in `internal/instructions/*.md` (embedded via `//go:embed` in `internal/instructions/instructions.go`) — tune prompts by editing the markdown, not the Go code
- The `serve-web` server needs no credentials at startup; `POST /run` takes them per request. The BigQuery `Store` is built per request from the request's service-account JSON + dataset URL
- Agent-response evals live in `internal/eval/`; they run the orchestrator against a seeded `FakeStore` and check the narrative against the deterministic `reorder.RecommendAll` ground truth (keyword/set assertions).
- `serve-web` reports `server_credentials: true` via `GET /config` when `GOOGLE_API_KEY` + ADC BigQuery (`GCLOUD_PROJECT`/`BIGQUERY_DATASET`) are present; the web then skips the setup form. `web/` assets are embedded via `go:embed` (`web/web.go`).
- **Secrets:** real keys go in `.env` (git-ignored); `.env.example` holds placeholders only. Service-account `*.json` keys are git-ignored — never commit them

---

Also read `CLAUDE.md` if present — it may contain additional repo-specific instructions.
