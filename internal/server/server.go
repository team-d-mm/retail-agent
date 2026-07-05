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
