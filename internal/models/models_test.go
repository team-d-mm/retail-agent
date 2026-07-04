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
