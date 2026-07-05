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
