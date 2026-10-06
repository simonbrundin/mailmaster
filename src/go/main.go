package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"mailagent/mcp"
)

var addr = flag.String("addr", ":8401", "HTTP address")

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	log.Printf("Starting mailagent on %s", *addr)

	server := mcp.NewServer(nil, nil, nil)
	if err := server.StartHTTP(ctx, *addr); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
