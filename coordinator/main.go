package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/adk/tool/geminitool"
)

type URLContextArgs struct {
	URL string `json:"url"`
}

type URLContextResult struct {
	Content string `json:"content"`
}

func fetchURLContext(ctx agent.ToolContext, args URLContextArgs) (URLContextResult, error) {
	resp, err := http.Get(args.URL)
	if err != nil {
		return URLContextResult{Content: fmt.Sprintf("Error fetching URL: %v", err)}, nil
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return URLContextResult{Content: fmt.Sprintf("Error reading URL content: %v", err)}, nil
	}

	content := string(bodyBytes)
	if len(content) > 10000 {
		content = content[:10000]
	}
	return URLContextResult{Content: content}, nil
}

func main() {
	ctx := context.Background()

	modelName := os.Getenv("OLLAMA_MODEL")
	if modelName == "" {
		modelName = "gemma4:12b"
	}
	model := newOllamaModel(modelName)

	urlContextTool, err := functiontool.New(
		functiontool.Config{
			Name:        "url_context",
			Description: "Fetch and read the full text content of a webpage at a specified URL.",
		},
		fetchURLContext,
	)
	if err != nil {
		panic(err)
	}

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        "research_assistant",
		Model:       model,
		Description: "Searches the web and reads pages to answer questions.",
		Instruction: `You are a research assistant.

Process:
1. Call google_search with a focused query.
2. Pick the most relevant URL from the snippets.
3. Use url_context to read that page in full.
4. Decide whether you have enough. If not, fetch another URL or run a sharper search.
5. When you have enough, write a concise answer with citations.

Rules:
- Do not answer from memory. Always search first.
- Prefer primary sources over aggregators.
- Stop calling tools as soon as the answer is solid.`,
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
			urlContextTool,
		},
	})
	if err != nil {
		panic(err)
	}

	cfg := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(rootAgent),
	}

	l := full.NewLauncher()
	if err := l.Execute(ctx, cfg, os.Args[1:]); err != nil {
		panic(err)
	}
}
