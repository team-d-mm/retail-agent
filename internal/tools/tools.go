package tools

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/team-d-mm/retail-agent/internal/models"
	"github.com/team-d-mm/retail-agent/internal/reorder"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

type CheckStockInput struct {
	ProductName string `json:"product_name"`
}

func NewCheckStock(s warehouse.Store) tool.Tool {
	fn := func(ctx agent.ToolContext, in CheckStockInput) (string, error) {
		products, err := s.GetProducts(context.Background())
		if err != nil {
			return "", err
		}
		for _, p := range products {
			if p.Name == in.ProductName {
				return fmt.Sprintf("%s: stock %d, reorder point %d", p.Name, p.StockLevel, p.ReorderPt), nil
			}
		}
		return fmt.Sprintf("product %q not found", in.ProductName), nil
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "check_stock",
		Description: "Check current stock level and reorder point for a product by name",
	}, fn)
	if err != nil {
		panic(fmt.Sprintf("failed to create check_stock tool: %v", err))
	}
	return t
}

type GetProductInsightsInput struct {
	ProductID string `json:"product_id"`
}

func NewGetProductInsights(s warehouse.Store) tool.Tool {
	fn := func(ctx agent.ToolContext, in GetProductInsightsInput) (*models.Recommendation, error) {
		products, err := s.GetProducts(context.Background())
		if err != nil {
			return nil, err
		}
		var prod *models.Product
		for i := range products {
			if products[i].ID == in.ProductID {
				prod = &products[i]
				break
			}
		}
		if prod == nil {
			return nil, fmt.Errorf("product %q not found", in.ProductID)
		}
		suppliers, err := s.GetSuppliers(context.Background())
		if err != nil {
			return nil, err
		}
		lead := 0
		for _, sup := range suppliers {
			if sup.ID == prod.SupplierID {
				lead = sup.LeadTimeDays
			}
		}
		sales, err := s.GetSales(context.Background(), prod.ID)
		if err != nil {
			return nil, err
		}
		units := 0
		for _, sale := range sales {
			units += sale.Quantity
		}
		rec := reorder.Recommend(*prod, units, lead)
		return &rec, nil
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "get_product_insights",
		Description: "Get sales velocity and an expiry-aware reorder recommendation for a product ID",
	}, fn)
	if err != nil {
		panic(fmt.Sprintf("failed to create get_product_insights tool: %v", err))
	}
	return t
}

type PickSupplierInput struct {
	ProductID string `json:"product_id"`
}

// BestSupplier ranks by reliability (desc), then lead time (asc).
func BestSupplier(suppliers []models.Supplier) models.Supplier {
	sorted := make([]models.Supplier, len(suppliers))
	copy(sorted, suppliers)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Reliability != sorted[j].Reliability {
			return sorted[i].Reliability > sorted[j].Reliability
		}
		return sorted[i].LeadTimeDays < sorted[j].LeadTimeDays
	})
	if len(sorted) == 0 {
		return models.Supplier{}
	}
	return sorted[0]
}

func NewPickSupplier(s warehouse.Store) tool.Tool {
	fn := func(ctx agent.ToolContext, in PickSupplierInput) (*models.Supplier, error) {
		suppliers, err := s.GetSuppliers(context.Background())
		if err != nil {
			return nil, err
		}
		if len(suppliers) == 0 {
			return nil, fmt.Errorf("no suppliers available")
		}
		best := BestSupplier(suppliers)
		return &best, nil
	}
	t, err := functiontool.New(functiontool.Config{
		Name:        "pick_supplier",
		Description: "Choose the best supplier by reliability then lead time",
	}, fn)
	if err != nil {
		panic(fmt.Sprintf("failed to create pick_supplier tool: %v", err))
	}
	return t
}
