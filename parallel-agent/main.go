package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/parallelagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type TranslateArgs struct {
	Text       string `json:"text"`
	TargetLang string `json:"target_lang"`
}

type TranslateResult struct {
	Translation string `json:"translation"`
}

func translateText(ctx agent.ToolContext, args TranslateArgs) (TranslateResult, error) {
	apiURL := fmt.Sprintf("https://api.mymemory.translated.net/get?q=%s&langpair=en|%s", url.QueryEscape(args.Text), url.QueryEscape(args.TargetLang))
	resp, err := http.Get(apiURL)
	if err != nil {
		return TranslateResult{}, err
	}
	defer resp.Body.Close()

	var data struct {
		ResponseData struct {
			TranslatedText string `json:"translatedText"`
		} `json:"responseData"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return TranslateResult{}, err
	}

	return TranslateResult{Translation: data.ResponseData.TranslatedText}, nil
}

func main() {
	ctx := context.Background()

	modelName := os.Getenv("OLLAMA_MODEL")
	if modelName == "" {
		modelName = "gemma4:12b"
	}
	model := newOllamaModel(modelName)

	translateTool, err := functiontool.New(
		functiontool.Config{
			Name:        "translate_text",
			Description: "Translate text from English to a target language using the MyMemory translation API",
		},
		translateText,
	)
	if err != nil {
		panic(err)
	}

	frenchAgent, err := llmagent.New(llmagent.Config{
		Name:        "FrenchTranslator",
		Model:       model,
		Description: "Translates the user's text into French.",
		Instruction: `You are a French translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "fr". Return only the translated text, nothing else.`,
		Tools:       []tool.Tool{translateTool},
		OutputKey:   "french_translation",
	})
	if err != nil {
		panic(err)
	}

	japaneseAgent, err := llmagent.New(llmagent.Config{
		Name:        "JapaneseTranslator",
		Model:       model,
		Description: "Translates the user's text into Japanese.",
		Instruction: `You are a Japanese translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "ja". Return only the translated text, nothing else.`,
		Tools:       []tool.Tool{translateTool},
		OutputKey:   "japanese_translation",
	})
	if err != nil {
		panic(err)
	}

	spanishAgent, err := llmagent.New(llmagent.Config{
		Name:        "SpanishTranslator",
		Model:       model,
		Description: "Translates the user's text into Spanish.",
		Instruction: `You are a Spanish translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "es". Return only the translated text, nothing else.`,
		Tools:       []tool.Tool{translateTool},
		OutputKey:   "spanish_translation",
	})
	if err != nil {
		panic(err)
	}

	italianAgent, err := llmagent.New(llmagent.Config{
		Name:        "ItalianTranslator",
		Model:       model,
		Description: "Translates the user's text into italian.",
		Instruction: `You are an italian translator. Whatever the user says, treat their entire message as text to translate. Immediately call the translate_text tool with the user's exact message as the text and target_lang "it". Return only the translated text, nothing else.`,
		Tools:       []tool.Tool{translateTool},
		OutputKey:   "italian_translation",
	})
	if err != nil {
		panic(err)
	}

	parallelTranslator, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name:        "ParallelTranslator",
			Description: "Runs Italian, French, Japanese, and Spanish translators in parallel.",
			SubAgents:   []agent.Agent{italianAgent, frenchAgent, japaneseAgent, spanishAgent},
		},
	})
	if err != nil {
		panic(err)
	}

	aggregatorAgent, err := llmagent.New(llmagent.Config{
		Name:        "TranslationAggregator",
		Model:       model,
		Description: "Aggregates parallel translation results and provides linguistic insights.",
		Instruction: `You are an aggregator in a parallel agent pipeline. You receive four translations of the same word or phrase. Present them clearly, then add a one-sentence note on any interesting linguistic differences between them.

Italian: {italian_translation}
French: {french_translation}
Japanese: {japanese_translation}
Spanish: {spanish_translation}`,
		OutputKey: "aggregated_result",
	})
	if err != nil {
		panic(err)
	}

	rootAgent, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:        "ParallelTranslationPipeline",
			Description: "Translates text into 4 languages in parallel, then aggregates the results.",
			SubAgents:   []agent.Agent{parallelTranslator, aggregatorAgent},
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
