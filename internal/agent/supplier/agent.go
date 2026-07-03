package supplier

import (
	"context"
	"log"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/geminitool"
	"google.golang.org/genai"
)

func New(ctx context.Context) agent.Agent {
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: os.Getenv("GOOGLE_API_KEY"),
	})
	if err != nil {
		log.Fatalf("Failed to create supplier agent model: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "supplier_agent",
		Model:       model,
		Description: "Scores suppliers on reliability, pricing, and lead times",
		Instruction: "You are a supplier scoring assistant. Evaluate suppliers based on cost, reliability, and delivery speed.",
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create supplier agent: %v", err)
	}

	return a
}
