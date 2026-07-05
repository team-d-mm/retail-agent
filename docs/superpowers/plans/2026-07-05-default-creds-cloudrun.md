# Server-Default Credentials, Eval Trim, Cloud Run — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve the web app with ambient server credentials (Cloud Run "default credential" style) so the setup form is only needed when the server has none, drop the LLM-as-judge eval, and ship a self-contained container.

**Architecture:** `serve-web` detects env/ADC credentials at startup and exposes `GET /config`; `POST /run` becomes dual-mode (per-request creds OR server defaults). The frontend picks its view from `/config` with a browser-creds override. Web assets are embedded via `go:embed` and served with `http.FileServerFS`, and a `Dockerfile` builds a single Cloud Run image.

**Tech Stack:** Go 1.26, ADK, `google.golang.org/genai`, BigQuery, `go:embed`, Docker, Cloud Run.

## Global Constraints

- Module path `github.com/team-d-mm/retail-agent`, Go 1.26.4.
- Gemini auth = API key from env (`GOOGLE_API_KEY`); BigQuery = ambient ADC via `warehouse.NewBigQueryFromEnv` (`GCLOUD_PROJECT` + `BIGQUERY_DATASET`).
- `POST /run` body stays `{ai_studio_key, service_account_json, dataset_url}`; response stays `{recommendations, narrative, sales_trend, top_sellers}`.
- Error contract: per-request store/query failure → 502; best-effort narrative (failure → empty, still 200); missing/partial creds with no server defaults → 400.
- Frontend renders server strings via `textContent`/DOM only (no `innerHTML` with server data).
- Live-agent tests skip without `GOOGLE_API_KEY`; server unit tests are offline (FakeStore + injected factory + `Defaults`).
- CI must stay green: `go build ./... && go vet ./... && go test ./...`.
- `serve-web` must start and serve with no credentials present.

---

## File Structure

- `internal/eval/judge.go`, `internal/eval/judge_test.go` (delete) + `internal/eval/eval_test.go` (remove one case).
- `internal/server/server.go` (modify) — `Defaults`, `GET /config`, dual-mode `POST /run`; extract a shared run-pipeline helper.
- `internal/server/server_test.go` (modify) — update `Handler(...)` calls; add `/config` + default-path + no-default tests.
- `web/web.go` (create) — `package web` with `//go:embed` of the three assets.
- `cmd/retail-agent/main.go` (modify) — `serveWeb` serves embedded FS, builds `Defaults`, adds `/config` route.
- `web/index.html`, `web/app.js` (modify) — `/config`-driven view selection + browser override.
- `Dockerfile` (create) — multi-stage build, `CMD serve-web`.
- `README.md`, `AGENTS.md` (modify) — Cloud Run deploy docs.

---

## Task 1: Remove LLM-as-judge

**Files:**
- Delete: `internal/eval/judge.go`, `internal/eval/judge_test.go`
- Modify: `internal/eval/eval_test.go`

**Interfaces:**
- Produces: `internal/eval` becomes a test-only package (`package eval_test`) with four deterministic cases; no `eval.Judge` API.

- [ ] **Step 1: Delete the judge files and the judge case**

```bash
git rm internal/eval/judge.go internal/eval/judge_test.go
```
Then edit `internal/eval/eval_test.go`: remove the entire `func TestEval_JudgeQuality(t *testing.T) { ... }` function, and remove the now-unused imports it required — delete `"fmt"` and the `"github.com/team-d-mm/retail-agent/internal/eval"` and `"github.com/team-d-mm/retail-agent/internal/reorder"` import lines **only if** no other code in the file references them. (After removing the judge case, `reorder`, `eval`, and `fmt` are no longer used; `context`, `os`, `strings`, `sync`, `testing`, `regexp`, `agentrun`, `models`, `warehouse` remain.)

- [ ] **Step 2: Verify build, vet, and skip behavior**

Run: `go build ./... && go vet ./... && go test ./internal/eval/ -v`
Expected: build + vet clean; the four `TestEval_*` cases SKIP (no key); no `TestParseVerdict`/`TestEval_JudgeQuality` remain. `go vet` must report no unused imports.

