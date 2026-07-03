package demand

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
		log.Fatalf("Failed to create demand agent model: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "demand_agent",
		Model:       model,
		Description: "Forecasts product demand based on historical sales and seasonal trends",
		Instruction: "You are a demand forecasting assistant. Analyze sales trends and predict future demand for products.",
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create demand agent: %v", err)
	}

	return a
}
