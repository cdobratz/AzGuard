package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Response struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

type Provider interface {
	Chat(messages []Message) (string, error)
	Name() string
}

type OllamaProvider struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	return &OllamaProvider{
		BaseURL: baseURL,
		Model:   model,
		Client:  &http.Client{},
	}
}

func (o *OllamaProvider) Name() string {
	return "ollama"
}

func (o *OllamaProvider) Chat(messages []Message) (string, error) {
	url := o.BaseURL + "/api/chat"

	req := Request{
		Model:    o.Model,
		Messages: messages,
		Stream:   false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.Client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from ollama")
	}

	return result.Choices[0].Message.Content, nil
}

type AnthropicProvider struct {
	APIKey string
	Model  string
	Client *http.Client
}

func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *AnthropicProvider) Name() string {
	return "anthropic"
}

type AnthropicRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Messages    []Message `json:"messages"`
}

type AnthropicResponse struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
}

func (a *AnthropicProvider) Chat(messages []Message) (string, error) {
	if a.APIKey == "" {
		a.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if a.APIKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	url := "https://api.anthropic.com/v1/messages"

	req := AnthropicRequest{
		Model:     a.Model,
		MaxTokens: 4096,
		Messages:  messages,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.Client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anthropic request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result AnthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from anthropic")
	}

	return result.Content[0].Text, nil
}

type CLIProvider struct {
	Model string
}

func NewCLIProvider(model string) *CLIProvider {
	return &CLIProvider{Model: model}
}

func (c *CLIProvider) Name() string {
	return "cli"
}

func (c *CLIProvider) Chat(messages []Message) (string, error) {
	var allContent []string
	for _, m := range messages {
		allContent = append(allContent, m.Content)
	}

	prompt := strings.Join(allContent, "\n\n")

	cmd := exec.Command("az", "ai", "exec", "--prompt", prompt)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute CLI command: %w", err)
	}

	return string(output), nil
}

func NewProvider(providerType, baseURL, model, apiKey string) (Provider, error) {
	switch providerType {
	case "ollama":
		return NewOllamaProvider(baseURL, model), nil
	case "anthropic":
		return NewAnthropicProvider(apiKey, model), nil
	case "cli":
		return NewCLIProvider(model), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}
