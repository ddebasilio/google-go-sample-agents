package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type InteractionRequest struct {
	Agent       string `json:"agent"`
	Input       string `json:"input"`
	Environment string `json:"environment"`
}

type InteractionResponse struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environment_id"`
	OutputText    string `json:"output_text"`
}

func createInteraction(ctx context.Context, apiKey string, req InteractionRequest) (*InteractionResponse, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/interactions?key=%s", apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("interactions API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var res InteractionResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &res, nil
}

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}

	interaction, err := createInteraction(ctx, apiKey, InteractionRequest{
		Agent:       "antigravity-preview-05-2026",
		Input:       "Write a Python script that generates the first 20 Fibonacci numbers and saves them to fibonacci.txt. Then read the file and print its contents.",
		Environment: "remote",
	})
	if err != nil {
		fmt.Printf("Error creating interaction 1: %v\n", err)
		return
	}

	fmt.Printf("Interaction ID: %s\n", interaction.ID)
	fmt.Printf("Environment ID: %s\n", interaction.EnvironmentID)
	fmt.Printf("Output: %s\n", interaction.OutputText)

	interaction2, err := createInteraction(ctx, apiKey, InteractionRequest{
		Agent:       "antigravity-preview-05-2026",
		Input:       "what's the sum of the individual numbers of the last calculated number?",
		Environment: interaction.EnvironmentID,
	})
	if err != nil {
		fmt.Printf("Error creating interaction 2: %v\n", err)
		return
	}

	fmt.Printf("Output: %s\n", interaction2.OutputText)
}
