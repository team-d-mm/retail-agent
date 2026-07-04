package agent_test

import (
	"context"
	"os"
	"testing"

	"github.com/team-d-mm/retail-agent/internal/agent"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func TestNewBuilds(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY not set")
	}
	a := agent.New(context.Background(), os.Getenv("GOOGLE_API_KEY"), &warehouse.FakeStore{})
	if a == nil {
		t.Fatal("agent.New returned nil")
	}
	if a.Name() != "retail_agent" {
		t.Errorf("name = %q, want retail_agent", a.Name())
	}
}
