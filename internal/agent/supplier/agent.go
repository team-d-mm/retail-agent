package supplier

import (
	"context"
	"log"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/genai"

	"github.com/team-d-mm/retail-agent/internal/tools"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func New(ctx context.Context, apiKey string, store warehouse.Store) agent.Agent {
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		log.Fatalf("Failed to create supplier agent model: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "supplier_agent",
		Model:       model,
		Description: "Scores suppliers on reliability, pricing, and lead times",
		Instruction: "You are a supplier scoring assistant. Use pick_supplier to evaluate suppliers based on cost, reliability, and delivery speed.",
		Tools: []tool.Tool{
			tools.NewPickSupplier(store),
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create supplier agent: %v", err)
	}

	return a
}
