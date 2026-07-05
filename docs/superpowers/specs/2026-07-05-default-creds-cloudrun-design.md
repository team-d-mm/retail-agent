# Server-Default Credentials, Eval Trim, Cloud Run — Design

**Date:** 2026-07-05
**Goal:** Make the app deployable to Cloud Run with ambient ("default") credentials, so the web setup form is only needed when the server has none; drop the LLM-as-judge eval; and ship a self-contained container.
**Scope:** `internal/eval` (trim), `internal/server`, `cmd/retail-agent/main.go`, `web/` (embed + frontend), a `Dockerfile`, and deploy docs. No change to the agents, tools, reorder math, or BigQuery queries.

## Decisions (from brainstorming)

- **Gemini auth:** API key from env / Secret Manager (keep the current `genai` API-key client). BigQuery uses ambient Application Default Credentials (the Cloud Run service account). No agent-code change.
- **Setup behavior:** server-default with browser override. When the server has credentials, the web skips the setup form by default but still offers a way to enter your own (stored in `localStorage`).

## Part 1 — Remove LLM-as-judge

- Delete `internal/eval/judge.go`, `internal/eval/judge_test.go`, and `TestEval_JudgeQuality` from `internal/eval/eval_test.go`.
- Keep the four deterministic cases: `TestEval_UsesDataTools`, `TestEval_RecommendsCorrectItems`, `TestEval_NoFalseOrders`, `TestEval_MentionsSpoilageInsight`. They still run the live agent (so they `t.Skip` without `GOOGLE_API_KEY`), but there is no second judge call and no `eval.Judge` API surface.
- After removal, `internal/eval` contains only `eval_test.go` (package `eval_test`). That is fine — a test-only package is valid.

## Part 2 — Server-default credentials (`serve-web`)

### Detection (startup)
`serve-web` builds a `Defaults` value once at startup:
- `APIKey` = `os.Getenv("GOOGLE_API_KEY")`.
- `Store` = `warehouse.NewBigQueryFromEnv(ctx)` (ambient ADC + `GCLOUD_PROJECT` + `BIGQUERY_DATASET`); if that returns an error, `Store` is nil.
- Defaults are "available" when `APIKey != ""` **and** `Store != nil`.

The BigQuery client is safe for concurrent use, so the single default `Store` is shared across requests.

### `internal/server` changes
```go
type Defaults struct {
    APIKey string
    Store  warehouse.Store // nil when unavailable
}
func (d Defaults) Available() bool { return d.APIKey != "" && d.Store != nil }

func Handler(run RunFunc, defaults Defaults) http.Handler
```
- **`GET /config`** → `{"server_credentials": <defaults.Available()>}`. Lets the frontend decide whether to show setup.
- **`POST /run`** becomes dual-mode:
  - Body with all three fields non-empty → per-request path (build store from the request's service-account JSON + dataset URL, use the request's API key) — unchanged behavior.
  - Body with all three fields empty (or absent) **and** `defaults.Available()` → use `defaults.APIKey` and `defaults.Store`.
  - Any other combination (partial creds, or empty with no defaults) → `400`.
- The existing error-code contract (502 on store/query failure, best-effort narrative) is unchanged. The `storeFactory` seam for tests stays for the per-request path.

### `cmd/retail-agent/main.go`
`serveWeb` builds `Defaults` from the environment and passes it to `server.Handler(agentrun.Run, defaults)`. The launcher CLI path is unchanged.

## Part 3 — Frontend (`/config`-driven)

On load, `app.js` calls `GET /config`, then chooses a view by precedence:

1. **Browser creds present** (`localStorage["retailAgentCreds"]`) → Dashboard; `/run` posts those creds (override).
2. **Else server_credentials true** → Dashboard in "server mode"; `/run` posts an empty body `{}` so the server uses its defaults. A subtle line ("Using server credentials") plus a **"Use my own credentials"** link opens the setup form to store browser creds (override).
3. **Else** → Setup form (today's first-run flow).

- **Setup save** (unchanged): validate 3 fields + JSON, store in `localStorage`, go to Dashboard, clear inputs.
- **Reset / "Use my own credentials":** Reset clears browser creds; the view then falls back to server mode if `server_credentials`, else the setup form. "Use my own credentials" opens the setup form even when server creds exist.
- The reorder table, CSV export, chart, and XSS-safe rendering are unchanged.

## Part 4 — Cloud Run deployment

### Embed web assets
The current `http.FileServer(http.Dir("web"))` reads the working directory — it breaks in a minimal container. Fix:
- New `web/web.go`, `package web`, with `//go:embed index.html app.js style.css` exposing `var Files embed.FS`.
- `serveWeb` serves `http.FileServerFS(web.Files)` instead of `http.Dir("web")`. (Adding a Go file to `web/` makes it a package; the HTML/CSS/JS files are unaffected.)

### Dockerfile
Multi-stage:
- Stage 1: `golang:1.26` — `go build -o /retail-agent ./cmd/retail-agent` (web assets are embedded in the binary).
- Stage 2: a small base (`gcr.io/distroless/base-debian12` or `alpine`) — copy the binary, `EXPOSE 8080`, `ENV PORT=8080`, `CMD ["/retail-agent", "serve-web"]`.

The server already reads `PORT` (Cloud Run sets it), defaulting to 8080.

### Deploy docs
Add a "Deploy to Cloud Run" section to `README.md` (and a pointer in `AGENTS.md`):
```
gcloud run deploy retail-agent \
  --source . \
  --region <REGION> \
  --service-account retail-agent@<PROJECT>.iam.gserviceaccount.com \
  --set-env-vars GCLOUD_PROJECT=<PROJECT>,BIGQUERY_DATASET=<PROJECT>.retail \
  --set-secrets GOOGLE_API_KEY=GOOGLE_API_KEY:latest \
  --allow-unauthenticated
```
- Service account needs **BigQuery Data Viewer** (and BigQuery Job User to run queries) on the dataset's project.
- `GOOGLE_API_KEY` stored in Secret Manager, mounted as an env var.
- With these set, `serve-web` reports `server_credentials: true` and the deployed app shows the dashboard with no setup form.

## Error handling

- `GET /config` never fails hard: it reports `server_credentials` as a boolean derived from startup detection.
- `POST /run` with partial creds, or empty creds when no server defaults → `400` with a clear message ("provide credentials or configure server defaults").
- Frontend `/config` fetch failure → fall back to treating the server as having no defaults (show browser flow), so a config hiccup never blocks first-run setup.
- Startup: if `NewBigQueryFromEnv` errors, the server still starts (defaults just unavailable) — it logs a line and serves the browser-setup flow.

## Testing

- `go build ./... && go vet ./... && go test ./...` stays green.
- `internal/server` unit tests (with `FakeStore` + injected factory) extended for: `GET /config` true/false; `/run` server-default path (empty body + defaults available → 200); `/run` empty body + no defaults → 400; per-request path still works. These are offline (no live model) using the injected `RunFunc`/factory and a `Defaults{APIKey:"x", Store: fake}`.
- The four deterministic eval cases remain (skip without a key).
- Container: `docker build` succeeds; a documented manual `docker run -e ... -p 8080:8080` smoke (health + `/config`) — manual, since it needs Docker.
- Frontend: manual browser smoke — server-mode (no browser creds) shows dashboard directly; "Use my own credentials" opens setup; reset returns to server mode.

## Out of scope

- Vertex AI / ADC-based Gemini auth (chose API-key-from-env).
- CI/CD pipeline for deploys (manual `gcloud run deploy` documented).
- Any change to the agents, tools, reorder math, or BigQuery queries.
