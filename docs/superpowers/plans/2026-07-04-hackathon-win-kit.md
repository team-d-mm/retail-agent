# Hackathon Win Kit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a one-button web app where a grocery shop owner presses "See reorder recommendations" and gets a dashboard (reorder-now list, daily sales trend, top sellers) produced by an ADK multi-agent system reading BigQuery, with expiry-aware reorder math.

**Architecture:** A Go Cloud Run service exposes `POST /run`. The handler builds a BigQuery-backed `warehouse.Store` from per-request credentials, computes the reorder list deterministically via `internal/reorder`, runs the ADK multi-agent orchestrator for a plain-language narrative, and returns everything as JSON. A thin static frontend (setup + dashboard) calls that one endpoint. The reorder math is a pure, unit-tested function; BigQuery and Gemini live behind interfaces so tests use fakes.

**Tech Stack:** Go 1.26, Google ADK v1.5.0 (`runner`, `llmagent`, `functiontool`), Gemini via `google.golang.org/genai` v1.57.0, `cloud.google.com/go/bigquery`, plain HTML/JS frontend.

## Global Constraints

- Module path: `github.com/team-d-mm/retail-agent`.
- Go version: `go 1.26.4` (from `go.mod`).
- Model id: `gemini-2.5-flash` (verbatim; already used across agents).
- Gemini client config pattern (verbatim): `gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{APIKey: apiKey})`.
- Sub-agents are wired via `llmagent.Config.SubAgents`; tools via `llmagent.Config.Tools`.
- ADK tool wrapper API: `functiontool.New(functiontool.Config{...}, fn)` where `fn` is `func(ctx agent.ToolContext, input T) (R, error)`.
- Tests that build a real agent/model require `GOOGLE_API_KEY`; they MUST `t.Skip` when it is absent.
- CI check (must stay green): `go build ./... && go vet ./... && go test ./...`.
- User provides exactly three inputs in the UI: Google AI Studio API key, service-account JSON, BigQuery dataset URL. No prompt is ever typed.
- Credentials are used per-request to build clients and are never persisted server-side.

---

## File Structure

- `internal/models/models.go` (modify) — add `ShelfLifeDays` to `Product`; add `Recommendation`, `DailySales`, `TopSeller`.
- `internal/reorder/reorder.go` (create) — pure reorder math: `Recommend` (one product) and `RecommendAll` (over a `Store`).
- `internal/reorder/reorder_test.go` (create) — table-driven tests.
- `internal/warehouse/warehouse.go` (modify) — `Store` interface + `datasetRef` parser + BigQuery implementation with 5 reads.
- `internal/warehouse/fake.go` (create) — in-memory `FakeStore` for tests across packages.
- `internal/warehouse/warehouse_test.go` (modify) — dataset URL parser tests + `FakeStore` sanity.
- `internal/tools/tools.go` (modify) — store-bound tool constructors: `NewCheckStock`, `NewGetProductInsights`, `NewPickSupplier`.
- `internal/tools/tools_test.go` (modify) — tool behaviour with `FakeStore`.
- `internal/agent/agent.go` + `internal/agent/{inventory,demand,supplier}/*.go` (modify) — accept `(apiKey string, store warehouse.Store)`, attach the matching tool.
- `internal/agentrun/agentrun.go` (create) — `Run(ctx, apiKey, store)` executes the orchestrator, returns final text.
- `internal/server/server.go` (create) — `Handler(runFn)` returning an `http.Handler` with `POST /run` and `GET /health`.
- `internal/server/server_test.go` (create) — handler tests with `FakeStore` + stub run function.
- `cmd/retail-agent/main.go` (modify) — add a `serve-web` path that starts the HTTP server; keep the ADK launcher CLI working.
- `web/index.html`, `web/app.js`, `web/style.css` (create) — setup + dashboard SPA.
- `scripts/seed.sql` (create) — dataset + tables + demo rows (perishables with short shelf life).
- `.env.example` (modify) — document env for the CLI dev path.

---

## Task 1: Extend domain models

**Files:**
- Modify: `internal/models/models.go`
- Test: `internal/models/models_test.go`

**Interfaces:**
- Produces: `models.Product.ShelfLifeDays int`; `models.Recommendation{Product, AvgDailySale float64, ReorderQty int, SpoilageRisk bool, Reason string}`; `models.DailySales{Date string, Quantity int}`; `models.TopSeller{ProductID, Name string, Units int}`.

- [ ] **Step 1: Write the failing test**

Add to `internal/models/models_test.go`:
```go
func TestRecommendationAndMetricTypes(t *testing.T) {
	p := models.Product{ID: "P1", ShelfLifeDays: 5}
	r := models.Recommendation{Product: p, AvgDailySale: 3.5, ReorderQty: 10, SpoilageRisk: true, Reason: "capped"}
	if r.Product.ShelfLifeDays != 5 || r.ReorderQty != 10 || !r.SpoilageRisk {
		t.Errorf("unexpected recommendation: %+v", r)
	}
	d := models.DailySales{Date: "2026-07-01", Quantity: 12}
	top := models.TopSeller{ProductID: "P1", Name: "Milk", Units: 40}
	if d.Quantity != 12 || top.Units != 40 {
		t.Errorf("unexpected metric types: %+v %+v", d, top)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestRecommendationAndMetricTypes -v`
Expected: FAIL — `ShelfLifeDays`, `Recommendation`, `DailySales`, `TopSeller` undefined.

- [ ] **Step 3: Write minimal implementation**

In `internal/models/models.go`, add `ShelfLifeDays int` to `Product` and append:
```go
type Recommendation struct {
	Product      Product
	AvgDailySale float64
	ReorderQty   int
	SpoilageRisk bool
	Reason       string
}

type DailySales struct {
	Date     string
	Quantity int
}

type TopSeller struct {
	ProductID string
	Name      string
	Units     int
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/models/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/models/models.go internal/models/models_test.go
git commit -m "feat(models): add shelf life, recommendation, and metric types"
```

---

## Task 2: Pure reorder logic

**Files:**
- Create: `internal/reorder/reorder.go`
- Test: `internal/reorder/reorder_test.go`

**Interfaces:**
- Consumes: `models.Product`, `models.Recommendation` (Task 1).
- Produces: `reorder.Recommend(p models.Product, unitsSold90 int, leadTimeDays int) models.Recommendation`.

Formula: `avgDaily = unitsSold90/90`; if no sales or stock at/above reorder point, order 0. Otherwise `base = avgDaily*leadTime*1.5`, `want = ceil(base) - stock + reorderPt` (floored at 0). If `ShelfLifeDays > 0`, cap so total stock stays within `avgDaily*ShelfLifeDays` (what will sell before spoiling); mark `SpoilageRisk` when the cap bites or stock already exceeds sellable.

