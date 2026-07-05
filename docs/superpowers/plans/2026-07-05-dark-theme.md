# Switchable Dark Theme Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a header toggle that switches the dashboard between light and dark, remembered across visits (default light).

**Architecture:** The stylesheet already drives all colors from CSS variables on `:root`; dark mode overrides those variables under `html[data-theme="dark"]`. A no-flash inline head script applies the saved theme before paint; `app.js` wires the toggle and makes the Chart.js trend chart follow the theme.

**Tech Stack:** Static HTML/CSS/vanilla JS, CSS custom properties, Chart.js 4, `localStorage`.

## Global Constraints

- Frontend only: `web/index.html`, `web/app.js`, `web/style.css`. No Go changes.
- Default is light; dark only when toggled; choice persisted in `localStorage["retailAgentTheme"]` (`"dark"`/`"light"`). No `prefers-color-scheme` detection.
- Dark applies via `html[data-theme="dark"]`; light = attribute absent.
- Server strings still rendered via `textContent` (unchanged); `localStorage` access wrapped in try/catch.
- CI must stay green: `go build ./... && go vet ./... && go test ./...` (no Go changes — regression guard).

---

## File Structure

- `web/index.html` (modify) — no-flash inline head script + `themeBtn` in the header.
- `web/style.css` (modify) — parameterize the few hardcoded light colors, add the dark palette.
- `web/app.js` (modify) — `currentTheme`/`applyTheme`/`toggleTheme`, theme-aware chart, wireup.

---

## Task 1: Theme palette + toggle button (index.html + style.css)

**Files:**
- Modify: `web/index.html`
- Modify: `web/style.css`

**Interfaces:**
- Produces: `html[data-theme="dark"]` palette; `#themeBtn` header button; a no-flash head script reading `localStorage["retailAgentTheme"]`. `app.js` (Task 2) consumes `#themeBtn` and the CSS variables `--muted`/`--border` for chart colors.

- [ ] **Step 1: Add the no-flash script and toggle button to index.html**

In `web/index.html`, insert the theme script in `<head>` immediately after the viewport meta and **before** the stylesheet link, so the attribute is set before CSS applies:
```html
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<script>
		try {
			if (localStorage.getItem("retailAgentTheme") === "dark") {
				document.documentElement.dataset.theme = "dark";
			}
		} catch (e) {}
	</script>
	<title>Retail Agent — Reorder Assistant</title>
```
Then add the toggle as the first control in `.topbar-actions`:
```html
		<div class="topbar-actions">
			<button id="themeBtn" class="btn btn-ghost" aria-label="Toggle theme">🌙</button>
			<span id="credMode" class="muted"></span>
			<button id="useOwnBtn" class="btn btn-ghost" hidden>Use my own credentials</button>
			<button id="resetBtn" class="btn btn-ghost" hidden>Reset credentials</button>
		</div>
```

- [ ] **Step 2: Parameterize hardcoded colors + add the dark palette in style.css**

In `web/style.css`, add two variables to the existing `:root` block (after `--danger: #b91c1c;`):
```css
	--input-bg: #ffffff;
	--row-hover: #fafbfe;
```
Immediately after the closing `}` of the `:root` block, add the dark palette:
```css
html[data-theme="dark"] {
	--bg: #0f172a;
	--surface: #1e293b;
	--border: #334155;
	--text: #e2e8f0;
	--muted: #94a3b8;
	--input-bg: #0b1220;
	--row-hover: #243044;
	--shadow: 0 1px 2px rgba(0, 0, 0, 0.3), 0 4px 16px rgba(0, 0, 0, 0.45);
}
```
Then replace the three hardcoded light colors so they use the variables:
- In `.input`, change `background: #fff;` to `background: var(--input-bg);`
- In `.btn-secondary`, change `background: #fff;` to `background: var(--surface);`
- In `.data-table tbody tr:hover`, change `background: #fafbfe;` to `background: var(--row-hover);`

(The keys stay; only those three `background` values change. Everything else already references variables and needs no edit.)

- [ ] **Step 3: Verify served + dark palette present**

Run:
```bash
go build ./... && (PORT=8098 go run ./cmd/retail-agent serve-web &) && sleep 2 && \
  curl -s localhost:8098/ | grep -c 'id="themeBtn"' && \
  curl -s localhost:8098/ | grep -c 'retailAgentTheme' && \
  curl -s localhost:8098/style.css | grep -c 'data-theme="dark"' && \
  pkill -f "retail-agent serve-web"
```
Expected: `1` (toggle button), `1` (head script references the key), `1` (dark palette rule present).

- [ ] **Step 4: Commit**

```bash
git add web/index.html web/style.css
git commit -m "feat(web): dark theme palette, no-flash head script, header toggle"
```

---

## Task 2: Theme logic + theme-aware chart (app.js)

**Files:**
- Modify: `web/app.js`

**Interfaces:**
- Consumes: `#themeBtn` and the dark palette from Task 1; `localStorage["retailAgentTheme"]`.

- [ ] **Step 1: Add a theme state variable**

In `web/app.js`, near the other top-level `let` declarations (`let trendChart = null;` / `let lastRecommendations = [];` / `let serverMode = false;`), add:
```js
let lastTrend = null; // {labels, values} for re-drawing the chart on theme change
```

- [ ] **Step 2: Add the theme helpers**

