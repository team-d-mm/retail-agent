# Web UX: Credential Setup, Redesign, CSV Export — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the retail-agent web frontend so credentials are collected once and kept in the browser (never re-prompted, never re-shown), the UI is a clean light SaaS dashboard, and the reorder list can be downloaded as CSV.

**Architecture:** Frontend only — rewrite `web/index.html`, `web/app.js`, `web/style.css`. Two mutually exclusive views (Setup / Dashboard) toggled by whether credentials exist in `localStorage`. The `POST /run` request body and the backend are unchanged; the server already starts with no credentials.

**Tech Stack:** Static HTML/CSS/vanilla JS, Chart.js 4 (CDN), browser `localStorage`, `Blob` download API.

## Global Constraints

- Files touched: only `web/index.html`, `web/app.js`, `web/style.css`. No Go changes.
- `POST /run` body stays `{ai_studio_key, service_account_json, dataset_url}`; response stays `{recommendations, narrative, sales_trend, top_sellers}`.
- Response field access (verbatim, no JSON tags on the Go structs so fields are capitalized): `recommendations[].Product.Name`, `recommendations[].Product.StockLevel`, `recommendations[].ReorderQty`, `recommendations[].SpoilageRisk`, `recommendations[].Reason`; `sales_trend[].Date`, `sales_trend[].Quantity`; `top_sellers[].Name`, `top_sellers[].Units`.
- Credentials stored ONLY in `localStorage` under key `retailAgentCreds`; the server never receives them except in the `/run` body. Secret values are never written back into any input.
- All server-supplied strings rendered via `textContent` / DOM construction — never `innerHTML` (XSS-safe, matches the existing pattern).
- CI must stay green: `go build ./... && go vet ./... && go test ./...` (regression guard; no Go changes).
- `serve-web` starts with only `PORT` and requires no credentials — do not change that.

---

## File Structure

- `web/index.html` (rewrite) — page shell: sticky header with brand + Reset button; Setup view (one-time form); Dashboard view (run button + three result cards). Element ids are the contract with `app.js`.
- `web/app.js` (rewrite) — credential storage, view toggle, setup/reset/run handlers, dashboard render, CSV export.
- `web/style.css` (rewrite) — clean light SaaS styling.

The three files are one deliverable but independently reviewable (structure vs. behavior vs. presentation). Each is committed separately; full behavioral verification is Task 4.

**Element-id contract** (index.html defines, app.js consumes): `resetBtn`, `setupView`, `dashboardView`, `aiKey`, `saJson`, `datasetUrl`, `saveBtn`, `setupStatus`, `runBtn`, `runStatus`, `results`, `narrative`, `reorderBody`, `csvBtn`, `trendChart`, `topSellers`.

---

## Task 1: Rewrite `web/index.html` (two-view shell)

**Files:**
- Modify (full rewrite): `web/index.html`

**Interfaces:**
- Produces: the element-id contract above; loads `/style.css`, Chart.js CDN, and `/app.js` (both scripts `defer`).
- Consumes: nothing (static).

- [ ] **Step 1: Replace the file with the full markup**

