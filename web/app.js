const $ = (id) => document.getElementById(id);
let trendChart;

$("goBtn").addEventListener("click", async () => {
	const payload = {
		ai_studio_key: $("aiKey").value.trim(),
		service_account_json: $("saJson").value.trim(),
		dataset_url: $("datasetUrl").value.trim(),
	};
	if (!payload.ai_studio_key || !payload.service_account_json || !payload.dataset_url) {
		$("status").textContent = "Please fill in all three fields.";
		return;
	}
	$("status").textContent = "Agents working…";
	$("goBtn").disabled = true;
	try {
		const res = await fetch("/run", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(payload),
		});
		if (!res.ok) throw new Error(await res.text());
		render(await res.json());
		$("status").textContent = "";
	} catch (e) {
		$("status").textContent = "Error: " + e.message;
	} finally {
		$("goBtn").disabled = false;
	}
});

function render(data) {
	$("dashboard").hidden = false;
	$("narrative").textContent = data.narrative || "";

	const tbody = $("reorderTable").querySelector("tbody");
	tbody.innerHTML = "";
	(data.recommendations || []).forEach((r) => {
		const tr = document.createElement("tr");
		const tdName = document.createElement("td");
		tdName.textContent = r.Product.Name;
		const tdStock = document.createElement("td");
		tdStock.textContent = r.Product.StockLevel;
		const tdQty = document.createElement("td");
		tdQty.textContent = String(r.ReorderQty) + (r.SpoilageRisk ? " ⚠️" : "");
		const tdReason = document.createElement("td");
		tdReason.textContent = r.Reason;
		tr.appendChild(tdName);
		tr.appendChild(tdStock);
		tr.appendChild(tdQty);
		tr.appendChild(tdReason);
		tbody.appendChild(tr);
	});

	const labels = (data.sales_trend || []).map((d) => d.Date);
	const values = (data.sales_trend || []).map((d) => d.Quantity);
	if (trendChart) trendChart.destroy();
	trendChart = new Chart($("trendChart"), {
		type: "line",
		data: { labels, datasets: [{ label: "Units sold", data: values }] },
	});

	const ol = $("topSellers");
	ol.innerHTML = "";
	(data.top_sellers || []).forEach((t) => {
		const li = document.createElement("li");
		li.textContent = `${t.Name} — ${t.Units} units`;
		ol.appendChild(li);
	});
}
