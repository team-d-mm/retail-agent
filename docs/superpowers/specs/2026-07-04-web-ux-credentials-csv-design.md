# Web UX: One-Time Credential Setup, Redesign, CSV Export — Design

**Date:** 2026-07-04
**Goal:** Make the retail-agent web app usable and polished: never require credentials at startup, collect them once and keep them safely without re-prompting, modernize the UI, and let the owner download the reorder list as CSV.
**Scope:** Frontend only — `web/index.html`, `web/app.js`, `web/style.css`. No Go changes.

## Background — what already exists

- `cmd/retail-agent serve-web` (`main.go:44`) starts an HTTP server reading only `PORT`. It does **not** build a BigQuery client, agent, or read any credential at startup. It mounts `POST /run`, `GET /health`, and serves `web/` static files.
- `POST /run` accepts `{ai_studio_key, service_account_json, dataset_url}` per request, and returns `{recommendations, narrative, sales_trend, top_sellers}`. This contract is unchanged by this work.
- The current frontend is a single always-visible form + dashboard with minimal styling.

**Requirement 1 (no mandatory credentials at startup, backend and frontend serve without user input) is already satisfied by the backend.** This design verifies that and delivers the remaining, frontend-only requirements.

## Requirements

1. Backend and frontend start and serve with no credentials present. (Already true; verified, not modified.)
2. Credentials are requested **once**, on first use, via a setup screen.
3. After the owner fills them in, the inputs are **never shown again** and the secret values are not echoed back into any field.
4. Credentials are kept so the setup is not repeated on later visits.
5. Modern, elegant UI (clean light SaaS style).
6. A button to download the reorder ("order items") list as a CSV file.

## Decisions

- **Credential storage: browser `localStorage`, client-side only.** The server never persists secrets. Chosen because the app is a single-operator localhost tool; this keeps secrets off the server entirely and satisfies "one-time" + "not shown again" + "kept for later visits". Trade-off: `localStorage` is plaintext and readable by any script on the page — acceptable here because the page loads no third-party scripts except the Chart.js CDN, the data is the operator's own, and the deployment is single-user. (Documented as not a production auth pattern, consistent with the existing README note.)
- **UI style: clean light SaaS** — card panels, soft shadows, rounded corners, a single accent color, system font stack, generous spacing, responsive.

## Architecture / views

The frontend is a single page with two mutually exclusive views, toggled by whether credentials exist in `localStorage`:

```
page load
   │
   ├─ localStorage["retailAgentCreds"] absent ──► SETUP view
   │        (form: AI Studio key, service-account JSON, dataset URL, Save)
   │        on Save (all fields non-empty) ─► store JSON ─► DASHBOARD view
   │
   └─ present ──────────────────────────────────► DASHBOARD view
            header shows "Reset credentials"
            [See reorder recommendations] button
              └─ POST /run with stored creds ─► render 3 cards
```

- **Setup view:** centered card, three labeled fields (key = password input; service-account JSON = textarea; dataset URL = text), a primary **Save & continue** button, and a helper link to https://aistudio.google.com/app/apikey. Inline validation: all three required; the service-account field must parse as JSON (cheap `JSON.parse` guard with a friendly message).
- **Dashboard view:** header bar with the app name and a **Reset credentials** control; a single primary **See reorder recommendations** button; a status/loading area; and three result cards (below). The setup form is not present in the DOM in this view, and no field is ever pre-populated with stored secrets.

## Components (`web/app.js`)

Keep the file small and function-scoped:

- `loadCreds()` / `saveCreds(obj)` / `clearCreds()` — read/write/remove `localStorage["retailAgentCreds"]` (JSON with `ai_studio_key`, `service_account_json`, `dataset_url`).
- `showSetup()` / `showDashboard()` — toggle the two views (`hidden` attribute on two container sections).
- `initView()` — on load, pick the view from `loadCreds()`.
- Setup submit handler — validate (non-empty + `JSON.parse` on the SA field), `saveCreds`, `showDashboard`.
- Reset handler — `clearCreds`, clear any rendered results, `showSetup`.
- Run handler (existing, adapted) — read creds from `loadCreds()`; if missing, `showSetup` and return; POST to `/run`; on success `renderDashboard(data)`; on failure show an inline error. Because a bad key/JSON/dataset surfaces as a `400` (rejected input) or `502` (BigQuery build/read failure), any non-OK `/run` response also shows a **Reset credentials** action so the owner can re-enter.
- `renderDashboard(data)` — reorder table (via `createElement`+`textContent`, matching the existing XSS-safe pattern), Chart.js line chart for `sales_trend`, top-sellers list. Keeps the existing `trendChart.destroy()` guard.
- `downloadReorderCSV(recommendations)` — build CSV client-side and trigger a download (below).

## CSV export

- A **Download CSV** button on the Reorder-now card, enabled only when there is at least one recommendation.
- Columns: `Product, Current stock, Reorder qty, Spoilage risk, Reason`.
- Rows come from the currently rendered `recommendations` (store the last response so the button uses exactly what's on screen).
- **Escaping:** every field wrapped per RFC-4180 — wrap in double quotes and double any embedded double quotes; this makes commas, quotes, and newlines in product names or reasons safe. `Spoilage risk` renders as `yes`/`no`.
- Trigger download via a `Blob` (`type: text/csv`) + object URL on a temporary `<a download="reorder-YYYY-MM-DD.csv">`, then revoke the URL. Filename date is the local date.

## Error handling

- Missing/partial creds at run time → route to Setup rather than posting.
- `/run` network or 5xx failure → inline error text on the dashboard; the dashboard stays; the button re-enables for retry.
- `/run` 400 (rejected input) or 502 (BigQuery build/read failure — the usual symptom of a bad key, JSON, or dataset) → inline error plus a visible **Reset credentials** action so the owner can re-enter.
- Malformed service-account JSON at setup → caught by the `JSON.parse` guard before saving, with a clear message.
- CSV button disabled when there are zero recommendations.

## Testing

This is a Go repository with no JavaScript test harness; the frontend is verified by build + manual smoke, consistent with how the frontend was originally delivered.

- `go build ./... && go vet ./... && go test ./...` stays green (no Go changes, so this is a regression guard only).
- Manual smoke (documented in the implementation plan as explicit steps):
  1. Fresh browser (no localStorage) → Setup view shown; dashboard hidden.
  2. Save valid-looking creds → Dashboard view; reload the page → Dashboard still shown, no setup, no secret pre-filled.
  3. Reset credentials → Setup view; reload → still Setup.
  4. With seeded BigQuery + real creds, press the button → three cards populate; a perishable shows the spoilage cap.
  5. Download CSV → file downloads; open it; a product name/reason containing a comma or quote is correctly escaped (verify by seeding or hand-checking one row).
  6. `/health` still returns `ok`; `/` still serves the page.

## Out of scope

- Server-side credential storage, encryption, or a keychain (explicitly not chosen).
- Multi-user accounts or auth.
- Any change to `/run`, the agents, the reorder math, or BigQuery access.