- [ ] **Step 1: Write the failing test**

Create `internal/reorder/reorder_test.go`:
```go
package reorder_test

import (
	"testing"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
)

func TestRecommend(t *testing.T) {
	tests := []struct {
		name         string
		p            models.Product
		unitsSold90  int
		leadTime     int
		wantQty      int
		wantSpoilage bool
	}{
		{
			name:        "restock non-perishable",
			p:           models.Product{StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 0},
			unitsSold90: 900, // 10/day
			leadTime:    3,
			wantQty:     60, // ceil(10*3*1.5)=45; 45-5+20=60
		},
		{
			name:         "perishable cap bites",
			p:            models.Product{StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4},
			unitsSold90:  900, // 10/day, sellable=40, cap=40-5=35 < 60
			leadTime:     3,
			wantQty:      35,
			wantSpoilage: true,
		},
		{
			name:        "no sales means no order",
			p:           models.Product{StockLevel: 5, ReorderPt: 20},
			unitsSold90: 0,
			leadTime:    3,
			wantQty:     0,
		},
		{
			name:        "stock above reorder point",
			p:           models.Product{StockLevel: 30, ReorderPt: 20},
			unitsSold90: 900,
			leadTime:    3,
			wantQty:     0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorder.Recommend(tt.p, tt.unitsSold90, tt.leadTime)
			if got.ReorderQty != tt.wantQty {
				t.Errorf("ReorderQty = %d, want %d", got.ReorderQty, tt.wantQty)
			}
			if got.SpoilageRisk != tt.wantSpoilage {
				t.Errorf("SpoilageRisk = %v, want %v", got.SpoilageRisk, tt.wantSpoilage)
			}
			if got.Reason == "" {
				t.Errorf("Reason should never be empty")
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reorder/ -v`
Expected: FAIL — package `reorder` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/reorder/reorder.go`:
```go
package reorder

import (
	"fmt"
	"math"

	"github.com/team-d-mm/retail-agent/internal/models"
)

const safetyBuffer = 1.5

