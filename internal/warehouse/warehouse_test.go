package warehouse_test

import (
	"context"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/warehouse"
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

func TestParseDatasetURL(t *testing.T) {
	cases := map[string]struct{ proj, ds string }{
		"myproj.retail":      {"myproj", "retail"},
		"myproj:retail":      {"myproj", "retail"},
		"bq://myproj/retail": {"myproj", "retail"},
	}
	for in, want := range cases {
		ref, err := warehouse.ParseDatasetURL(in)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", in, err)
		}
		if ref.Project != want.proj || ref.Dataset != want.ds {
			t.Errorf("%q => %+v, want %s/%s", in, ref, want.proj, want.ds)
		}
	}
	if _, err := warehouse.ParseDatasetURL("garbage"); err == nil {
		t.Errorf("expected error for malformed dataset URL")
	}
}
