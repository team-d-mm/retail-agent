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
