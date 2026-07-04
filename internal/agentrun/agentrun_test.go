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
