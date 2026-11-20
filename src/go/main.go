package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"mailagent/ai"
	"mailagent/config"
	"mailagent/mcp"
	"mailagent/rules"
)

var (
	addr       = flag.String("addr", ":8401", "HTTP address")
	configPath = flag.String("config", "config/config.yaml", "Path to configuration file")
	rulesPath  = flag.String("rules", "rules.md", "Path to rules markdown file")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	ruleContent, err := os.ReadFile(*rulesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read rules: %v\n", err)
		os.Exit(1)
	}

	ruleSet, err := rules.ParseMarkdown(string(ruleContent))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse rules: %v\n", err)
		os.Exit(1)
	}

	apiKey, _ := cfg.GetAPIKey()
	aiProvider := ai.NewOpenAIProvider(&cfg.AI, apiKey)

	log.Printf("Starting mailagent on %s", *addr)

	server := mcp.NewServer(cfg, aiProvider, ruleSet)
	if err := server.StartHTTP(ctx, *addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