Add these functions to `web/app.js` (anywhere at top level, e.g. just above `// ---- view toggle ----`):
```js
// ---- theme ----
function currentTheme() {
	try {
		return localStorage.getItem("retailAgentTheme") === "dark" ? "dark" : "light";
	} catch {
		return "light";
	}
}
function applyTheme(theme) {
	if (theme === "dark") {
		document.documentElement.dataset.theme = "dark";
	} else {
		delete document.documentElement.dataset.theme;
	}
	try {
		localStorage.setItem("retailAgentTheme", theme);
	} catch {}
	$("themeBtn").textContent = theme === "dark" ? "☀️" : "🌙";
}
function toggleTheme() {
	applyTheme(currentTheme() === "dark" ? "light" : "dark");
	drawChart(); // re-render with theme colors if a chart is shown
}
function themeColor(varName) {
	return getComputedStyle(document.documentElement).getPropertyValue(varName).trim();
}
```

- [ ] **Step 3: Add a theme-aware chart renderer and use it**

Add a `drawChart` function (top level, near the theme helpers):
```js
function drawChart() {
	if (!lastTrend) return;
	const tick = themeColor("--muted");
	const grid = themeColor("--border");
	if (trendChart) trendChart.destroy();
	trendChart = new Chart($("trendChart"), {
		type: "line",
		data: {
			labels: lastTrend.labels,
			datasets: [{
				label: "Units sold",
				data: lastTrend.values,
				tension: 0.3,
				borderColor: "#2563eb",
				backgroundColor: "rgba(37,99,235,0.1)",
				fill: true,
			}],
		},
		options: {
			plugins: { legend: { display: false } },
			scales: {
				x: { ticks: { color: tick }, grid: { color: grid } },
				y: { ticks: { color: tick }, grid: { color: grid } },
			},
		},
	});
}
```
Then, inside `renderDashboard`, replace the existing inline chart block:
```js
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
```
with:
```js
	lastTrend = {
		labels: (data.sales_trend || []).map((d) => d.Date),
		values: (data.sales_trend || []).map((d) => d.Quantity),
	};
	drawChart();
```

- [ ] **Step 4: Also clear the cached trend on reset**

In `handleReset`, where it currently sets `lastRecommendations = []` and destroys the chart, also reset the cached trend. Change the line `lastRecommendations = [];` to:
```js
	lastRecommendations = [];
	lastTrend = null;
```

- [ ] **Step 5: Wire the toggle and sync the button label on load**

At the bottom of `web/app.js`, in the wire-up block, add the theme listener and a load-time sync. Change:
```js
$("saveBtn").addEventListener("click", handleSave);
$("useOwnBtn").addEventListener("click", handleUseOwn);
$("resetBtn").addEventListener("click", handleReset);
$("runBtn").addEventListener("click", handleRun);
$("csvBtn").addEventListener("click", handleDownloadCSV);
initView();
```
to:
```js
$("themeBtn").addEventListener("click", toggleTheme);
$("saveBtn").addEventListener("click", handleSave);
$("useOwnBtn").addEventListener("click", handleUseOwn);
$("resetBtn").addEventListener("click", handleReset);
$("runBtn").addEventListener("click", handleRun);
$("csvBtn").addEventListener("click", handleDownloadCSV);
applyTheme(currentTheme()); // sync button label; head script already set the attribute
initView();
```

- [ ] **Step 6: Verify served + syntax**

Run:
```bash
go build ./... && go vet ./... && go test ./... 2>&1 | tail -3 && \
  (PORT=8099 go run ./cmd/retail-agent serve-web &) && sleep 2 && \
  curl -s localhost:8099/app.js | grep -c "toggleTheme" && \
  pkill -f "retail-agent serve-web"
```
Expected: tests pass; count `1`. If `node` is present, `node --check web/app.js` exits 0.

- [ ] **Step 7: Commit**

```bash
git add web/app.js
git commit -m "feat(web): theme toggle logic and theme-aware trend chart"
```

---

## Task 3: Manual verification

**Files:** none.

- [ ] **Step 1: Manual browser smoke (human/controller)**

`go run ./cmd/retail-agent serve-web`, open http://localhost:8080:
1. First-ever visit (clear site data) → light theme; header shows 🌙.
2. Click the toggle → whole UI goes dark, button shows ☀️; reload → still dark (persisted), no flash of light on load.
3. Toggle back → light; reload → light.
4. Run recommendations (or in server mode) → the trend chart is legible in the current theme; toggle while the dashboard is shown → chart re-renders with theme-appropriate axis/grid colors.
5. Toggle works in both the setup view and the dashboard view.

- [ ] **Step 2: No commit** (verification only).

---

## Self-Review

**Spec coverage:**
- Header toggle in `.topbar-actions`, both views → Task 1 (`#themeBtn`). ✅
- Dark palette overriding CSS variables under `html[data-theme="dark"]` → Task 1. ✅
- Persistence in `localStorage["retailAgentTheme"]`, default light → Task 2 `applyTheme`/`currentTheme`. ✅
- No flash of wrong theme → Task 1 head script (runs before stylesheet/paint). ✅
- Chart follows theme, re-renders on toggle → Task 2 `drawChart` + `toggleTheme`. ✅
- try/catch around localStorage → Task 2. ✅

**Placeholder scan:** No TBD/TODO; every step has complete code or exact edits. ✅

**Type/name consistency:** `#themeBtn` (Task 1 HTML) consumed by Task 2 wireup + `applyTheme`. `retailAgentTheme` key identical in the head script (Task 1) and `currentTheme`/`applyTheme` (Task 2). `data-theme="dark"` attribute set by both the head script and `applyTheme`, matched by the CSS selector. `lastTrend` set in `renderDashboard`, read in `drawChart`, cleared in `handleReset`. CSS variables `--muted`/`--border` (read by `themeColor`) exist in both palettes. ✅
