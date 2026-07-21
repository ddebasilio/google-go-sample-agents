package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"os"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

type OllamaModel struct {
	modelName string
	baseURL   string
}

func newOllamaModel(modelName string, baseURL ...string) model.LLM {
	url := os.Getenv("OLLAMA_BASE_URL")
	if url == "" {
		url = "http://localhost:11434"
	}
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = baseURL[0]
	}
	return &OllamaModel{
		modelName: modelName,
		baseURL:   url,
	}
}

func (m *OllamaModel) Name() string {
	return m.modelName
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIChatReq struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIChatResp struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

func (m *OllamaModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		var messages []openAIMessage

		if req.Config != nil && req.Config.SystemInstruction != nil {
			var sysText string
			for _, p := range req.Config.SystemInstruction.Parts {
				if p.Text != "" {
					sysText += p.Text
				}
			}
			if sysText != "" {
				messages = append(messages, openAIMessage{
					Role:    "system",
					Content: sysText,
				})
			}
		}

		for _, c := range req.Contents {
			role := c.Role
			if role == "model" {
				role = "assistant"
			}
			if role == "" {
				role = "user"
			}

			var text string
			var toolCalls []openAIToolCall

			for _, p := range c.Parts {
				if p.Text != "" {
					text += p.Text
				}
				if p.FunctionCall != nil {
					argsBytes, _ := json.Marshal(p.FunctionCall.Args)
					toolCalls = append(toolCalls, openAIToolCall{
						ID:   fmt.Sprintf("call_%s", p.FunctionCall.Name),
						Type: "function",
						Function: struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      p.FunctionCall.Name,
							Arguments: string(argsBytes),
						},
					})
				}
				if p.FunctionResponse != nil {
					respBytes, _ := json.Marshal(p.FunctionResponse.Response)
					messages = append(messages, openAIMessage{
						Role:       "tool",
						Content:    string(respBytes),
						ToolCallID: fmt.Sprintf("call_%s", p.FunctionResponse.Name),
					})
				}
			}

			if text != "" || len(toolCalls) > 0 {
				messages = append(messages, openAIMessage{
					Role:      role,
					Content:   text,
					ToolCalls: toolCalls,
				})
			}
		}

		payload := openAIChatReq{
			Model:    m.modelName,
			Messages: messages,
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			yield(nil, err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v1/chat/completions", bytes.NewBuffer(bodyBytes))
		if err != nil {
			yield(nil, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			yield(nil, fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(b)))
			return
		}

		var chatResp openAIChatResp
		if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
			yield(nil, err)
			return
		}

		if len(chatResp.Choices) == 0 {
			yield(nil, fmt.Errorf("no choices returned from ollama model %s", m.modelName))
			return
		}

		msg := chatResp.Choices[0].Message
		var parts []*genai.Part

		if msg.Content != "" {
			parts = append(parts, genai.NewPartFromText(msg.Content))
		}

		for _, tc := range msg.ToolCalls {
			var args map[string]any
			if tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			parts = append(parts, genai.NewPartFromFunctionCall(tc.Function.Name, args))
		}

		if len(parts) == 0 {
			parts = append(parts, genai.NewPartFromText(""))
		}

		res := &model.LLMResponse{
			Content: &genai.Content{
				Role:  "model",
				Parts: parts,
			},
		}

		yield(res, nil)
	}
}