- [ ] **Step 3: Run the full suite**

Run: `go test ./...`
Expected: all packages ok (eval cases skip).

- [ ] **Step 4: Commit**

```bash
git add -A internal/eval/
git commit -m "test(eval): remove LLM-as-judge; keep deterministic cases"
```

---

## Task 2: Embed web assets + serve via FileServerFS

**Files:**
- Create: `web/web.go`
- Modify: `cmd/retail-agent/main.go` (the `serveWeb` static handler line only)

**Interfaces:**
- Produces: `web.Files embed.FS` (embeds `index.html`, `app.js`, `style.css`).
- Consumes: nothing new.

- [ ] **Step 1: Create the embed package**

Create `web/web.go`:
```go
// Package web holds the embedded static frontend assets.
package web

import "embed"

//go:embed index.html app.js style.css
var Files embed.FS
```

- [ ] **Step 2: Serve the embedded FS from main**

In `cmd/retail-agent/main.go`, add the import `"github.com/team-d-mm/retail-agent/internal/..."` is not needed — add `"github.com/team-d-mm/retail-agent/web"`. Change the static-file line in `serveWeb` from:
```go
	mux.Handle("/", http.FileServer(http.Dir("web")))
```
to:
```go
	mux.Handle("/", http.FileServerFS(web.Files))
```

- [ ] **Step 3: Verify build + serving works from any cwd**

Run:
```bash
go build ./... && (cd /tmp && PORT=8095 go run github.com/team-d-mm/retail-agent/cmd/retail-agent serve-web &) && sleep 2 && \
  curl -s localhost:8095/ | grep -c 'id="setupView"' && \
  curl -s localhost:8095/app.js | grep -c "initView" && \
  curl -s localhost:8095/health && \
  pkill -f "retail-agent serve-web"
```
Expected: `1`, `1`, `ok` — the assets are served even when the process runs from `/tmp` (proves embedding, not cwd).

- [ ] **Step 4: Commit**

```bash
git add web/web.go cmd/retail-agent/main.go
git commit -m "feat(web): embed frontend assets, serve via FileServerFS"
```

---

