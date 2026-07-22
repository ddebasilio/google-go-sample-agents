package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type EmptyArgs struct{}

type QuoteResult struct {
	Quote  string `json:"quote"`
	Author string `json:"author"`
}

type WikiArgs struct {
	Query string `json:"query"`
}

type WikiResult struct {
	Result string `json:"result"`
}

func getRandomQuote(ctx agent.ToolContext, _ EmptyArgs) (QuoteResult, error) {
	resp, err := http.Get("https://zenquotes.io/api/random")
	if err != nil {
		return QuoteResult{}, fmt.Errorf("quote request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return QuoteResult{}, fmt.Errorf("quote request failed status %d", resp.StatusCode)
	}

	var data []struct {
		Q string `json:"q"`
		A string `json:"a"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil || len(data) == 0 {
		return QuoteResult{}, fmt.Errorf("failed to parse quote response")
	}

	return QuoteResult{Quote: data[0].Q, Author: data[0].A}, nil
}

func searchWikipedia(ctx agent.ToolContext, args WikiArgs) (WikiResult, error) {
	searchUrl := fmt.Sprintf("https://en.wikipedia.org/w/api.php?action=query&list=search&srsearch=%s&format=json", url.QueryEscape(args.Query))
	resp, err := http.Get(searchUrl)
	if err != nil {
		return WikiResult{}, err
	}
	defer resp.Body.Close()

	var searchData struct {
		Query struct {
			Search []struct {
				Title string `json:"title"`
			} `json:"search"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchData); err != nil || len(searchData.Query.Search) == 0 {
		return WikiResult{Result: "No results found"}, nil
	}

	title := searchData.Query.Search[0].Title
	summaryUrl := fmt.Sprintf("https://en.wikipedia.org/api/rest_v1/page/summary/%s", url.PathEscape(title))
	sumResp, err := http.Get(summaryUrl)
	if err != nil {
		return WikiResult{}, err
	}
	defer sumResp.Body.Close()

	var summaryData struct {
		Extract string `json:"extract"`
	}
	if err := json.NewDecoder(sumResp.Body).Decode(&summaryData); err != nil {
		return WikiResult{Result: "No information found"}, nil
	}

	if summaryData.Extract == "" {
		summaryData.Extract = "No information found"
	}
	return WikiResult{Result: summaryData.Extract}, nil
}

func main() {
	ctx := context.Background()

	modelName := os.Getenv("OLLAMA_MODEL")
	if modelName == "" {
		modelName = "gemma4:e4b"
	}
	model := newOllamaModel(modelName)

	quoteTool, err := functiontool.New(
		functiontool.Config{
			Name:        "get_random_quote",
			Description: "Fetch a random inspirational quote with its author",
		},
		getRandomQuote,
	)
	if err != nil {
		panic(err)
	}

	wikiTool, err := functiontool.New(
		functiontool.Config{
			Name:        "search_wikipedia",
			Description: "Search Wikipedia for biographical information about a person",
		},
		searchWikipedia,
	)
	if err != nil {
		panic(err)
	}

	quoteSchema := &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"quote":  {Type: genai.TypeString, Description: "The text of the quote"},
			"author": {Type: genai.TypeString, Description: "The author of the quote"},
		},
		Required: []string{"quote", "author"},
	}

	quoteFetcherAgent, err := llmagent.New(llmagent.Config{
		Name:         "QuoteFetcherAgent",
		Model:        model,
		Description:  "Fetches a random inspirational quote using the quote tool.",
		Instruction:  `You fetch random quotes using the provided tool. Call the get_random_quote tool, then report the quote text and its author. The structured output schema enforces the response shape — fill in the quote and author fields from the tool result.`,
		Tools:        []tool.Tool{quoteTool},
		OutputSchema: quoteSchema,
		OutputKey:    "quote_result",
	})
	if err != nil {
		panic(err)
	}

	wikipediaResearcherAgent, err := llmagent.New(llmagent.Config{
		Name:        "WikipediaResearcherAgent",
		Model:       model,
		Description: "Researches a person on Wikipedia and returns a concise bio.",
		Instruction: `You research people on Wikipedia. The previous agent fetched a quote — here is its structured output (a JSON object with "quote" and "author" fields):

{quote_result}

Take the "author" field and search for that person using the Wikipedia tool. Return a concise 2-3 sentence bio about who they are and why they are notable.`,
		Tools:     []tool.Tool{wikiTool},
		OutputKey: "author_bio",
	})
	if err != nil {
		panic(err)
	}

	inspirationCardAgent, err := llmagent.New(llmagent.Config{
		Name:        "InspirationCardAgent",
		Model:       model,
		Description: "Writes a punchy one-line daily inspiration card.",
		Instruction: `You write punchy "Daily Inspiration" cards. Combine the quote with the author's background into exactly ONE line that ends with "— <Author Name>". No preamble, no quotes around the output, no extra formatting.

Quote information:
{quote_result}

Author bio:
{author_bio}`,
		OutputKey: "inspiration_card",
	})
	if err != nil {
		panic(err)
	}

	rootAgent, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:        "QuoteNerdPipeline",
			Description: "Fetches a quote, researches the author, and writes an inspiration card.",
			SubAgents:   []agent.Agent{quoteFetcherAgent, wikipediaResearcherAgent, inspirationCardAgent},
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
