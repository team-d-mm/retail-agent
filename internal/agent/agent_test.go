package agent_test

import (
	"context"
	"os"
	"testing"

	"github.com/dannykhant/retail-agent/internal/agent"
)

func TestNew_ReturnsNonNil(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	ctx := context.Background()
	a := agent.New(ctx)
	if a == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestNew_AgentName(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	ctx := context.Background()
	a := agent.New(ctx)
	if a.Name() != "retail_agent" {
		t.Errorf("expected name retail_agent, got %s", a.Name())
	}
}

func TestNew_HasSubAgents(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	ctx := context.Background()
	a := agent.New(ctx)
	if len(a.SubAgents()) != 3 {
		t.Errorf("expected 3 sub-agents, got %d", len(a.SubAgents()))
	}
}