## Task 3: Server default credentials (`Defaults`, `/config`, dual `/run`)

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/server_test.go`
- Modify: `cmd/retail-agent/main.go` (`serveWeb`: build `Defaults`, pass to `Handler`, add `/config` route)

**Interfaces:**
- Produces: `server.Defaults{APIKey string, Store warehouse.Store}` with `Available() bool`; `server.Handler(run RunFunc, defaults Defaults) http.Handler`; `GET /config` → `{"server_credentials": bool}`; dual-mode `POST /run`.
- Consumes: `warehouse.NewBigQueryFromEnv`, `agentrun.Run`.

- [ ] **Step 1: Write the failing tests**

In `internal/server/server_test.go`, update the two existing `server.Handler(run)` / `server.Handler(nil)` calls to pass an empty `server.Defaults{}` as the second argument (e.g. `server.Handler(run, server.Defaults{})`, `server.Handler(nil, server.Defaults{})`). Then add:
```go
func TestConfigServerCredentials(t *testing.T) {
	fs := &warehouse.FakeStore{}
	cases := []struct {
		name string
		def  server.Defaults
		want bool
	}{
		{"available", server.Defaults{APIKey: "k", Store: fs}, true},
		{"no key", server.Defaults{Store: fs}, false},
		{"no store", server.Defaults{APIKey: "k"}, false},
		{"empty", server.Defaults{}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h := server.Handler(nil, tt.def)
			req := httptest.NewRequest(http.MethodGet, "/config", nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d", rr.Code)
			}
			var resp struct {
				ServerCredentials bool `json:"server_credentials"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.ServerCredentials != tt.want {
				t.Errorf("server_credentials = %v, want %v", resp.ServerCredentials, tt.want)
			}
		})
	}
}

func TestRunUsesServerDefaults(t *testing.T) {
	fs := &warehouse.FakeStore{
		Products:       []models.Product{{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4}},
		Suppliers:      []models.Supplier{{ID: "S1", LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{"P1": {{ProductID: "P1", Quantity: 900}}},
		Trend:          []models.DailySales{{Date: "2026-07-01", Quantity: 12}},
		TopSellers:     []models.TopSeller{{ProductID: "P1", Name: "Milk", Units: 40}},
	}
	ran := false
	run := func(ctx context.Context, apiKey string, s warehouse.Store) (string, error) {
		ran = true
		if apiKey != "server-key" {
			t.Errorf("expected server default api key, got %q", apiKey)
		}
		return "narrative", nil
	}
	h := server.Handler(run, server.Defaults{APIKey: "server-key", Store: fs})

	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader([]byte("{}")))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !ran {
		t.Error("run should have been called with server defaults")
	}
	var resp struct {
		Recommendations []models.Recommendation `json:"recommendations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || len(resp.Recommendations) != 1 {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
}

func TestRunEmptyBodyNoDefaults(t *testing.T) {
	h := server.Handler(nil, server.Defaults{})
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader([]byte("{}")))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -v`
Expected: FAIL — `server.Defaults`/second Handler arg undefined (compile error).

- [ ] **Step 3: Implement the server changes**

Replace `internal/server/server.go` with:
```go
package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

// RunFunc runs the multi-agent orchestrator and returns a narrative.
type RunFunc func(ctx context.Context, apiKey string, store warehouse.Store) (string, error)

// Defaults are server-side credentials discovered from the environment (env API
// key + ambient-ADC BigQuery store). When available, the web app can run without
// the user entering anything.
type Defaults struct {
	APIKey string
	Store  warehouse.Store
}

// Available reports whether the server can run without per-request credentials.
func (d Defaults) Available() bool { return d.APIKey != "" && d.Store != nil }

type storeFactoryFunc func(ctx context.Context, credsJSON []byte, datasetURL string) (warehouse.Store, error)

var storeFactory storeFactoryFunc = func(ctx context.Context, credsJSON []byte, datasetURL string) (warehouse.Store, error) {
	return warehouse.NewBigQuery(ctx, credsJSON, datasetURL)
}

// SetStoreFactoryForTest overrides the BigQuery store builder in tests.
func SetStoreFactoryForTest(f storeFactoryFunc) { storeFactory = f }

type runRequest struct {
	AIStudioKey        string `json:"ai_studio_key"`
	ServiceAccountJSON string `json:"service_account_json"`
	DatasetURL         string `json:"dataset_url"`
}

type runResponse struct {
	Recommendations []models.Recommendation `json:"recommendations"`
	Narrative       string                  `json:"narrative"`
	SalesTrend      []models.DailySales     `json:"sales_trend"`
	TopSellers      []models.TopSeller      `json:"top_sellers"`
}

// Handler builds the HTTP handler. run may be nil for health/config-only use in tests.
func Handler(run RunFunc, defaults Defaults) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"server_credentials": defaults.Available()})
	})

	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		ctx := r.Context()

		allSet := req.AIStudioKey != "" && req.ServiceAccountJSON != "" && req.DatasetURL != ""
		allEmpty := req.AIStudioKey == "" && req.ServiceAccountJSON == "" && req.DatasetURL == ""

		var apiKey string
		var store warehouse.Store
		switch {
		case allSet:
			apiKey = req.AIStudioKey
			s, err := storeFactory(ctx, []byte(req.ServiceAccountJSON), req.DatasetURL)
			if err != nil {
				http.Error(w, "could not connect to BigQuery: "+err.Error(), http.StatusBadGateway)
				return
			}
			store = s
		case allEmpty && defaults.Available():
			apiKey = defaults.APIKey
			store = defaults.Store
		default:
			http.Error(w, "provide ai_studio_key, service_account_json, and dataset_url, or configure server default credentials", http.StatusBadRequest)
			return
		}

		recs, err := reorder.RecommendAll(ctx, store)
		if err != nil {
			http.Error(w, "could not read products/sales: "+err.Error(), http.StatusBadGateway)
			return
		}
		trend, err := store.GetSalesTrend(ctx, 30)
		if err != nil {
			http.Error(w, "could not read sales trend: "+err.Error(), http.StatusBadGateway)
			return
		}
		top, err := store.GetTopSellers(ctx, 5)
		if err != nil {
			http.Error(w, "could not read top sellers: "+err.Error(), http.StatusBadGateway)
			return
		}

		var narrative string
		if run != nil {
			// Best-effort: a narrative failure must not blank the dashboard.
			if n, err := run(ctx, apiKey, store); err == nil {
				narrative = n
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runResponse{
			Recommendations: recs,
			Narrative:       narrative,
			SalesTrend:      trend,
			TopSellers:      top,
		})
	})

	return mux
}
```

- [ ] **Step 4: Wire main.go**

In `cmd/retail-agent/main.go` `serveWeb`, build `Defaults` from the environment and register `/config`. Replace the body of `serveWeb` so the handler section reads:
```go
	apiKey := os.Getenv("GOOGLE_API_KEY")
	var defStore warehouse.Store
	if s, err := warehouse.NewBigQueryFromEnv(ctx); err != nil {
		log.Printf("server default BigQuery credentials unavailable (%v); web will ask for credentials", err)
	} else {
		defStore = s
	}
	api := server.Handler(agentrun.Run, server.Defaults{APIKey: apiKey, Store: defStore})

	mux := http.NewServeMux()
	mux.Handle("/run", api)
	mux.Handle("/health", api)
	mux.Handle("/config", api)
	mux.Handle("/", http.FileServerFS(web.Files))
