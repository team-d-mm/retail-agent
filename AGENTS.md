# retail-agent

Retail decision-making agent built with Google ADK in Go. Helps family-run shop owners with product purchasing decisions. Hackathon project for Hack2Skill + Google Cloud AI Decision Making Platform.

## Stack

- **Language:** Go
- **AI:** Google ADK (Agent Development Kit), multi-agent architecture
- **Infra:** Google Cloud (to be chosen during implementation)
- **Data:** Data warehouse to store product/sales/decision data for LLM context

## Design

- Orchestrator agent (`retail_agent`) delegates to sub-agents (inventory analysis, demand forecasting, supplier scoring)
- Sub-agents live in separate packages under `internal/agent/`
- Custom function tools in `internal/tools/`
- Data warehouse abstraction in `internal/warehouse/`
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
| Run vet (lint) | `go vet ./...` |
| Tidy deps | `go mod tidy` |
| Full CI check | `go build ./... && go vet ./... && go test ./...` |

## Structure

- `cmd/retail-agent/main.go` — entrypoint
- `internal/agent/` — orchestrator agent (wires sub-agents)
- `internal/agent/inventory/` — inventory analysis sub-agent
- `internal/agent/demand/` — demand forecasting sub-agent
- `internal/agent/supplier/` — supplier scoring sub-agent
- `internal/instructions/` — per-agent instruction markdown files, embedded via `go:embed`
- `internal/tools/` — custom function tools
- `internal/warehouse/` — data warehouse abstraction layer (BigQuery `BQStore` + in-memory `FakeStore`)
- `internal/models/` — shared domain types
- `internal/reorder/` — reorder recommendation engine
- `internal/agentrun/` — runs orchestrator with fixed prompt, returns narrative + tool calls
- `internal/server/` — HTTP handler (POST /run, GET /health)
- `internal/eval/` — LLM-as-judge evaluation framework for agent responses
- `scripts/seed.sql` — BigQuery seed data (6 grocery products, 3 suppliers, 90 days of sales)
- `web/` — dashboard frontend (HTML + JS + CSS)
- `.github/workflows/ci.yml` — CI (build + vet + test)
- `go.mod` / `go.sum` — Go module (module path: `github.com/team-d-mm/retail-agent`)

## Setup

1. Get a Gemini API key at https://aistudio.google.com/app/apikey
2. Copy `.env.example` to `.env` and add your key: `export GOOGLE_API_KEY="..."`
3. (For BigQuery) set `GCLOUD_PROJECT` and `BIGQUERY_DATASET`, authenticate via ADC or service account

## Notes for agents

- Tests that call `agent.New` require `GOOGLE_API_KEY` set; they skip when absent
- `functiontool.New` is the ADK API for wrapping Go functions as tools
- Sub-agents are wired via `llmagent.Config.SubAgents` field
- Agent instructions are in `internal/instructions/*.md` — edit the markdown, not the Go code
- `internal/instructions/instructions.go` embeds all instruction files via `//go:embed`

---

Also read `CLAUDE.md` if present — it may contain additional repo-specific instructions.
