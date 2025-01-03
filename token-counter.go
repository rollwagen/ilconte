package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	apiURL     = "https://api.anthropic.com/v1/messages/count_tokens"
	apiVersion = "2023-06-01"
	apiTimeout = 10 // seconds
)

type TokenRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TokenResponse struct {
	InputTokens int `json:"input_tokens"`
}

type APIError struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	// Parse command line flags
	model := flag.String("model", "claude-3-sonnet-20240229", "Model to use for token counting")
	verbose := flag.Bool("verbose", false, "Show detailed output")
	flag.Parse()

	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing Anthropic API key. Set ANTHROPIC_API_KEY environment variable.")
	}

	// Read input from files or stdin
	var text, fileInput, pipedInput string
	var err error
	if flag.NArg() > 0 {
		fileInput, err = readFiles(flag.Args())
	}
	if err != nil {
		log.Fatalf("Error reading file input: %v", err)
	}

	pipedInput, err = readStdin()
	if err != nil {
		log.Fatalf("Error reading piped input: %v", err)
	}

	text = pipedInput + fileInput

	if text == "" {
		log.Fatal("Error: No input provided. Pipe text via stdin and/or provide file paths")
	}

	// Count tokens
	count, err := countTokens(text, apiKey, *model)
	if err != nil {
		log.Fatalf("Error counting tokens: %v", err)
	}

	// Output results
	if *verbose {
		fmt.Printf("Model: %s\n", *model)
		fmt.Printf("Input length: %d characters\n", len(text))
	}
	fmt.Printf("Token count: %d\n", count)
}

func readStdin() (string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("error checking stdin: %w", err)
	}

	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", nil // No data piped to stdin
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("error reading from stdin: %w", err)
	}

	return string(data), nil
}

func readFiles(paths []string) (string, error) {
	var texts []string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("error reading file %s: %w", path, err)
		}
		texts = append(texts, string(data))
	}
	return strings.Join(texts, "\n"), nil
}

func countTokens(text string, apiKey, model string) (int, error) {
	if text == "" {
		return 0, errors.New("empty input text")
	}

	// Prepare request
	reqBody := TokenRequest{
		Model: model,
		Messages: []Message{
			{
				Role:    "user",
				Content: text,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("error marshaling request: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return 0, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("content-type", "application/json")

	// Send request
	createHTTPClient := func() *http.Client {
		return &http.Client{
			Timeout: apiTimeout * time.Second,
		}
	}
	client := createHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("error reading response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode != http.StatusOK {
		var apiError APIError
		if err := json.Unmarshal(body, &apiError); err == nil && apiError.Error.Message != "" {
			return 0, fmt.Errorf("API error: %s", apiError.Error.Message)
		}
		return 0, fmt.Errorf("API error: %s", resp.Status)
	}

	// Parse response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return 0, fmt.Errorf("error parsing response: %w", err)
	}

	return tokenResp.InputTokens, nil
}