```
Keep the surrounding `port`/`ListenAndServe` code. Ensure `warehouse` is imported in main.go (it already is).

- [ ] **Step 5: Run tests + build to verify**

Run: `go build ./... && go vet ./... && go test ./internal/server/ -v`
Expected: build + vet clean; all server tests pass (existing per-request + new config/default/no-default cases).

- [ ] **Step 6: Commit**

```bash
git add internal/server/ cmd/retail-agent/main.go
git commit -m "feat(server): server-default credentials via /config and dual-mode /run"
```

---

## Task 4: Frontend `/config`-driven view + browser override

**Files:**
- Modify: `web/index.html` (header controls)
- Modify: `web/app.js` (config fetch + precedence + server mode)

**Interfaces:**
- Consumes: `GET /config` (`{server_credentials}`), `POST /run` (empty body in server mode).

- [ ] **Step 1: Add header controls to index.html**

In `web/index.html`, replace the `<header class="topbar"> ... </header>` block with:
```html
	<header class="topbar">
		<div class="brand">
			<span class="brand-mark">🛒</span>
			<span class="brand-name">Retail Agent</span>
		</div>
		<div class="topbar-actions">
			<span id="credMode" class="muted"></span>
			<button id="useOwnBtn" class="btn btn-ghost" hidden>Use my own credentials</button>
			<button id="resetBtn" class="btn btn-ghost" hidden>Reset credentials</button>
		</div>
	</header>
```

- [ ] **Step 2: Replace web/app.js**

Replace `web/app.js` with (adds `/config`, `serverMode`, header management, empty-body run; keeps storage, render, CSV unchanged):
```js
const STORAGE_KEY = "retailAgentCreds";
const $ = (id) => document.getElementById(id);
let trendChart = null;
let lastRecommendations = [];
let serverMode = false; // server has default credentials

// ---- credential storage (browser-only) ----
function loadCreds() {
	try {
		return JSON.parse(localStorage.getItem(STORAGE_KEY)) || null;
	} catch {
		return null;
	}
}
function saveCreds(creds) {
	localStorage.setItem(STORAGE_KEY, JSON.stringify(creds));
}
function clearCreds() {
	localStorage.removeItem(STORAGE_KEY);
}

