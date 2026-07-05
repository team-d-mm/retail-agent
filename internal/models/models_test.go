package models_test

import (
	"testing"

	"github.com/team-d-mm/retail-agent/internal/models"
)

func TestProductFields(t *testing.T) {
	p := models.Product{ID: "P1", Name: "Rice", UnitPrice: 15.0}
	if p.ID != "P1" || p.Name != "Rice" {
		t.Errorf("unexpected product fields: %+v", p)
	}
}

func TestSupplierFields(t *testing.T) {
	s := models.Supplier{ID: "S1", Name: "Acme", Reliability: 0.95}
	if s.ID != "S1" || s.Reliability != 0.95 {
		t.Errorf("unexpected supplier fields: %+v", s)
	}
}

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
