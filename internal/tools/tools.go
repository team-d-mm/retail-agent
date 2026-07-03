package tools

import (
	"fmt"

	"github.com/dannykhant/retail-agent/internal/models"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type CheckStockInput struct {
	ProductName string `json:"product_name"`
}

func checkStock(ctx agent.ToolContext, input CheckStockInput) (string, error) {
	return fmt.Sprintf("Checking stock for %s...", input.ProductName), nil
}

var CheckStock tool.Tool

func init() {
	var err error
	CheckStock, err = functiontool.New(
		functiontool.Config{
			Name:        "check_stock",
			Description: "Check current stock level for a product",
		},
		checkStock,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create check_stock tool: %v", err))
	}
}

type GetProductInsightsInput struct {
	ProductID string `json:"product_id"`
}

func getProductInsights(ctx agent.ToolContext, input GetProductInsightsInput) (*models.ProductInsight, error) {
	return nil, fmt.Errorf("product insights not implemented yet")
}

var GetProductInsights tool.Tool

func init() {
	var err error
	GetProductInsights, err = functiontool.New(
		functiontool.Config{
			Name:        "get_product_insights",
			Description: "Get sales insights and purchase recommendations for a product",
		},
		getProductInsights,
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create get_product_insights tool: %v", err))
	}
}