// ---- header state ----
function setHeader(mode) {
	// mode: "browser" | "server" | "setup"
	if (mode === "browser") {
		$("credMode").textContent = "";
		$("useOwnBtn").hidden = true;
		$("resetBtn").hidden = false;
	} else if (mode === "server") {
		$("credMode").textContent = "Using server credentials";
		$("useOwnBtn").hidden = false;
		$("resetBtn").hidden = true;
	} else {
		$("credMode").textContent = "";
		$("useOwnBtn").hidden = true;
		$("resetBtn").hidden = true;
	}
}

// ---- view toggle ----
function showSetup() {
	$("setupView").hidden = false;
	$("dashboardView").hidden = true;
	setHeader("setup");
}
function showDashboard(mode) {
	$("setupView").hidden = true;
	$("dashboardView").hidden = false;
	setHeader(mode);
}

async function serverHasCreds() {
	try {
		const res = await fetch("/config");
		if (!res.ok) return false;
		const cfg = await res.json();
		return !!cfg.server_credentials;
	} catch {
		return false;
	}
}

async function initView() {
	if (loadCreds()) {
		serverMode = await serverHasCreds();
		showDashboard("browser");
		return;
	}
	serverMode = await serverHasCreds();
	if (serverMode) {
		showDashboard("server");
	} else {
		showSetup();
	}
}

// ---- setup ----
function handleSave() {
	const creds = {
		ai_studio_key: $("aiKey").value.trim(),
		service_account_json: $("saJson").value.trim(),
		dataset_url: $("datasetUrl").value.trim(),
	};
	if (!creds.ai_studio_key || !creds.service_account_json || !creds.dataset_url) {
		$("setupStatus").textContent = "Please fill in all three fields.";
		return;
	}
	try {
		JSON.parse(creds.service_account_json);
	} catch {
		$("setupStatus").textContent = "Service account JSON is not valid JSON.";
		return;
	}
	saveCreds(creds);
	$("aiKey").value = "";
	$("saJson").value = "";
	$("datasetUrl").value = "";
	$("setupStatus").textContent = "";
	showDashboard("browser");
}

// ---- open the setup form on demand (browser override) ----
function handleUseOwn() {
	showSetup();
}

// ---- reset ----
function handleReset() {
	clearCreds();
	lastRecommendations = [];
	$("results").hidden = true;
	$("runStatus").textContent = "";
	$("runStatus").className = "status";
	$("csvBtn").disabled = true;
	if (trendChart) {
		trendChart.destroy();
		trendChart = null;
	}
	if (serverMode) {
		showDashboard("server");
	} else {
		showSetup();
	}
}

// ---- run ----
async function handleRun() {
	const creds = loadCreds();
	if (!creds && !serverMode) {
		showSetup();
		return;
	}
	const body = creds ? creds : {}; // empty body => server uses default creds
	$("runStatus").className = "status";
	$("runStatus").textContent = "Agents working…";
	$("runBtn").disabled = true;
	try {
		const res = await fetch("/run", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(body),
		});
		if (!res.ok) {
			const detail = (await res.text()).trim();
			throw new Error(detail || "Request failed (" + res.status + ")");
		}
		renderDashboard(await res.json());
		$("runStatus").textContent = "";
	} catch (e) {
		$("runStatus").className = "status status-error";
		$("runStatus").textContent =
			"Couldn't get recommendations: " + e.message +
			" — check your credentials.";
	} finally {
		$("runBtn").disabled = false;
	}
}

// ---- render ----
function appendCell(tr, value) {
	const td = document.createElement("td");
	td.textContent = value;
	tr.appendChild(td);
}
function renderDashboard(data) {
	lastRecommendations = data.recommendations || [];
	$("results").hidden = false;
	$("narrative").textContent = data.narrative || "";

	const body = $("reorderBody");
	body.textContent = "";
	lastRecommendations.forEach((r) => {
		const tr = document.createElement("tr");
		appendCell(tr, r.Product.Name);
		appendCell(tr, r.Product.StockLevel);
		appendCell(tr, String(r.ReorderQty) + (r.SpoilageRisk ? " ⚠️" : ""));
		appendCell(tr, r.Reason);
		body.appendChild(tr);
	});
	$("csvBtn").disabled = lastRecommendations.length === 0;

	const labels = (data.sales_trend || []).map((d) => d.Date);
	const values = (data.sales_trend || []).map((d) => d.Quantity);
	if (trendChart) trendChart.destroy();
	trendChart = new Chart($("trendChart"), {
		type: "line",
		data: {
			labels,
			datasets: [{
				label: "Units sold",
				data: values,
				tension: 0.3,
				borderColor: "#2563eb",
				backgroundColor: "rgba(37,99,235,0.1)",
				fill: true,
			}],
		},
		options: { plugins: { legend: { display: false } } },
	});

	const list = $("topSellers");
	list.textContent = "";
	(data.top_sellers || []).forEach((t) => {
		const li = document.createElement("li");
		li.textContent = `${t.Name} — ${t.Units} units`;
		list.appendChild(li);
	});
}