```html
<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>Retail Agent — Reorder Assistant</title>
	<link rel="stylesheet" href="/style.css" />
	<script src="https://cdn.jsdelivr.net/npm/chart.js@4" defer></script>
	<script src="/app.js" defer></script>
</head>
<body>
	<header class="topbar">
		<div class="brand">
			<span class="brand-mark">🛒</span>
			<span class="brand-name">Retail Agent</span>
		</div>
		<button id="resetBtn" class="btn btn-ghost" hidden>Reset credentials</button>
	</header>

	<main class="container">
		<!-- SETUP VIEW -->
		<section id="setupView" class="view">
			<div class="card setup-card">
				<h1 class="card-title">Welcome — one-time setup</h1>
				<p class="muted">Enter your credentials once. They are stored only in this browser and are sent only to your own Google services.</p>

				<label class="field">
					<span class="field-label">Google AI Studio API key</span>
					<input id="aiKey" class="input" type="password" autocomplete="off" placeholder="AIza…" />
					<span class="field-hint">Get one at <a href="https://aistudio.google.com/app/apikey" target="_blank" rel="noreferrer">aistudio.google.com</a></span>
				</label>

				<label class="field">
					<span class="field-label">Service account JSON</span>
					<textarea id="saJson" class="input" rows="6" autocomplete="off" placeholder='{ "type": "service_account", … }'></textarea>
				</label>

				<label class="field">
					<span class="field-label">BigQuery dataset URL</span>
					<input id="datasetUrl" class="input" type="text" placeholder="my-project.retail" />
				</label>

				<button id="saveBtn" class="btn btn-primary btn-block">Save &amp; continue</button>
				<p id="setupStatus" class="status status-error"></p>
			</div>
		</section>

		<!-- DASHBOARD VIEW -->
		<section id="dashboardView" class="view" hidden>
			<div class="dashboard-head">
				<div>
					<h1 class="page-title">Reorder recommendations</h1>
					<p class="muted">One click reads your inventory and sales, then tells you what to buy.</p>
				</div>
				<button id="runBtn" class="btn btn-primary">See reorder recommendations</button>
			</div>
			<p id="runStatus" class="status"></p>

			<div id="results" hidden>
				<section class="card">
					<div class="card-head">
						<h2 class="card-title">Reorder now</h2>
						<button id="csvBtn" class="btn btn-secondary" disabled>Download CSV</button>
					</div>
					<p id="narrative" class="narrative muted"></p>
					<div class="table-wrap">
						<table class="data-table">
							<thead><tr><th>Product</th><th>Current stock</th><th>Order qty</th><th>Reason</th></tr></thead>
							<tbody id="reorderBody"></tbody>
						</table>
					</div>
				</section>

				<section class="card">
					<h2 class="card-title">Daily sales trend</h2>
					<canvas id="trendChart" height="120"></canvas>
				</section>

				<section class="card">
					<h2 class="card-title">Top selling products</h2>
					<ol id="topSellers" class="top-list"></ol>
				</section>
			</div>
		</section>
	</main>
</body>
</html>
```

- [ ] **Step 2: Verify the page is served and structurally correct**

