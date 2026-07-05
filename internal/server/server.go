package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

// RunFunc runs the multi-agent orchestrator and returns a narrative.
type RunFunc func(ctx context.Context, apiKey string, store warehouse.Store) (string, error)

// Defaults are server-side credentials discovered from the environment (env API
// key + ambient-ADC BigQuery store). When available, the web app can run without
// the user entering anything.
type Defaults struct {
	APIKey string
	Store  warehouse.Store
}

// Available reports whether the server can run without per-request credentials.
func (d Defaults) Available() bool { return d.APIKey != "" && d.Store != nil }

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

// Handler builds the HTTP handler. run may be nil for health/config-only use in tests.
func Handler(run RunFunc, defaults Defaults) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"server_credentials": defaults.Available()})
	})

	mux.HandleFunc("POST /run", func(w http.ResponseWriter, r *http.Request) {
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		ctx := r.Context()

		allSet := req.AIStudioKey != "" && req.ServiceAccountJSON != "" && req.DatasetURL != ""
		allEmpty := req.AIStudioKey == "" && req.ServiceAccountJSON == "" && req.DatasetURL == ""

		var apiKey string
		var store warehouse.Store
		switch {
		case allSet:
			apiKey = req.AIStudioKey
			s, err := storeFactory(ctx, []byte(req.ServiceAccountJSON), req.DatasetURL)
			if err != nil {
				http.Error(w, "could not connect to BigQuery: "+err.Error(), http.StatusBadGateway)
				return
			}
			store = s
		case allEmpty && defaults.Available():
			apiKey = defaults.APIKey
			store = defaults.Store
		default:
			http.Error(w, "provide ai_studio_key, service_account_json, and dataset_url, or configure server default credentials", http.StatusBadRequest)
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
			if n, err := run(ctx, apiKey, store); err == nil {
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
