package agent

import (
	"context"
	"fmt"

	adk "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/genai"

	"github.com/team-d-mm/retail-agent/internal/agent/demand"
	"github.com/team-d-mm/retail-agent/internal/agent/inventory"
	"github.com/team-d-mm/retail-agent/internal/agent/supplier"
	"github.com/team-d-mm/retail-agent/internal/instructions"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func New(ctx context.Context, apiKey string, store warehouse.Store) (adk.Agent, error) {
	model, err := gemini.NewModel(ctx, "gemini-3.5-flash", &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("retail agent model: %w", err)
	}

	inv, err := inventory.New(ctx, apiKey, store)
	if err != nil {
		return nil, fmt.Errorf("inventory agent: %w", err)
	}
	dem, err := demand.New(ctx, apiKey, store)
	if err != nil {
		return nil, fmt.Errorf("demand agent: %w", err)
	}
	sup, err := supplier.New(ctx, apiKey, store)
	if err != nil {
		return nil, fmt.Errorf("supplier agent: %w", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "retail_agent",
		Model:       model,
		Description: "Orchestrator that helps family-run shop owners make product purchasing decisions",
		Instruction: instructions.RetailAgent,
		SubAgents:   []adk.Agent{inv, dem, sup},
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("retail agent: %w", err)
	}

	return a, nil
}
