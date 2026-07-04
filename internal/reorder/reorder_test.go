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
