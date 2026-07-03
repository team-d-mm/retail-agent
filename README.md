# retail-agent

Retail decision-making agent built with Google ADK in Go. Helps family-run shop owners with product purchasing decisions. Hackathon project for Hack2Skill + Google Cloud AI Decision Making Platform.

## Stack

- **Language:** Go
- **AI:** Google ADK (Agent Development Kit), multi-agent architecture
- **Infra:** Google Cloud (to be chosen during implementation)
- **Data:** Data warehouse to store product/sales/decision data for LLM context

## Setup

1. Get a Gemini API key at https://aistudio.google.com/app/apikey
2. Copy `.env.example` to `.env` and add your key:

   ```bash
   export GOOGLE_API_KEY="your-key-here"
   ```

## Run

**CLI:**

```bash
source .env && go run ./cmd/retail-agent
```

**Web UI** (dev-only, at `http://localhost:8080`):

```bash
source .env && go run ./cmd/retail-agent web api webui
```

**Build all:**

```bash
go build ./...
```

## Structure

- `cmd/retail-agent/main.go` — entrypoint
- `internal/agent/` — agent definition
