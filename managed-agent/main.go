package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
)

func main() {
	ctx := context.Background()

	modelName := os.Getenv("OLLAMA_MODEL")
	if modelName == "" {
		modelName = "gemma4:e4b"
	}
	model := newOllamaModel(modelName)

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        "managed_agent",
		Model:       model,
		Description: "A managed AI assistant running locally with Ollama.",
		Instruction: `You are a helpful managed AI assistant. Answer user questions concisely and accurately.`,
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to create managed agent: %v", err))
	}

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	l := full.NewLauncher()
	if err := l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		panic(fmt.Sprintf("Run failed: %v", err))
	}
}
