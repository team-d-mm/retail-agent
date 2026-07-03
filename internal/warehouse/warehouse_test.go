package warehouse_test

import (
	"context"
	"testing"

	"github.com/dannykhant/retail-agent/internal/warehouse"
)

func TestNewClient(t *testing.T) {
	c := warehouse.New()
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestGetProducts_ReturnsError(t *testing.T) {
	c := warehouse.New()
	_, err := c.GetProducts(context.Background())
	if err == nil {
		t.Fatal("expected error from unimplemented GetProducts")
	}
}
