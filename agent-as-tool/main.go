package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
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
		modelName = "gemma4:e4b"
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

	findingsSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"findings": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"point":  {Type: genai.TypeString, Description: "A concise finding that addresses the brief"},
						"source": {Type: genai.TypeString, Description: "The source URL backing this finding"},
					},
					Required: []string{"point", "source"},
				},
				Description: "4 to 8 findings covering the brief, each with a source URL",
			},
		},
		Required: []string{"findings"},
	}

	critiqueSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"verdict": {
				Type:        genai.TypeString,
				Enum:        []string{"APPROVED", "NEEDS_REVISION"},
				Description: "APPROVED if the findings fully cover the request, otherwise NEEDS_REVISION",
			},
			"gaps": {
				Type: genai.TypeArray,
				Items: &genai.Schema{
					Type: genai.TypeString,
				},
				Description: "Specific, actionable gaps a researcher could act on directly",
			},
		},
		Required: []string{"verdict", "gaps"},
	}

	researcher, err := llmagent.New(llmagent.Config{
		Name:        "researcher",
		Model:       model,
		Description: "Researches a topic on the web. Accepts a brief and returns findings with sources. Can be re-called with a sharper brief to fill specific gaps.",
		Instruction: `You are a researcher.

Given a brief, search the web and return 4 to 8 findings. Each finding must be a single concise point paired with the source URL it came from.

If the brief targets specific gaps (e.g. "find compliance deadlines"), focus only on those gaps. Do not repeat earlier ground.

The structured output schema enforces the shape — populate the findings array; do not add preamble.`,
		Tools: []tool.Tool{
			geminitool.GoogleSearch{},
			urlContextTool,
		},
		OutputSchema: findingsSchema,
		OutputKey:    "findings",
	})
	if err != nil {
		panic(err)
	}

	critic, err := llmagent.New(llmagent.Config{
		Name:        "critic",
		Model:       model,
		Description: "Reviews research findings for completeness. Returns a structured verdict (APPROVED or NEEDS_REVISION) with specific gaps.",
		Instruction: `You are a research critic.

Read the findings against the original user request. Check for:
- Missing facts directly implied by the request
- Vague claims without sources
- Outdated or generic information where specifics are needed

Set verdict to "APPROVED" if the findings fully cover the request, otherwise "NEEDS_REVISION". When NEEDS_REVISION, list each gap in the gaps array — each gap must be concrete enough that a researcher could act on it directly. When APPROVED, leave gaps as an empty array.

Do not rewrite the findings. Only judge them.`,
		OutputSchema: critiqueSchema,
	})
	if err != nil {
		panic(err)
	}

	researcherTool := agenttool.New(researcher, nil)
	criticTool := agenttool.New(critic, nil)

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        "research_root",
		Model:       model,
		Description: "Iteratively researches a topic with a critic-driven refinement loop.",
		Instruction: `You orchestrate research with a critic.

Both tools return structured JSON:
- researcher returns { findings: [{ point, source }] }.
- critic returns { verdict: "APPROVED" | "NEEDS_REVISION", gaps: [string] }.

Process:
1. Call researcher with a brief derived from the user's request.
2. Call critic with the findings.
3. If critic's verdict is "APPROVED", write the final report and stop.
4. If critic's verdict is "NEEDS_REVISION", call researcher AGAIN with a brief targeting ONLY the entries in critic's gaps array. Do not re-research what you already have.
5. Call critic again with the combined findings.
6. Repeat steps 4 and 5 until the verdict is "APPROVED".

Rules:
- Always start by calling researcher. Never answer from memory.
- When re-calling researcher, the brief must reference the specific gaps from the critic.
- The final report should integrate all research passes, not just the last one.
- Cite the source URL from each finding in the final report.`,
		Tools: []tool.Tool{researcherTool, criticTool},
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
