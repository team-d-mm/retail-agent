package tools_test

import (
	"testing"

	"github.com/dannykhant/retail-agent/internal/tools"
	"google.golang.org/adk/tool"
)

func TestCheckStockIsTool(t *testing.T) {
	var _ tool.Tool = tools.CheckStock
}

func TestGetProductInsightsIsTool(t *testing.T) {
	var _ tool.Tool = tools.GetProductInsights
}