// ---- CSV export (RFC-4180 escaping) ----
function csvCell(v) {
	return '"' + String(v).replace(/"/g, '""') + '"';
}
function buildReorderCSV(recs) {
	const header = ["Product", "Current stock", "Reorder qty", "Spoilage risk", "Reason"];
	const rows = [header.map(csvCell).join(",")];
	recs.forEach((r) => {
		rows.push([
			csvCell(r.Product.Name),
			csvCell(r.Product.StockLevel),
			csvCell(r.ReorderQty),
			csvCell(r.SpoilageRisk ? "yes" : "no"),
			csvCell(r.Reason),
		].join(","));
	});
	return rows.join("\r\n");
}
function todayStamp() {
	const d = new Date();
	return (
		d.getFullYear() +
		"-" + String(d.getMonth() + 1).padStart(2, "0") +
		"-" + String(d.getDate()).padStart(2, "0")
	);
}
function handleDownloadCSV() {
	if (lastRecommendations.length === 0) return;
	const csv = buildReorderCSV(lastRecommendations);
	const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
	const url = URL.createObjectURL(blob);
	const a = document.createElement("a");
	a.href = url;
	a.download = "reorder-" + todayStamp() + ".csv";
	document.body.appendChild(a);
	a.click();
	a.remove();
	URL.revokeObjectURL(url);
}

// ---- wire up (scripts are deferred, so the DOM is ready here) ----
$("saveBtn").addEventListener("click", handleSave);
$("useOwnBtn").addEventListener("click", handleUseOwn);
$("resetBtn").addEventListener("click", handleReset);
$("runBtn").addEventListener("click", handleRun);
$("csvBtn").addEventListener("click", handleDownloadCSV);
initView();
```

- [ ] **Step 3: Add minimal style for the header actions**

In `web/style.css`, append:
```css
.topbar-actions { display: flex; align-items: center; gap: 0.75rem; }
```

- [ ] **Step 4: Verify served + structural check**

Run:
```bash
go build ./... && (PORT=8096 go run ./cmd/retail-agent serve-web &) && sleep 2 && \
  curl -s localhost:8096/app.js | grep -c "serverHasCreds" && \
  curl -s localhost:8096/ | grep -c 'id="useOwnBtn"' && \
  curl -s localhost:8096/config && \
  pkill -f "retail-agent serve-web"
```
Expected: `1`, `1`, and a JSON `{"server_credentials":false}` (no env creds in this shell). If `node` is present, `node --check web/app.js` exits 0.

- [ ] **Step 5: Commit**

```bash
git add web/index.html web/app.js web/style.css
git commit -m "feat(web): /config-driven setup with server-default mode and browser override"
```

---

## Task 5: Dockerfile + Cloud Run deploy docs

**Files:**
- Create: `Dockerfile`
- Modify: `README.md`, `AGENTS.md`

**Interfaces:** none (build + docs).

- [ ] **Step 1: Create the Dockerfile**

Create `Dockerfile`:
```dockerfile
# syntax=docker/dockerfile:1

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /retail-agent ./cmd/retail-agent

FROM gcr.io/distroless/base-debian12
COPY --from=build /retail-agent /retail-agent
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/retail-agent"]
CMD ["serve-web"]
```

- [ ] **Step 2: Verify the image builds and serves (if Docker is available)**

Run:
```bash
docker build -t retail-agent:dev . && \
  docker run --rm -d -p 8097:8080 --name ra-smoke retail-agent:dev && sleep 3 && \
  curl -s localhost:8097/health && echo && \
  curl -s localhost:8097/config && echo && \
  docker rm -f ra-smoke
```
Expected: `ok` and `{"server_credentials":false}` (no creds passed). If Docker is unavailable in this environment, skip the run and note it — the multi-stage build is standard and the binary embeds the web assets (verified in Task 2).

- [ ] **Step 3: Add deploy docs**

Append a "Deploy to Cloud Run" section to `README.md`:
```markdown
## Deploy to Cloud Run

The container serves the web app with server-side default credentials, so the
deployed app shows the dashboard with no setup form.

1. Store the Gemini key in Secret Manager:
   ```bash
   printf '%s' "$GOOGLE_API_KEY" | gcloud secrets create GOOGLE_API_KEY --data-file=-
   ```
2. Deploy (BigQuery is read via the service account's ambient credentials):
   ```bash
   gcloud run deploy retail-agent \
     --source . \
     --region <REGION> \
     --service-account retail-agent@<PROJECT>.iam.gserviceaccount.com \
     --set-env-vars GCLOUD_PROJECT=<PROJECT>,BIGQUERY_DATASET=<PROJECT>.retail \
     --set-secrets GOOGLE_API_KEY=GOOGLE_API_KEY:latest \
     --allow-unauthenticated
   ```
3. The service account needs **BigQuery Data Viewer** + **BigQuery Job User** on the dataset's project.

Locally, the same defaults apply when `.env` exports `GOOGLE_API_KEY`,
`GCLOUD_PROJECT`, `BIGQUERY_DATASET`, and `GOOGLE_APPLICATION_CREDENTIALS`
(a service-account JSON): `source .env && go run ./cmd/retail-agent serve-web`
then open http://localhost:8080 — the dashboard appears with no setup form.
```
In `AGENTS.md`, add a Commands row:
```
| Build container | `docker build -t retail-agent .` |
| Deploy to Cloud Run | `gcloud run deploy retail-agent --source . ...` (see README) |
```
And add one line under "Notes for agents":
```
- `serve-web` reports `server_credentials: true` via `GET /config` when `GOOGLE_API_KEY` + ADC BigQuery (`GCLOUD_PROJECT`/`BIGQUERY_DATASET`) are present; the web then skips the setup form. `web/` assets are embedded via `go:embed` (`web/web.go`).
```

- [ ] **Step 4: Full CI check**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile README.md AGENTS.md
git commit -m "feat(deploy): Dockerfile and Cloud Run deploy docs"
```

---

## Self-Review

**Spec coverage:**
- Remove LLM-as-judge, keep 4 deterministic cases → Task 1. ✅
- Server-default detection + `GET /config` + dual-mode `/run` → Task 3. ✅
- Gemini API-key-from-env + ADC BigQuery defaults → Task 3 Step 4 (`GOOGLE_API_KEY` + `NewBigQueryFromEnv`). ✅
- Frontend precedence (browser → server → setup) + browser override → Task 4. ✅
- Embed web assets + `FileServerFS` (container-safe) → Task 2. ✅
- Dockerfile + Cloud Run deploy docs → Task 5. ✅
- `serve-web` starts with no creds → Task 3 Step 4 (logs + serves setup flow when defaults unavailable); Task 4 view falls back to setup. ✅

**Placeholder scan:** No TBD/TODO; every code step is complete. `<REGION>`/`<PROJECT>` are deploy-time placeholders in docs, expected. ✅

**Type/name consistency:** `server.Defaults{APIKey, Store}` + `Available()` + `Handler(run, defaults)` used consistently across Task 3 and main.go wiring. `GET /config` JSON key `server_credentials` matches the frontend `cfg.server_credentials` in Task 4. `web.Files` embed used in Task 2 main.go. `/run` empty-body → server-default path (Task 3) matches the frontend empty-body post (Task 4). CSV/render field names unchanged from the existing frontend. ✅