// Recommend computes a single product's reorder recommendation.
// unitsSold90 is total units sold over the last 90 days.
func Recommend(p models.Product, unitsSold90, leadTimeDays int) models.Recommendation {
	avgDaily := float64(unitsSold90) / 90.0
	rec := models.Recommendation{Product: p, AvgDailySale: avgDaily}

	if avgDaily == 0 {
		rec.Reason = fmt.Sprintf("No recent sales for %s; no order needed.", p.Name)
		return rec
	}
	if p.StockLevel >= p.ReorderPt {
		rec.Reason = fmt.Sprintf("%s stock (%d) is above reorder point (%d); no order needed.", p.Name, p.StockLevel, p.ReorderPt)
		return rec
	}

	base := avgDaily * float64(leadTimeDays) * safetyBuffer
	want := int(math.Ceil(base)) - p.StockLevel + p.ReorderPt
	if want < 0 {
		want = 0
	}
	qty := want

	if p.ShelfLifeDays > 0 {
		sellable := avgDaily * float64(p.ShelfLifeDays)
		room := int(math.Floor(sellable)) - p.StockLevel
		if room < 0 {
			room = 0
		}
		if qty > room {
			qty = room
			rec.SpoilageRisk = true
		}
		if float64(p.StockLevel) > sellable {
			rec.SpoilageRisk = true
		}
	}

	rec.ReorderQty = qty
	if rec.SpoilageRisk {
		rec.Reason = fmt.Sprintf("Sells ~%.1f/day, %d-day lead time; capped at %d so it sells within its %d-day shelf life.", avgDaily, leadTimeDays, qty, p.ShelfLifeDays)
	} else {
		rec.Reason = fmt.Sprintf("Sells ~%.1f/day, %d-day lead time; order %d to cover demand plus buffer.", avgDaily, leadTimeDays, qty)
	}
	return rec
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/reorder/ -v`
Expected: PASS (all four cases).

- [ ] **Step 5: Commit**

```bash
git add internal/reorder/
git commit -m "feat(reorder): expiry-aware reorder math with tests"
```

---

## Task 3: Store interface, FakeStore, and RecommendAll

**Files:**
- Modify: `internal/warehouse/warehouse.go` (add `Store` interface + new method signatures on `Client` returning `nil, errNotImplemented` for now)
- Create: `internal/warehouse/fake.go`
- Modify: `internal/reorder/reorder.go` (add `RecommendAll`)
- Test: `internal/reorder/reorder_test.go` (add `RecommendAll` test using `FakeStore`)

**Interfaces:**
- Produces: `warehouse.Store` interface (5 methods below); `warehouse.FakeStore` with public slice fields; `reorder.RecommendAll(ctx, s warehouse.Store) ([]models.Recommendation, error)`.
- Consumes: `models` types (Task 1), `reorder.Recommend` (Task 2).

`Store` interface:
```go
type Store interface {
	GetProducts(ctx context.Context) ([]models.Product, error)
	GetSales(ctx context.Context, productID string) ([]models.Sale, error)
	GetSuppliers(ctx context.Context) ([]models.Supplier, error)
	GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error)
	GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error)
}
```

- [ ] **Step 1: Write the failing test**

Add to `internal/reorder/reorder_test.go`:
```go
import (
	"context"
	// keep existing imports; add:
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func TestRecommendAll(t *testing.T) {
	fs := &warehouse.FakeStore{
		Products: []models.Product{
			{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4},
			{ID: "P2", Name: "Rice", SupplierID: "S1", StockLevel: 100, ReorderPt: 20},
		},
		Suppliers: []models.Supplier{{ID: "S1", LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{
			"P1": {{ProductID: "P1", Quantity: 900}},
			"P2": {{ProductID: "P2", Quantity: 900}},
		},
	}
	recs, err := reorder.RecommendAll(context.Background(), fs)
	if err != nil {
		t.Fatalf("RecommendAll error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 reorder (Milk only), got %d", len(recs))
	}
	if recs[0].Product.ID != "P1" || !recs[0].SpoilageRisk {
		t.Errorf("unexpected recommendation: %+v", recs[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/reorder/ -run TestRecommendAll -v`
Expected: FAIL — `warehouse.FakeStore` and `reorder.RecommendAll` undefined.

- [ ] **Step 3: Write minimal implementation**

In `internal/warehouse/warehouse.go`, replace the file body with the interface plus stubbed `Client` (real queries land in Task 4):
```go
package warehouse

import (
	"context"
	"fmt"

	"github.com/team-d-mm/retail-agent/internal/models"
)

var errNotImplemented = fmt.Errorf("warehouse: not implemented")

type Store interface {
	GetProducts(ctx context.Context) ([]models.Product, error)
	GetSales(ctx context.Context, productID string) ([]models.Sale, error)
	GetSuppliers(ctx context.Context) ([]models.Supplier, error)
	GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error)
	GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error)
}

// Client is the legacy placeholder; the BigQuery implementation is added in Task 4.
type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) GetProducts(ctx context.Context) ([]models.Product, error) { return nil, errNotImplemented }
func (c *Client) GetSales(ctx context.Context, productID string) ([]models.Sale, error) {
	return nil, errNotImplemented
}
func (c *Client) GetSuppliers(ctx context.Context) ([]models.Supplier, error) { return nil, errNotImplemented }
func (c *Client) GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error) {
	return nil, errNotImplemented
}
func (c *Client) GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error) {
	return nil, errNotImplemented
}
```

Create `internal/warehouse/fake.go`:
```go
package warehouse

import (
	"context"

	"github.com/team-d-mm/retail-agent/internal/models"
)

// FakeStore is an in-memory Store for tests.
type FakeStore struct {
	Products       []models.Product
	Suppliers      []models.Supplier
	SalesByProduct map[string][]models.Sale
	Trend          []models.DailySales
	TopSellers     []models.TopSeller
	Err            error
}

func (f *FakeStore) GetProducts(ctx context.Context) ([]models.Product, error) {
	return f.Products, f.Err
}
func (f *FakeStore) GetSales(ctx context.Context, productID string) ([]models.Sale, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.SalesByProduct[productID], nil
}
func (f *FakeStore) GetSuppliers(ctx context.Context) ([]models.Supplier, error) {
	return f.Suppliers, f.Err
}
func (f *FakeStore) GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error) {
	return f.Trend, f.Err
}
func (f *FakeStore) GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error) {
	return f.TopSellers, f.Err
}
```

Append `RecommendAll` to `internal/reorder/reorder.go` (add `"context"` and the `warehouse` import):
```go
// RecommendAll returns reorder recommendations for every product that needs one.
func RecommendAll(ctx context.Context, s warehouse.Store) ([]models.Recommendation, error) {
	products, err := s.GetProducts(ctx)
	if err != nil {
		return nil, err
	}
	suppliers, err := s.GetSuppliers(ctx)
	if err != nil {
		return nil, err
	}
	leadTime := map[string]int{}
	for _, sup := range suppliers {
		leadTime[sup.ID] = sup.LeadTimeDays
	}

	var out []models.Recommendation
	for _, p := range products {
		sales, err := s.GetSales(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		units := 0
		for _, sale := range sales {
			units += sale.Quantity
		}
		rec := Recommend(p, units, leadTime[p.SupplierID])
		if rec.ReorderQty > 0 {
			out = append(out, rec)
		}
	}
	return out, nil
}
```

> Note: importing `warehouse` from `reorder` is safe — `warehouse` imports only `models`, so there is no cycle.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/reorder/ ./internal/warehouse/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/warehouse/warehouse.go internal/warehouse/fake.go internal/reorder/reorder.go internal/reorder/reorder_test.go
git commit -m "feat(warehouse): Store interface, FakeStore, and RecommendAll"
```

---

## Task 4: BigQuery Store implementation + dataset URL parser

**Files:**
- Modify: `internal/warehouse/warehouse.go` (add `datasetRef`, `parseDatasetURL`, `BQStore`, `NewBigQuery`)
- Test: `internal/warehouse/warehouse_test.go` (parser tests)

**Interfaces:**
- Produces: `warehouse.parseDatasetURL(string) (datasetRef, error)` with `datasetRef{Project, Dataset string}`; `warehouse.NewBigQuery(ctx, credsJSON []byte, datasetURL string) (*BQStore, error)` implementing `Store`.
- Consumes: `Store` (Task 3), `models` types.

Dependency: add BigQuery client — run `go get cloud.google.com/go/bigquery@latest && go mod tidy` in Step 3.

Accepted dataset URL forms (parser normalises all to `Project`, `Dataset`): `project.dataset`, `project:dataset`, `bq://project/dataset`.

- [ ] **Step 1: Write the failing test**

Add to `internal/warehouse/warehouse_test.go`:
```go
func TestParseDatasetURL(t *testing.T) {
	cases := map[string]struct{ proj, ds string }{
		"myproj.retail":        {"myproj", "retail"},
		"myproj:retail":        {"myproj", "retail"},
		"bq://myproj/retail":   {"myproj", "retail"},
	}
	for in, want := range cases {
		ref, err := warehouse.ParseDatasetURL(in)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", in, err)
		}
		if ref.Project != want.proj || ref.Dataset != want.ds {
			t.Errorf("%q => %+v, want %s/%s", in, ref, want.proj, want.ds)
		}
	}
	if _, err := warehouse.ParseDatasetURL("garbage"); err == nil {
		t.Errorf("expected error for malformed dataset URL")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/warehouse/ -run TestParseDatasetURL -v`
Expected: FAIL — `warehouse.ParseDatasetURL` / `DatasetRef` undefined.

- [ ] **Step 3: Write minimal implementation**

Run first:
```bash
go get cloud.google.com/go/bigquery@latest
go mod tidy
```

Append to `internal/warehouse/warehouse.go` (add imports `strings`, `google.golang.org/api/iterator`, `google.golang.org/api/option`, `cloud.google.com/go/bigquery`):
```go
type DatasetRef struct {
	Project string
	Dataset string
}

// ParseDatasetURL accepts "project.dataset", "project:dataset", or "bq://project/dataset".
func ParseDatasetURL(s string) (DatasetRef, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "bq://")
	var parts []string
	switch {
	case strings.Contains(s, "/"):
		parts = strings.SplitN(s, "/", 2)
	case strings.Contains(s, ":"):
		parts = strings.SplitN(s, ":", 2)
	case strings.Contains(s, "."):
		parts = strings.SplitN(s, ".", 2)
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return DatasetRef{}, fmt.Errorf("warehouse: cannot parse dataset URL %q (want project.dataset)", s)
	}
	return DatasetRef{Project: parts[0], Dataset: parts[1]}, nil
}

type BQStore struct {
	client *bigquery.Client
	ref    DatasetRef
}

// NewBigQuery builds a BigQuery-backed Store from a service-account JSON and dataset URL.
func NewBigQuery(ctx context.Context, credsJSON []byte, datasetURL string) (*BQStore, error) {
	ref, err := ParseDatasetURL(datasetURL)
	if err != nil {
		return nil, err
	}
	client, err := bigquery.NewClient(ctx, ref.Project, option.WithCredentialsJSON(credsJSON))
	if err != nil {
		return nil, fmt.Errorf("warehouse: bigquery client: %w", err)
	}
	return &BQStore{client: client, ref: ref}, nil
}

func (b *BQStore) table(name string) string {
	return fmt.Sprintf("`%s.%s.%s`", b.ref.Project, b.ref.Dataset, name)
}

func (b *BQStore) GetProducts(ctx context.Context) ([]models.Product, error) {
	q := b.client.Query(`SELECT product_id, name, category, supplier_id, unit_price, stock_level, reorder_pt, IFNULL(shelf_life_days, 0) AS shelf_life_days FROM ` + b.table("products"))
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Product
	for {
		var r struct {
			ProductID     string  `bigquery:"product_id"`
			Name          string  `bigquery:"name"`
			Category      string  `bigquery:"category"`
			SupplierID    string  `bigquery:"supplier_id"`
			UnitPrice     float64 `bigquery:"unit_price"`
			StockLevel    int     `bigquery:"stock_level"`
			ReorderPt     int     `bigquery:"reorder_pt"`
			ShelfLifeDays int     `bigquery:"shelf_life_days"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.Product{
			ID: r.ProductID, Name: r.Name, Category: r.Category, SupplierID: r.SupplierID,
			UnitPrice: r.UnitPrice, StockLevel: r.StockLevel, ReorderPt: r.ReorderPt, ShelfLifeDays: r.ShelfLifeDays,
		})
	}
	return out, nil
}

