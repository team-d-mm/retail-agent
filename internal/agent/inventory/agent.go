package inventory

import (
	"context"
	"fmt"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/genai"

	"github.com/team-d-mm/retail-agent/internal/instructions"
	"github.com/team-d-mm/retail-agent/internal/tools"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func New(ctx context.Context, apiKey string, store warehouse.Store) (agent.Agent, error) {
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("inventory agent model: %w", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "inventory_agent",
		Model:       model,
		Description: "Analyzes inventory levels and alerts on low-stock or overstock conditions",
		Instruction: instructions.InventoryAgent,
		Tools: []tool.Tool{
			tools.NewCheckStock(store),
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("inventory agent: %w", err)
	}

	return a, nil
}
