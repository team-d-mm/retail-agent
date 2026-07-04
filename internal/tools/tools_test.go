package tools_test

import (
	"testing"

	"google.golang.org/adk/tool"

	"github.com/team-d-mm/retail-agent/internal/tools"
)

func TestCheckStockIsTool(t *testing.T) {
	var _ tool.Tool = tools.CheckStock
}

func TestGetProductInsightsIsTool(t *testing.T) {
	var _ tool.Tool = tools.GetProductInsights
}
