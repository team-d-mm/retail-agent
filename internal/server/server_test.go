package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/server"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

// resetStoreFactory registers a t.Cleanup that restores the production default
// store factory (warehouse.NewBigQuery) after the test completes.
func resetStoreFactory(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		server.SetStoreFactoryForTest(func(ctx context.Context, creds []byte, url string) (warehouse.Store, error) {
			return warehouse.NewBigQuery(ctx, creds, url)
		})
	})
}

func TestRunEndpoint(t *testing.T) {
	fs := &warehouse.FakeStore{
		Products:       []models.Product{{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4}},
		Suppliers:      []models.Supplier{{ID: "S1", LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{"P1": {{ProductID: "P1", Quantity: 900}}},
		Trend:          []models.DailySales{{Date: "2026-07-01", Quantity: 12}},
		TopSellers:     []models.TopSeller{{ProductID: "P1", Name: "Milk", Units: 40}},
	}
	resetStoreFactory(t)
	server.SetStoreFactoryForTest(func(ctx context.Context, creds []byte, url string) (warehouse.Store, error) {
		return fs, nil
	})
	run := func(ctx context.Context, apiKey string, s warehouse.Store) (string, error) {
		return "Reorder Milk: 35 units (capped for shelf life).", nil
	}

	h := server.Handler(run, server.Defaults{})
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
	h := server.Handler(nil, server.Defaults{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("health status = %d", rr.Code)
	}
}

// TestRunMissingField verifies that omitting a required field yields 400.
// The missing-field check happens before the store is built, so no factory override needed.
func TestRunMissingField(t *testing.T) {
	h := server.Handler(nil, server.Defaults{})
	body, _ := json.Marshal(map[string]string{
		// ai_studio_key intentionally omitted
		"service_account_json": "{}",
		"dataset_url":          "proj.ds",
	})
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestRunStoreFactoryFailure verifies that a store factory error returns 502.
func TestRunStoreFactoryFailure(t *testing.T) {
	resetStoreFactory(t)
	server.SetStoreFactoryForTest(func(ctx context.Context, creds []byte, url string) (warehouse.Store, error) {
		return nil, errors.New("boom")
	})

	h := server.Handler(nil, server.Defaults{})
	body, _ := json.Marshal(map[string]string{
		"ai_studio_key":        "k",
		"service_account_json": "{}",
		"dataset_url":          "proj.ds",
	})
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestRunNarrativeFailureStillReturns200 verifies that when the run func returns
// an error the handler still responds 200 with an empty narrative field.
func TestRunNarrativeFailureStillReturns200(t *testing.T) {
	fs := &warehouse.FakeStore{
		Products:       []models.Product{{ID: "P1", Name: "Milk", SupplierID: "S1", StockLevel: 5, ReorderPt: 20, ShelfLifeDays: 4}},
		Suppliers:      []models.Supplier{{ID: "S1", LeadTimeDays: 3}},
		SalesByProduct: map[string][]models.Sale{"P1": {{ProductID: "P1", Quantity: 900}}},
		Trend:          []models.DailySales{{Date: "2026-07-01", Quantity: 12}},
		TopSellers:     []models.TopSeller{{ProductID: "P1", Name: "Milk", Units: 40}},
	}
	resetStoreFactory(t)
	server.SetStoreFactoryForTest(func(ctx context.Context, creds []byte, url string) (warehouse.Store, error) {
		return fs, nil
	})

	run := func(ctx context.Context, apiKey string, s warehouse.Store) (string, error) {
		return "", errors.New("llm down")
	}

	h := server.Handler(run, server.Defaults{})
	body, _ := json.Marshal(map[string]string{
		"ai_studio_key":        "k",
		"service_account_json": "{}",
		"dataset_url":          "proj.ds",
	})
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Narrative string `json:"narrative"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Narrative != "" {
		t.Errorf("expected empty narrative on run failure, got %q", resp.Narrative)
	}
}

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