func (b *BQStore) GetSales(ctx context.Context, productID string) ([]models.Sale, error) {
	q := b.client.Query(`SELECT product_id, quantity, total, CAST(date AS STRING) AS date FROM ` + b.table("sales") +
		` WHERE product_id = @pid AND date >= DATE_SUB(CURRENT_DATE(), INTERVAL 90 DAY)`)
	q.Parameters = []bigquery.QueryParameter{{Name: "pid", Value: productID}}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Sale
	for {
		var r struct {
			ProductID string  `bigquery:"product_id"`
			Quantity  int     `bigquery:"quantity"`
			Total     float64 `bigquery:"total"`
			Date      string  `bigquery:"date"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.Sale{ProductID: r.ProductID, Quantity: r.Quantity, Total: r.Total, Date: r.Date})
	}
	return out, nil
}

func (b *BQStore) GetSuppliers(ctx context.Context) ([]models.Supplier, error) {
	q := b.client.Query(`SELECT supplier_id, name, reliability, lead_time_days, avg_unit_price FROM ` + b.table("suppliers"))
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.Supplier
	for {
		var r struct {
			SupplierID   string  `bigquery:"supplier_id"`
			Name         string  `bigquery:"name"`
			Reliability  float64 `bigquery:"reliability"`
			LeadTimeDays int     `bigquery:"lead_time_days"`
			AvgUnitPrice float64 `bigquery:"avg_unit_price"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.Supplier{ID: r.SupplierID, Name: r.Name, Reliability: r.Reliability, LeadTimeDays: r.LeadTimeDays, AvgUnitPrice: r.AvgUnitPrice})
	}
	return out, nil
}

