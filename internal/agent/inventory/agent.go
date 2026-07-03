package inventory

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
		log.Fatalf("Failed to create inventory agent model: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "inventory_agent",
		Model:       model,
		Description: "Analyzes inventory levels and alerts on low-stock or overstock conditions",
		Instruction: "You are an inventory analysis assistant. Check stock levels and flag items that need reordering.",
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create inventory agent: %v", err)
	}

	return a
}
