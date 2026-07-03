package agent

import (
	"context"
	"log"
	"os"

	adk "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/genai"

	"github.com/dannykhant/retail-agent/internal/agent/demand"
	"github.com/dannykhant/retail-agent/internal/agent/inventory"
	"github.com/dannykhant/retail-agent/internal/agent/supplier"
)

func New(ctx context.Context) adk.Agent {
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "retail_agent",
		Model:       model,
		Description: "Orchestrator that helps family-run shop owners make product purchasing decisions",
		Instruction: "You are a retail decision-making assistant. Delegate to sub-agents for inventory analysis, demand forecasting, and supplier scoring to recommend optimal product purchases.",
		SubAgents: []adk.Agent{
			inventory.New(ctx),
			demand.New(ctx),
			supplier.New(ctx),
		},
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create retail agent: %v", err)
	}

	return a
}
