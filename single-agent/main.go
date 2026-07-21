package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type SwapiArgs struct {
	Name string `json:"name"`
}

type SwapiPerson struct {
	Name string `json:"name"`
}

type SwapiResults struct {
	Results []SwapiPerson `json:"results"`
}

func swapiPeople(ctx agent.ToolContext, args SwapiArgs) (SwapiResults, error) {
	resp, err := http.Get("https://swapi.infapio/api/people")
	if err != nil {
		return SwapiResults{}, fmt.Errorf("SWAPI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SwapiResults{}, fmt.Errorf("SWAPI request failed: status %d", resp.StatusCode)
	}

	var all []SwapiPerson
	if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
		return SwapiResults{}, fmt.Errorf("failed to decode SWAPI response: %w", err)
	}

	needle := strings.ToLower(args.Name)
	var matched []SwapiPerson
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name), needle) {
			matched = append(matched, p)
		}
	}

	return SwapiResults{Results: matched}, nil
}

func main() {
	ctx := context.Background()

	modelName := os.Getenv("OLLAMA_MODEL")
	if modelName == "" {
		modelName = "gemma4:12b"
	}
	model := newOllamaModel(modelName)

	swapiTool, err := functiontool.New(
		functiontool.Config{
			Name:        "swapi_people",
			Description: "Search Star Wars characters from SWAPI by name.",
		},
		swapiPeople,
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to create swapi tool: %v", err))
	}

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        "star_wars_lookup",
		Model:       model,
		Description: "Looks up Star Wars characters using SWAPI.",
		Instruction: `You are a Star Wars character lookup assistant. When the user
gives you a character name, call the swapi_people tool and summarize the
result concisely. If the user hasn't given a name, ask for one.`,
		Tools: []tool.Tool{swapiTool},
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to create agent: %v", err))
	}

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	l := full.NewLauncher()
	if err := l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		panic(fmt.Sprintf("Run failed: %v", err))
	}
}
