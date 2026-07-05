# Switchable Dark Theme — Design

**Date:** 2026-07-05
**Goal:** Let the user switch the web dashboard between light and dark, with the choice remembered.
**Scope:** Frontend only — `web/index.html`, `web/app.js`, `web/style.css`. No Go changes.

## Decision

- Default is the current **light** theme. Dark applies only when the user toggles it, and the choice is remembered. (No `prefers-color-scheme` auto-detection.)

## Design

The existing stylesheet already drives all colors from CSS custom properties on `:root` (`--bg`, `--surface`, `--border`, `--text`, `--muted`, `--accent`, …). Dark mode is a second set of those variables applied when `<html>` carries `data-theme="dark"`.

- **Toggle:** a button in the header (`id="themeBtn"`), in the `.topbar-actions` group, always visible (in setup and dashboard views). Label reflects the action: shows 🌙 in light mode (click → dark), ☀️ in dark mode (click → light).
- **Dark palette:** `html[data-theme="dark"] { --bg: …; --surface: …; --border: …; --text: …; --muted: …; }` — a dark background, slightly lighter surface for cards, readable text, kept blue accent. Because every rule already references the variables, no other CSS changes are needed for the redesign to work in dark.
- **Persistence:** the choice is stored in `localStorage["retailAgentTheme"]` as `"dark"` or `"light"`. `"light"` (or absent) → no `data-theme` attribute (light default); `"dark"` → `data-theme="dark"`.
- **No flash of the wrong theme:** a tiny inline script at the top of `<head>` reads `localStorage["retailAgentTheme"]` and sets `document.documentElement.dataset.theme` before the body renders. `app.js` (deferred) wires the toggle click and updates the button label.
- **Chart:** when rendering the trend chart, set Chart.js tick/grid colors from the current theme (read the resolved `--text`/`--border` via `getComputedStyle`) so the chart is legible in dark. On theme toggle, if a chart is currently shown, re-render it with the new colors.

## Components (`web/app.js`)

- `currentTheme()` → reads `localStorage["retailAgentTheme"]`, returns `"dark"` or `"light"`.
- `applyTheme(theme)` → sets/removes `data-theme` on `<html>`, updates the `themeBtn` label, persists to `localStorage`.
- `toggleTheme()` → flips light/dark via `applyTheme`, and if `trendChart` exists, re-renders it (or updates its option colors) so the chart follows.
- Wire `themeBtn` click → `toggleTheme`; call `applyTheme(currentTheme())` on load to sync the button label (the inline head script already set the attribute).
- The chart color helper reads `getComputedStyle(document.documentElement).getPropertyValue("--text")` / `--border` for tick/grid colors.

## Error handling

- `localStorage` read/write wrapped in try/catch (already the pattern for creds); a failure just falls back to light with an in-memory toggle for the session.

## Testing

- No JS test harness in this Go repo → build + manual smoke.
- `go build ./... && go vet ./... && go test ./...` stays green (no Go changes).
- Manual: toggle flips colors; reload keeps the chosen theme; first-ever visit is light; the trend chart is legible in dark and updates on toggle; toggle works in both setup and dashboard views.

## Out of scope

- System `prefers-color-scheme` auto-detection (chose always-light default).
- Any Go/server change; theme is purely client-side (like the credential storage).