Run:
```bash
go build ./... && (PORT=8091 go run ./cmd/retail-agent serve-web &) && sleep 2 && \
  curl -s localhost:8091/ | grep -c 'id="setupView"' && \
  curl -s localhost:8091/ | grep -c 'id="dashboardView"' && \
  curl -s localhost:8091/health && \
  pkill -f "retail-agent serve-web"
```
Expected: two `1` counts (both views present), `ok` from health. (App.js/CSS may not match yet — that's Tasks 2–3; this checks the HTML is served.)

- [ ] **Step 3: Commit**

```bash
git add web/index.html
git commit -m "feat(web): two-view shell with header, setup card, dashboard cards"
```

---

## Task 2: Rewrite `web/app.js` (storage, views, run, render, CSV)

**Files:**
- Modify (full rewrite): `web/app.js`

**Interfaces:**
- Consumes: the element-id contract from Task 1; `POST /run`.
- Produces: `localStorage["retailAgentCreds"]` = `{ai_studio_key, service_account_json, dataset_url}`; a `reorder-YYYY-MM-DD.csv` download.

- [ ] **Step 1: Replace the file with the full script**

```js
const STORAGE_KEY = "retailAgentCreds";
const $ = (id) => document.getElementById(id);
let trendChart = null;
let lastRecommendations = [];

// ---- credential storage (browser-only; never persisted server-side) ----
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

// ---- view toggle ----
function showSetup() {
	$("setupView").hidden = false;
	$("dashboardView").hidden = true;
	$("resetBtn").hidden = true;
}
function showDashboard() {
	$("setupView").hidden = true;
	$("dashboardView").hidden = false;
	$("resetBtn").hidden = false;
}
function initView() {
	if (loadCreds()) showDashboard();
	else showSetup();
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
	// Never leave secrets in the DOM after saving.
	$("aiKey").value = "";
	$("saJson").value = "";
	$("datasetUrl").value = "";
	$("setupStatus").textContent = "";
	showDashboard();
}

// ---- reset ----
function handleReset() {
	clearCreds();
	lastRecommendations = [];
	$("results").hidden = true;
	$("runStatus").textContent = "";
	if (trendChart) {
		trendChart.destroy();
		trendChart = null;
	}
	showSetup();
}

// ---- run ----
async function handleRun() {
	const creds = loadCreds();
	if (!creds) {
		showSetup();
		return;
	}
	$("runStatus").className = "status";
	$("runStatus").textContent = "Agents working…";
	$("runBtn").disabled = true;
	try {
		const res = await fetch("/run", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(creds),
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
			" — check your credentials (use “Reset credentials” above to re-enter).";
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
$("resetBtn").addEventListener("click", handleReset);
$("runBtn").addEventListener("click", handleRun);
$("csvBtn").addEventListener("click", handleDownloadCSV);
initView();
```

- [ ] **Step 2: Verify served + no syntax error**

Run:
```bash
go build ./... && (PORT=8092 go run ./cmd/retail-agent serve-web &) && sleep 2 && \
  curl -s localhost:8092/app.js | grep -c "retailAgentCreds" && \
  curl -s localhost:8092/app.js | grep -c "buildReorderCSV" && \
  pkill -f "retail-agent serve-web"
```
Expected: both counts `1` (file served with the storage key and CSV builder present). Then confirm JS parses: if `node` is available, `node --check web/app.js` prints nothing and exits 0; if `node` is absent, skip this sub-check (browser smoke in Task 4 covers it).

- [ ] **Step 3: Commit**

```bash
git add web/app.js
git commit -m "feat(web): one-time localStorage credential setup, reset, and CSV export"
```

---

## Task 3: Rewrite `web/style.css` (clean light SaaS)

**Files:**
- Modify (full rewrite): `web/style.css`

**Interfaces:**
- Consumes: the class names used in `index.html` (`topbar`, `brand`, `card`, `btn btn-primary/secondary/ghost`, `field`, `input`, `data-table`, `status`, etc.).
- Produces: nothing consumed by JS.

- [ ] **Step 1: Replace the file with the full stylesheet**

```css
:root {
	--bg: #f5f7fb;
	--surface: #ffffff;
	--border: #e5e9f0;
	--text: #1f2937;
	--muted: #6b7280;
	--accent: #2563eb;
	--accent-hover: #1d4ed8;
	--danger: #b91c1c;
	--radius: 14px;
	--shadow: 0 1px 2px rgba(16, 24, 40, 0.04), 0 4px 16px rgba(16, 24, 40, 0.06);
	--font: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
}
* { box-sizing: border-box; }
body {
	margin: 0;
	font-family: var(--font);
	color: var(--text);
	background: var(--bg);
	line-height: 1.5;
}

.topbar {
	display: flex;
	align-items: center;
	justify-content: space-between;
	padding: 0.9rem 1.5rem;
	background: var(--surface);
	border-bottom: 1px solid var(--border);
	position: sticky;
	top: 0;
	z-index: 10;
}
.brand { display: flex; align-items: center; gap: 0.55rem; font-weight: 700; }
.brand-mark { font-size: 1.3rem; }
.brand-name { font-size: 1.05rem; letter-spacing: -0.01em; }

.container { max-width: 880px; margin: 0 auto; padding: 1.75rem 1.25rem 3rem; }
.view { animation: fade 0.2s ease; }
@keyframes fade {
	from { opacity: 0; transform: translateY(4px); }
	to { opacity: 1; transform: none; }
}

.card {
	background: var(--surface);
	border: 1px solid var(--border);
	border-radius: var(--radius);
	box-shadow: var(--shadow);
	padding: 1.5rem;
	margin-bottom: 1.25rem;
}
.setup-card { max-width: 520px; margin: 2rem auto; }
.card-head {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 1rem;
	margin-bottom: 0.75rem;
}
.card-title { font-size: 1.1rem; margin: 0 0 0.75rem; }
.card-head .card-title { margin: 0; }
.page-title { font-size: 1.5rem; margin: 0 0 0.25rem; letter-spacing: -0.02em; }
.muted { color: var(--muted); }

.field { display: block; margin: 1rem 0; }
.field-label { display: block; font-weight: 600; font-size: 0.9rem; margin-bottom: 0.35rem; }
.field-hint { display: block; font-size: 0.8rem; color: var(--muted); margin-top: 0.3rem; }
.input {
	width: 100%;
	padding: 0.6rem 0.7rem;
	border: 1px solid var(--border);
	border-radius: 9px;
	font: inherit;
	background: #fff;
	transition: border-color 0.15s, box-shadow 0.15s;
}
.input:focus {
	outline: none;
	border-color: var(--accent);
	box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.15);
}
textarea.input {
	resize: vertical;
	font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	font-size: 0.85rem;
}

.btn {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	gap: 0.4rem;
	padding: 0.6rem 1.1rem;
	border: 1px solid transparent;
	border-radius: 9px;
	font: inherit;
	font-weight: 600;
	cursor: pointer;
	transition: background 0.15s, border-color 0.15s, opacity 0.15s;
}
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: var(--accent); color: #fff; }
.btn-primary:hover:not(:disabled) { background: var(--accent-hover); }
.btn-secondary { background: #fff; color: var(--accent); border-color: var(--accent); }
.btn-secondary:hover:not(:disabled) { background: rgba(37, 99, 235, 0.06); }
.btn-ghost { background: transparent; color: var(--muted); border-color: var(--border); }
.btn-ghost:hover:not(:disabled) { color: var(--text); border-color: var(--muted); }
.btn-block { width: 100%; margin-top: 0.5rem; }

.dashboard-head {
	display: flex;
	align-items: flex-start;
	justify-content: space-between;
	gap: 1rem;
	margin-bottom: 0.75rem;
	flex-wrap: wrap;
}

.status { min-height: 1.3rem; margin: 0.5rem 0 1rem; color: var(--muted); font-size: 0.9rem; }
.status-error { color: var(--danger); }

.narrative { margin: 0 0 1rem; white-space: pre-wrap; }

.table-wrap { overflow-x: auto; }
.data-table { width: 100%; border-collapse: collapse; font-size: 0.92rem; }
.data-table th, .data-table td {
	text-align: left;
	padding: 0.6rem 0.7rem;
	border-bottom: 1px solid var(--border);
}
.data-table th {
	font-size: 0.78rem;
	text-transform: uppercase;
	letter-spacing: 0.03em;
	color: var(--muted);
	font-weight: 600;
}
.data-table tbody tr:last-child td { border-bottom: none; }
.data-table tbody tr:hover { background: #fafbfe; }

.top-list { margin: 0; padding-left: 1.2rem; }
.top-list li { padding: 0.25rem 0; }

@media (max-width: 640px) {
	.dashboard-head { flex-direction: column; }
	.dashboard-head .btn { width: 100%; }
}
```

- [ ] **Step 2: Verify served + CI green**

Run:
```bash
go build ./... && go vet ./... && go test ./... 2>&1 | tail -3 && \
  (PORT=8093 go run ./cmd/retail-agent serve-web &) && sleep 2 && \
  curl -s localhost:8093/style.css | grep -c "btn-primary" && \
  pkill -f "retail-agent serve-web"
```
Expected: tests pass; count `1` (stylesheet served with expected classes).

- [ ] **Step 3: Commit**

```bash
git add web/style.css
git commit -m "feat(web): clean light SaaS styling for setup and dashboard"
```

---

## Task 4: Integrated verification

**Files:** none (verification only).

**Interfaces:** none.

This task confirms the three files work together. The automated part can run headless; the behavioral part requires a browser and is a checklist for the human/controller.

- [ ] **Step 1: Automated serve check**

```bash
go build ./... && (PORT=8094 go run ./cmd/retail-agent serve-web &) && sleep 2 && \
  echo "-- index --" && curl -s localhost:8094/ | grep -Ec 'id="(setupView|dashboardView|runBtn|csvBtn|reorderBody)"' && \
  echo "-- app --"   && curl -s localhost:8094/app.js | grep -c "initView" && \
  echo "-- css --"   && curl -s localhost:8094/style.css | grep -c ":root" && \
  echo "-- health --" && curl -s localhost:8094/health && \
  pkill -f "retail-agent serve-web"
```
Expected: index count `5` (all five ids present), app `1`, css `1`, health `ok`.

- [ ] **Step 2: Manual browser smoke (human/controller)**

Start the app: `go run ./cmd/retail-agent serve-web`, open http://localhost:8080, then verify:
1. **Fresh browser** (clear site data / private window): Setup view shows; the "Reset credentials" header button is hidden.
2. **Save** with one field blank → inline "Please fill in all three fields." Save with an invalid JSON blob in the service-account field → "Service account JSON is not valid JSON."
3. **Save valid-looking creds** → Dashboard view; header now shows "Reset credentials". **Reload the page** → still Dashboard, no setup, and no field is pre-filled with the secret.
4. **Reset credentials** → back to Setup; reload → still Setup (localStorage cleared).
5. With the seeded BigQuery dataset + real creds, press **See reorder recommendations** → the three cards populate; a perishable (e.g. Milk) shows the ⚠️ spoilage cap.
6. **Download CSV** → `reorder-YYYY-MM-DD.csv` downloads; open it: the header row and one row per reorder item, `Spoilage risk` = `yes`/`no`, and any product name/reason containing a comma or quote is wrapped/escaped correctly. The button is disabled when there are no recommendations.
7. On a bad credential (e.g. wrong dataset), the run shows an inline error mentioning "Reset credentials", and the dashboard stays usable.

- [ ] **Step 3: No commit** (verification only; if a fix is needed, it belongs to Task 1–3's file and is committed there).

---

## Self-Review

**Spec coverage:**
- Req 1 (no creds at startup, backend + frontend serve without input) → unchanged backend + `initView()` renders a view with no creds needed; Task 4 Step 1 confirms serving. ✅
- Req 2 (credentials requested once on first use) → Setup view shown only when `loadCreds()` is null (Task 2). ✅
- Req 3 (never shown again; secrets not echoed) → inputs cleared after save; Dashboard has no secret fields; reload stays on Dashboard (Task 2, Task 4 Step 2.3). ✅
- Req 4 (kept for later visits) → `localStorage` persistence (Task 2). ✅
- Req 5 (modern elegant UI) → clean light SaaS stylesheet + card layout (Tasks 1, 3). ✅
- Req 6 (CSV download of order items) → `csvBtn` + `buildReorderCSV`/`handleDownloadCSV` (Task 2). ✅

**Placeholder scan:** No TBD/TODO; every code step contains the complete file. ✅

**Type/contract consistency:** Element ids in Task 1 markup exactly match `$()` lookups in Task 2 (`resetBtn`, `setupView`, `dashboardView`, `aiKey`, `saJson`, `datasetUrl`, `saveBtn`, `setupStatus`, `runBtn`, `runStatus`, `results`, `narrative`, `reorderBody`, `csvBtn`, `trendChart`, `topSellers`). CSS class names in Task 3 match those in Task 1. Response field access matches the Global Constraints list. ✅
