const STORAGE_KEY = "retailAgentCreds";
const $ = (id) => document.getElementById(id);
let trendChart = null;
let lastRecommendations = [];
let serverMode = false; // server has default credentials
let lastTrend = null; // {labels, values} for re-drawing the chart on theme change

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
	lastTrend = null;
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

	lastTrend = {
		labels: (data.sales_trend || []).map((d) => d.Date),
		values: (data.sales_trend || []).map((d) => d.Quantity),
	};
	drawChart();

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
$("themeBtn").addEventListener("click", toggleTheme);
$("saveBtn").addEventListener("click", handleSave);
$("useOwnBtn").addEventListener("click", handleUseOwn);
$("resetBtn").addEventListener("click", handleReset);
$("runBtn").addEventListener("click", handleRun);
$("csvBtn").addEventListener("click", handleDownloadCSV);
applyTheme(currentTheme()); // sync button label; head script already set the attribute
initView();
