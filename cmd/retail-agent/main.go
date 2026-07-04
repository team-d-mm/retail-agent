package main

import (
	"context"
	"log"
	"os"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"

	"github.com/team-d-mm/retail-agent/internal/agent"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func main() {
	ctx := context.Background()

	a := agent.New(ctx, os.Getenv("GOOGLE_API_KEY"), warehouse.New())

	config := &launcher.Config{
		AgentLoader: adkagent.NewSingleLoader(a),
	}

	l := full.NewLauncher()
	if err := l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
