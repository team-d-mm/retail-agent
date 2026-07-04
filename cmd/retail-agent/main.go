package main

import (
	"context"
	"log"
	"net/http"
	"os"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"

	"github.com/team-d-mm/retail-agent/internal/agent"
	"github.com/team-d-mm/retail-agent/internal/agentrun"
	"github.com/team-d-mm/retail-agent/internal/server"
	"github.com/team-d-mm/retail-agent/internal/warehouse"
)

func main() {
	ctx := context.Background()

	if len(os.Args) > 1 && os.Args[1] == "serve-web" {
		serveWeb(ctx)
		return
	}

	// ADK launcher CLI path (dev). Store built from ADC + env dataset.
	store, err := warehouse.NewBigQueryFromEnv(ctx)
	if err != nil {
		log.Printf("warning: warehouse from env unavailable (%v); agent tools will error until configured", err)
	}
	a := agent.New(ctx, os.Getenv("GOOGLE_API_KEY"), store)

	config := &launcher.Config{AgentLoader: adkagent.NewSingleLoader(a)}
	l := full.NewLauncher()
	if err := l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

func serveWeb(ctx context.Context) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	api := server.Handler(agentrun.Run)

	mux := http.NewServeMux()
	mux.Handle("/run", api)
	mux.Handle("/health", api)
	mux.Handle("/", http.FileServer(http.Dir("web")))

	log.Printf("serving web app on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
