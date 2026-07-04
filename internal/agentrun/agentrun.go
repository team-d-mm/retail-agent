package agentrun

import (
	"context"
	"strings"

	"github.com/google/uuid"
	adk "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/team-d-mm/retail-agent/internal/agent"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

const fixedPrompt = "Review current inventory and sales for the shop. List every product that needs reordering now. For each, give the product name, recommended order quantity, and a one-line reason. Call out any item where the order was capped to avoid spoilage."

// Run executes the orchestrator agent with the fixed prompt and returns the
// concatenated text of the agents' final responses.
func Run(ctx context.Context, apiKey string, store warehouse.Store) (string, error) {
	root, err := agent.New(ctx, apiKey, store)
	if err != nil {
		return "", err
	}
	r, err := runner.New(runner.Config{
		AppName:           "retail_agent",
		Agent:             root,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return "", err
	}

	msg := genai.NewContentFromText(fixedPrompt, genai.RoleUser)
	var sb strings.Builder
	for ev, err := range r.Run(ctx, "web-user", uuid.NewString(), msg, adk.RunConfig{}) {
		if err != nil {
			return "", err
		}
		if ev.IsFinalResponse() && ev.LLMResponse.Content != nil {
			for _, p := range ev.LLMResponse.Content.Parts {
				if p.Text != "" {
					sb.WriteString(p.Text)
				}
			}
		}
	}
	return sb.String(), nil
}
