package reorder_test

import (
	"context"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
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
