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