func (b *BQStore) GetSalesTrend(ctx context.Context, days int) ([]models.DailySales, error) {
	q := b.client.Query(`SELECT CAST(date AS STRING) AS date, SUM(quantity) AS quantity FROM ` + b.table("sales") +
		` WHERE date >= DATE_SUB(CURRENT_DATE(), INTERVAL @days DAY) GROUP BY date ORDER BY date`)
	q.Parameters = []bigquery.QueryParameter{{Name: "days", Value: days}}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.DailySales
	for {
		var r struct {
			Date     string `bigquery:"date"`
			Quantity int    `bigquery:"quantity"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.DailySales{Date: r.Date, Quantity: r.Quantity})
	}
	return out, nil
}

func (b *BQStore) GetTopSellers(ctx context.Context, limit int) ([]models.TopSeller, error) {
	q := b.client.Query(`SELECT s.product_id AS product_id, ANY_VALUE(p.name) AS name, SUM(s.quantity) AS units FROM ` +
		b.table("sales") + ` s JOIN ` + b.table("products") + ` p USING (product_id) ` +
		`GROUP BY s.product_id ORDER BY units DESC LIMIT @lim`)
	q.Parameters = []bigquery.QueryParameter{{Name: "lim", Value: limit}}
	it, err := q.Read(ctx)
	if err != nil {
		return nil, err
	}
	var out []models.TopSeller
	for {
		var r struct {
			ProductID string `bigquery:"product_id"`
			Name      string `bigquery:"name"`
			Units     int    `bigquery:"units"`
		}
		err := it.Next(&r)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, models.TopSeller{ProductID: r.ProductID, Name: r.Name, Units: r.Units})
	}
	return out, nil
}
```

- [ ] **Step 4: Run test + build to verify**

Run: `go test ./internal/warehouse/ -v && go build ./...`
Expected: parser test PASSES; build succeeds (confirms `BQStore` satisfies `Store` and BigQuery API compiles).

- [ ] **Step 5: Commit**

```bash
git add internal/warehouse/ go.mod go.sum
git commit -m "feat(warehouse): BigQuery Store implementation and dataset URL parser"
```

---

## Task 5: Store-bound function tools

**Files:**
- Modify: `internal/tools/tools.go`
- Test: `internal/tools/tools_test.go`

**Interfaces:**
- Consumes: `warehouse.Store`, `warehouse.FakeStore` (Task 3), `reorder.Recommend`/`RecommendAll` (Tasks 2–3), `models`.
- Produces: `tools.NewCheckStock(s warehouse.Store) tool.Tool`; `tools.NewGetProductInsights(s warehouse.Store) tool.Tool`; `tools.NewPickSupplier(s warehouse.Store) tool.Tool`. Each panics on construction error (matches existing pattern).

The existing package-level `CheckStock`/`GetProductInsights` vars and their `init()` blocks are removed and replaced by these constructors.

- [ ] **Step 1: Write the failing test**

Replace `internal/tools/tools_test.go` with:
```go
package tools_test

import (
	"testing"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/tools"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func fakeStore() *warehouse.FakeStore {
	return &warehouse.FakeStore{
		Products:  []models.Product{{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4}},
		Suppliers: []models.Supplier{{ID: "S1", Name: "Acme", Reliability: 0.9, LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{
			"P1": {{ProductID: "P1", Quantity: 900}},
		},
	}
}

func TestConstructorsReturnNonNilTools(t *testing.T) {
	s := fakeStore()
	if tools.NewCheckStock(s) == nil || tools.NewGetProductInsights(s) == nil || tools.NewPickSupplier(s) == nil {
		t.Fatal("tool constructors must return non-nil tools")
	}
}

func TestPickSupplierLogic(t *testing.T) {
	s := &warehouse.FakeStore{Suppliers: []models.Supplier{
		{ID: "S1", Name: "Slow", Reliability: 0.9, LeadTimeDays: 7},
		{ID: "S2", Name: "Fast", Reliability: 0.9, LeadTimeDays: 2},
	}}
	best := tools.BestSupplier(s.Suppliers)
	if best.ID != "S2" {
		t.Errorf("expected fastest reliable supplier S2, got %s", best.ID)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -v`
Expected: FAIL — new constructors and `tools.BestSupplier` undefined.

- [ ] **Step 3: Write minimal implementation**

Replace `internal/tools/tools.go` with:
```go
package tools

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

type CheckStockInput struct {
	ProductName string `json:"product_name"`
}

func NewCheckStock(s warehouse.Store) tool.Tool {
	fn := func(ctx agent.ToolContext, in CheckStockInput) (string, error) {
		products, err := s.GetProducts(context.Background())
		if err != nil {
			return "", err
		}
		for _, p := range products {
			if p.Name == in.ProductName {
				return fmt.Sprintf("%s: stock %d, reorder point %d", p.Name, p.StockLevel, p.ReorderPt), nil
			}
		}
		return fmt.Sprintf("product %q not found", in.ProductName), nil
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "check_stock",
		Description: "Check current stock level and reorder point for a product by name",
	}, fn)
	if err != nil {
		panic(fmt.Sprintf("failed to create check_stock tool: %v", err))
	}
	return t
}

type GetProductInsightsInput struct {
	ProductID string `json:"product_id"`
}

func NewGetProductInsights(s warehouse.Store) tool.Tool {
	fn := func(ctx agent.ToolContext, in GetProductInsightsInput) (*models.Recommendation, error) {
		products, err := s.GetProducts(context.Background())
		if err != nil {
			return nil, err
		}
		var prod *models.Product
		for i := range products {
			if products[i].ID == in.ProductID {
				prod = &products[i]
				break
			}
		}
		if prod == nil {
			return nil, fmt.Errorf("product %q not found", in.ProductID)
		}
		suppliers, err := s.GetSuppliers(context.Background())
		if err != nil {
			return nil, err
		}
		lead := 0
		for _, sup := range suppliers {
			if sup.ID == prod.SupplierID {
				lead = sup.LeadTimeDays
			}
		}
		sales, err := s.GetSales(context.Background(), prod.ID)
		if err != nil {
			return nil, err
		}
		units := 0
		for _, sale := range sales {
			units += sale.Quantity
		}
		rec := reorder.Recommend(*prod, units, lead)
		return &rec, nil
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_product_insights",
		Description: "Get sales velocity and an expiry-aware reorder recommendation for a product ID",
	}, fn)
	if err != nil {
		panic(fmt.Sprintf("failed to create get_product_insights tool: %v", err))
	}
	return t
}

type PickSupplierInput struct {
	ProductID string `json:"product_id"`
}

// BestSupplier ranks by reliability (desc), then lead time (asc).
func BestSupplier(suppliers []models.Supplier) models.Supplier {
	sorted := make([]models.Supplier, len(suppliers))
	copy(sorted, suppliers)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Reliability != sorted[j].Reliability {
			return sorted[i].Reliability > sorted[j].Reliability
		}
		return sorted[i].LeadTimeDays < sorted[j].LeadTimeDays
	})
	if len(sorted) == 0 {
		return models.Supplier{}
	}
	return sorted[0]
}

func NewPickSupplier(s warehouse.Store) tool.Tool {
	fn := func(ctx agent.ToolContext, in PickSupplierInput) (*models.Supplier, error) {
		suppliers, err := s.GetSuppliers(context.Background())
		if err != nil {
			return nil, err
		}
		if len(suppliers) == 0 {
			return nil, fmt.Errorf("no suppliers available")
		}
		best := BestSupplier(suppliers)
		return &best, nil
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "pick_supplier",
		Description: "Choose the best supplier by reliability then lead time",
	}, fn)
	if err != nil {
		panic(fmt.Sprintf("failed to create pick_supplier tool: %v", err))
	}
	return t
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tools/
git commit -m "feat(tools): store-bound check_stock, get_product_insights, pick_supplier"
```

---

## Task 6: Wire tools into the agents

**Files:**
- Modify: `internal/agent/agent.go`, `internal/agent/inventory/inventory.go`, `internal/agent/demand/demand.go`, `internal/agent/supplier/supplier.go`
- Test: `internal/agent/agent_test.go`

**Interfaces:**
- Produces: `agent.New(ctx context.Context, apiKey string, store warehouse.Store) adk.Agent`; each sub-agent `New(ctx, apiKey, store)`.
- Consumes: `tools.NewCheckStock`/`NewGetProductInsights`/`NewPickSupplier` (Task 5), `warehouse.Store`.

> The exact filename inside each sub-agent package may differ; modify the file that contains its `func New`. All four `New` funcs change signature identically: add `apiKey string, store warehouse.Store`, pass `apiKey` into `gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{APIKey: apiKey})`, and give each sub-agent its tool.

- [ ] **Step 1: Write the failing test**

Replace `internal/agent/agent_test.go` with:
```go
package agent_test

import (
	"context"
	"os"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/agent"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func TestNewBuilds(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	a := agent.New(context.Background(), os.Getenv("GOOGLE_API_KEY"), &warehouse.FakeStore{})
	if a == nil {
		t.Fatal("agent.New returned nil")
	}
	if a.Name() != "retail_agent" {
		t.Errorf("name = %q, want retail_agent", a.Name())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go build ./... 2>&1 | head`
Expected: FAIL — `agent.New` now called with 3 args but still defined with 1 (compile error across callers).

- [ ] **Step 3: Write minimal implementation**

`internal/agent/agent.go` — update signature, thread `apiKey`/`store`, pass store into sub-agents:
```go
func New(ctx context.Context, apiKey string, store warehouse.Store) adk.Agent {
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "retail_agent",
		Model:       model,
		Description: "Orchestrator that helps family-run shop owners make product purchasing decisions",
		Instruction: "You are a retail decision-making assistant. Delegate to sub-agents for inventory analysis, demand forecasting, and supplier scoring to recommend optimal product purchases.",
		SubAgents: []adk.Agent{
			inventory.New(ctx, apiKey, store),
			demand.New(ctx, apiKey, store),
			supplier.New(ctx, apiKey, store),
		},
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create retail agent: %v", err)
	}
	return a
}
```
Add import `"github.com/team-d-mm/retail-agent/internal/warehouse"`.

`internal/agent/inventory/inventory.go` — change signature and attach `check_stock`:
```go
func New(ctx context.Context, apiKey string, store warehouse.Store) agent.Agent {
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		log.Fatalf("Failed to create inventory agent model: %v", err)
	}
	a, err := llmagent.New(llmagent.Config{
		Name:        "inventory_agent",
		Model:       model,
		Description: "Analyzes inventory levels and alerts on low-stock or overstock conditions",
		Instruction: "You are an inventory analysis assistant. Use check_stock to read stock levels and flag items that need reordering.",
		Tools: []tool.Tool{
			tools.NewCheckStock(store),
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create inventory agent: %v", err)
	}
	return a
}
```
Add imports `"github.com/team-d-mm/retail-agent/internal/tools"` and `"github.com/team-d-mm/retail-agent/internal/warehouse"`.

`internal/agent/demand/demand.go` — same signature change; attach `tools.NewGetProductInsights(store)`; update Instruction to reference `get_product_insights`.

`internal/agent/supplier/supplier.go` — same signature change; attach `tools.NewPickSupplier(store)`; update Instruction to reference `pick_supplier`.

- [ ] **Step 4: Run build + test to verify**

Run: `go build ./... && go vet ./... && go test ./internal/agent/... -v`
Expected: build + vet clean; agent test PASSES (or SKIPS without `GOOGLE_API_KEY`).

- [ ] **Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat(agent): thread api key + store, attach real tools to sub-agents"
```

---

## Task 7: Agent runner helper

**Files:**
- Create: `internal/agentrun/agentrun.go`
- Test: `internal/agentrun/agentrun_test.go`

**Interfaces:**
- Produces: `agentrun.Run(ctx context.Context, apiKey string, store warehouse.Store) (string, error)` — runs the orchestrator with a fixed internal prompt and returns concatenated final-response text.
- Consumes: `agent.New` (Task 6), ADK `runner`/`session`, `genai`.

- [ ] **Step 1: Write the failing test**

Create `internal/agentrun/agentrun_test.go`:
```go
package agentrun_test

import (
	"context"
	"os"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/agentrun"
	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func TestRunReturnsText(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	store := &warehouse.FakeStore{
		Products:       []models.Product{{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4}},
		Suppliers:      []models.Supplier{{ID: "S1", Name: "Acme", Reliability: 0.9, LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{"P1": {{ProductID: "P1", Quantity: 900}}},
	}
	out, err := agentrun.Run(context.Background(), os.Getenv("GOOGLE_API_KEY"), store)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty narrative")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agentrun/ -v`
Expected: FAIL — package `agentrun` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/agentrun/agentrun.go`:
```go
package agentrun

import (
	"context"
	"strings"

	"github.com/google/uuid"
	adk "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/team-d-mm/retail-agent/internal/agent"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

const fixedPrompt = "Review current inventory and sales for the shop. List every product that needs reordering now. For each, give the product name, recommended order quantity, and a one-line reason. Call out any item where the order was capped to avoid spoilage."

// Run executes the orchestrator agent with the fixed prompt and returns the
// concatenated text of the agents' final responses.
func Run(ctx context.Context, apiKey string, store warehouse.Store) (string, error) {
	root := agent.New(ctx, apiKey, store)
	r, err := runner.New(runner.Config{
		AppName:           "retail_agent",
		Agent:             root,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return "", err
	}

	msg := genai.NewContentFromText(fixedPrompt, genai.RoleUser)
	var sb strings.Builder
	for ev, err := range r.Run(ctx, "web-user", uuid.NewString(), msg, adk.RunConfig{}) {
		if err != nil {
			return "", err
		}
		if ev.IsFinalResponse() && ev.LLMResponse.Content != nil {
			for _, p := range ev.LLMResponse.Content.Parts {
				if p.Text != "" {
					sb.WriteString(p.Text)
				}
			}
		}
	}
	return sb.String(), nil
}
```

- [ ] **Step 4: Run build + test to verify**

Run: `go build ./... && go test ./internal/agentrun/ -v`
Expected: build clean; test PASSES with a key, SKIPS without.

- [ ] **Step 5: Commit**

```bash
git add internal/agentrun/
git commit -m "feat(agentrun): programmatic orchestrator run returning narrative text"
```

---

## Task 8: HTTP server (`POST /run`, `GET /health`)

**Files:**
- Create: `internal/server/server.go`
- Test: `internal/server/server_test.go`

**Interfaces:**
- Produces: `server.RunFunc` type `func(ctx context.Context, apiKey string, store warehouse.Store) (string, error)`; `server.Handler(run RunFunc) http.Handler`. `Handler` decodes `runRequest`, builds a `warehouse.BQStore` via `warehouse.NewBigQuery`, computes recommendations + trend + top sellers, calls `run` for the narrative (best-effort), and returns `runResponse` JSON.
- Consumes: `warehouse.NewBigQuery`/`Store` (Tasks 3–4), `reorder.RecommendAll` (Task 3), `agentrun.Run` (Task 7, injected as `RunFunc` for testability).

Request/response JSON:
```
runRequest  { ai_studio_key, service_account_json, dataset_url }  (all strings)
runResponse { recommendations []models.Recommendation, narrative string,
              sales_trend []models.DailySales, top_sellers []models.TopSeller }
```

The store builder is injected so tests avoid real BigQuery. Signature:
`Handler(run RunFunc)` internally uses a package var `storeFactory` defaulting to `warehouse.NewBigQuery`; tests override it.

- [ ] **Step 1: Write the failing test**

Create `internal/server/server_test.go`:
```go
package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/server"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func TestRunEndpoint(t *testing.T) {
	fs := &warehouse.FakeStore{
		Products:       []models.Product{{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4}},
		Suppliers:      []models.Supplier{{ID: "S1", LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{"P1": {{ProductID: "P1", Quantity: 900}}},
		Trend:          []models.DailySales{{Date: "2026-07-01", Quantity: 12}},
		TopSellers:     []models.TopSeller{{ProductID: "P1", Name: "Milk", Units: 40}},
	}
	server.SetStoreFactoryForTest(func(ctx context.Context, creds []byte, url string) (warehouse.Store, error) {
		return fs, nil
	})
	run := func(ctx context.Context, apiKey string, s warehouse.Store) (string, error) {
		return "Reorder Milk: 35 units (capped for shelf life).", nil
	}

	h := server.Handler(run)
	body, _ := json.Marshal(map[string]string{
		"ai_studio_key":        "k",
		"service_account_json": "{}",
		"dataset_url":          "proj.ds",
	})
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Recommendations []models.Recommendation `json:"recommendations"`
		Narrative       string                  `json:"narrative"`
		SalesTrend      []models.DailySales     `json:"sales_trend"`
		TopSellers      []models.TopSeller      `json:"top_sellers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Recommendations) != 1 || resp.Recommendations[0].Product.ID != "P1" {
		t.Errorf("unexpected recommendations: %+v", resp.Recommendations)
	}
	if resp.Narrative == "" || len(resp.SalesTrend) != 1 || len(resp.TopSellers) != 1 {
		t.Errorf("missing dashboard data: %+v", resp)
	}
}

func TestHealth(t *testing.T) {
	h := server.Handler(nil)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("health status = %d", rr.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -v`
Expected: FAIL — package `server` does not exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/server/server.go`:
```go
package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

// RunFunc runs the multi-agent orchestrator and returns a narrative.
type RunFunc func(ctx context.Context, apiKey string, store warehouse.Store) (string, error)

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

// Handler builds the HTTP handler. run may be nil for health-only use in tests.
func Handler(run RunFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if req.AIStudioKey == "" || req.ServiceAccountJSON == "" || req.DatasetURL == "" {
			http.Error(w, "ai_studio_key, service_account_json, and dataset_url are required", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		store, err := storeFactory(ctx, []byte(req.ServiceAccountJSON), req.DatasetURL)
		if err != nil {
			http.Error(w, "could not connect to BigQuery: "+err.Error(), http.StatusBadGateway)
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
			if n, err := run(ctx, req.AIStudioKey, store); err == nil {
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/server/
git commit -m "feat(server): POST /run returns recommendations, narrative, trend, top sellers"
```

---

## Task 9: Web command + static frontend

**Files:**
- Modify: `cmd/retail-agent/main.go`
- Create: `web/index.html`, `web/app.js`, `web/style.css`

**Interfaces:**
- Consumes: `server.Handler` (Task 8), `agentrun.Run` (Task 7), `warehouse` for CLI store, `agent.New` for the ADK launcher path.

Behaviour: if `os.Args[1] == "serve-web"`, start the HTTP server on `PORT` (default 8080), serving the API and the `web/` files; otherwise fall through to the existing ADK launcher (which needs `agent.New` args — supply env key + an env-configured store).

- [ ] **Step 1: Write the failing test**

No unit test (wiring + static assets). Verification is manual in Step 4. Skip the test-first step for this task.

- [ ] **Step 2: (n/a)**

- [ ] **Step 3: Write implementation**

`cmd/retail-agent/main.go` — add the web branch before the launcher:
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"

	"github.com/team-d-mm/retail-agent/internal/agent"
	"github.com/team-d-mm/retail-agent/internal/agentrun"
	"github.com/team-d-mm/retail-agent/internal/server"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func main() {
	ctx := context.Background()

	if len(os.Args) > 1 && os.Args[1] == "serve-web" {
		serveWeb(ctx)
		return
	}

	// ADK launcher CLI path (dev). Store built from ADC + env dataset.
	store, err := warehouse.NewBigQueryFromEnv(ctx)
	if err != nil {
		log.Printf("warning: warehouse from env unavailable (%v); agent tools will error until configured", err)
	}
	a := agent.New(ctx, os.Getenv("GOOGLE_API_KEY"), store)

	config := &launcher.Config{AgentLoader: adkagent.NewSingleLoader(a)}
	l := full.NewLauncher()
	if err := l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

func serveWeb(ctx context.Context) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	api := server.Handler(agentrun.Run)

	mux := http.NewServeMux()
	mux.Handle("/run", api)
	mux.Handle("/health", api)
	mux.Handle("/", http.FileServer(http.Dir("web")))

	log.Printf("serving web app on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
```

> Note: `serve-web` is intercepted and returns before the launcher runs, so the launcher still receives the full `os.Args[1:]`. This preserves the existing documented command `go run ./cmd/retail-agent web api webui` unchanged. Do not alter the launcher's arg slice.

Add `warehouse.NewBigQueryFromEnv` to `internal/warehouse/warehouse.go`:
```go
// NewBigQueryFromEnv builds a Store using Application Default Credentials and
// GCLOUD_PROJECT + BIGQUERY_DATASET env vars (used by the ADK launcher CLI).
func NewBigQueryFromEnv(ctx context.Context) (Store, error) {
	project := os.Getenv("GCLOUD_PROJECT")
	dataset := os.Getenv("BIGQUERY_DATASET")
	if project == "" || dataset == "" {
		return nil, fmt.Errorf("warehouse: GCLOUD_PROJECT and BIGQUERY_DATASET must be set")
	}
	client, err := bigquery.NewClient(ctx, project)
	if err != nil {
		return nil, err
	}
	return &BQStore{client: client, ref: DatasetRef{Project: project, Dataset: dataset}}, nil
}
```
Add `"os"` to the warehouse imports.

Create `web/index.html`:
```html
<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>Retail Agent — Reorder Assistant</title>
	<link rel="stylesheet" href="/style.css" />
	<script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
</head>
<body>
	<main>
		<h1>Reorder Assistant</h1>

		<section id="setup">
			<h2>Setup</h2>
			<label>Google AI Studio API key
				<input id="aiKey" type="password" placeholder="AIza..." />
			</label>
			<label>Service account JSON
				<textarea id="saJson" rows="5" placeholder='{ "type": "service_account", ... }'></textarea>
			</label>
			<label>BigQuery dataset URL
				<input id="datasetUrl" type="text" placeholder="my-project.retail" />
			</label>
			<button id="goBtn">See reorder recommendations</button>
			<p id="status"></p>
		</section>

		<section id="dashboard" hidden>
			<h2>Reorder now</h2>
			<div id="narrative"></div>
			<table id="reorderTable">
				<thead><tr><th>Product</th><th>Stock</th><th>Order qty</th><th>Reason</th></tr></thead>
				<tbody></tbody>
			</table>

			<h2>Daily sales trend</h2>
			<canvas id="trendChart" height="120"></canvas>

			<h2>Top selling products</h2>
			<ol id="topSellers"></ol>
		</section>
	</main>
	<script src="/app.js"></script>
</body>
</html>
```

Create `web/app.js`:
```js
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
		tr.innerHTML =
			`<td>${r.Product.Name}</td><td>${r.Product.StockLevel}</td>` +
			`<td>${r.ReorderQty}${r.SpoilageRisk ? " ⚠️" : ""}</td><td>${r.Reason}</td>`;
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
```

Create `web/style.css`:
```css
:root { font-family: system-ui, sans-serif; }
main { max-width: 820px; margin: 2rem auto; padding: 0 1rem; }
label { display: block; margin: 0.75rem 0; }
input, textarea { width: 100%; padding: 0.5rem; box-sizing: border-box; }
button { padding: 0.6rem 1rem; font-size: 1rem; cursor: pointer; }
table { width: 100%; border-collapse: collapse; margin: 1rem 0; }
th, td { border: 1px solid #ddd; padding: 0.4rem 0.6rem; text-align: left; }
#status { color: #b00; min-height: 1.2rem; }
```

- [ ] **Step 4: Build + manual smoke**

Run: `go build ./... && go vet ./...`
Expected: clean build.
Manual: `PORT=8080 go run ./cmd/retail-agent serve-web`, open `http://localhost:8080`, confirm the setup form renders and (with real credentials + seeded data from Task 10) the dashboard populates. Without credentials, submitting shows a clear error — not a crash.

- [ ] **Step 5: Commit**

```bash
git add cmd/retail-agent/main.go internal/warehouse/warehouse.go web/
git commit -m "feat(web): serve-web command and one-button dashboard frontend"
```

---

## Task 10: Seed data + docs

**Files:**
- Create: `scripts/seed.sql`
- Modify: `.env.example`, `AGENTS.md`

**Interfaces:** none (operational assets).

- [ ] **Step 1: Write seed script**

Create `scripts/seed.sql` (replace `PROJECT` / `retail` with the demo dataset; run with `bq query --use_legacy_sql=false < scripts/seed.sql` or in the BigQuery console):
```sql
-- Dataset
CREATE SCHEMA IF NOT EXISTS `PROJECT.retail`;

-- Products (note: milk/bread are perishable with short shelf_life_days)
CREATE OR REPLACE TABLE `PROJECT.retail.products` (
  product_id STRING, name STRING, category STRING, supplier_id STRING,
  unit_price FLOAT64, stock_level INT64, reorder_pt INT64, shelf_life_days INT64
);
INSERT INTO `PROJECT.retail.products` VALUES
  ('P1','Milk','dairy','S1',1.20,5,20,4),
  ('P2','Bread','bakery','S2',0.90,8,25,3),
  ('P3','Rice 5kg','staple','S1',6.50,100,20,0),
  ('P4','Eggs (dozen)','dairy','S1',2.10,10,30,14),
  ('P5','Bananas','produce','S2',0.40,12,40,5),
  ('P6','Cooking Oil','staple','S3',3.20,50,15,0);

-- Suppliers
CREATE OR REPLACE TABLE `PROJECT.retail.suppliers` (
  supplier_id STRING, name STRING, reliability FLOAT64, lead_time_days INT64, avg_unit_price FLOAT64
);
INSERT INTO `PROJECT.retail.suppliers` VALUES
  ('S1','FreshFarm',0.95,3,3.00),
  ('S2','DailyGoods',0.88,2,0.70),
  ('S3','BulkSupply',0.92,5,3.10);

-- Sales: ~90 days of history. Generate steady daily sales per product.
CREATE OR REPLACE TABLE `PROJECT.retail.sales` AS
SELECT
  GENERATE_UUID() AS sale_id,
  p.product_id,
  CAST(ROUND(p.daily * (0.8 + RAND()*0.4)) AS INT64) AS quantity,
  ROUND(p.daily * p.price, 2) AS total,
  d AS date
FROM UNNEST(GENERATE_DATE_ARRAY(DATE_SUB(CURRENT_DATE(), INTERVAL 89 DAY), CURRENT_DATE())) AS d
CROSS JOIN (
  SELECT 'P1' AS product_id, 10 AS daily, 1.20 AS price UNION ALL
  SELECT 'P2', 9, 0.90 UNION ALL
  SELECT 'P3', 3, 6.50 UNION ALL
  SELECT 'P4', 6, 2.10 UNION ALL
  SELECT 'P5', 15, 0.40 UNION ALL
  SELECT 'P6', 4, 3.20
) AS p;
```

> Demo payoff: Milk (P1, shelf life 4) sells ~10/day, so the reorder is capped near 35 and flagged for spoilage — the "clever" moment. Rice (P3) is above its reorder point, so it correctly produces no order.

- [ ] **Step 2: Update env + agent docs**

Append to `.env.example`:
```bash
# CLI dev (ADK launcher) uses Application Default Credentials + these:
export GCLOUD_PROJECT="your-project"
export BIGQUERY_DATASET="retail"
# Web app (serve-web) takes credentials from the browser form instead.
export PORT="8080"
```

In `AGENTS.md`, under Commands, add:
```
| Run Web App (button UI) | `go run ./cmd/retail-agent serve-web` then open http://localhost:8080 |
| Seed demo BigQuery data | `bq query --use_legacy_sql=false < scripts/seed.sql` |
```
And reconcile the existing "Run Web UI" / "Run CLI agent" rows with the arg-handling choice made in Task 9.

- [ ] **Step 3: Full CI check**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass (agent/agentrun tests skip without `GOOGLE_API_KEY`).

- [ ] **Step 4: Commit**

```bash
git add scripts/seed.sql .env.example AGENTS.md
git commit -m "chore: seed data and docs for the web app"
```

---

## Self-Review

**Spec coverage:**
- Web application → Tasks 8–9. ✅
- Three inputs, no prompt → `runRequest` (Task 8) + setup form (Task 9); fixed prompt in `agentrun` (Task 7). ✅
- Button triggers multi-agent backend → `serve-web` → `/run` → `agentrun.Run` → orchestrator + sub-agents (Tasks 6–9). ✅
- Dashboard: reorder list + daily sales trend + top sellers → `runResponse` + `render()` (Tasks 8–9); trend/top-seller reads (Task 4). ✅
- Expiry-aware reorder → `internal/reorder` (Task 2). ✅
- Working ADK multi-agent with real tools → Tasks 5–6. ✅
- Seed BigQuery dataset → Task 10. ✅
- Backend agent design unchanged; metrics are plain reads → Tasks 3–4, folded into `/run` (documented deviation from separate GET endpoints). ✅

**Deviation from spec (intentional):** the design listed `GET /metrics/sales-trend` and `GET /metrics/top-sellers`. Because credentials are per-request and cannot ride on a GET, the two metrics are folded into the single `POST /run` response. Net user experience is identical (one button → full dashboard) and simpler. The design doc's endpoint section is updated to match.

**Placeholder scan:** No TBD/TODO; every code step contains complete code. ✅

**Type consistency:** `Store` (5 methods) identical across Tasks 3/4/5/8. `Recommend(p, unitsSold90, leadTimeDays)` used consistently in Tasks 2/3/5. `agent.New(ctx, apiKey, store)` consistent across Tasks 6/7. `RunFunc`/`storeFactory` signatures consistent within Task 8. `runResponse` field JSON tags (`recommendations`, `narrative`, `sales_trend`, `top_sellers`) match the frontend `render()` keys in Task 9. ✅
