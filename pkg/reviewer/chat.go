package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Chat sends a raw prompt to OpenRouter and returns the response.
func (r *Reviewer) Chat(ctx context.Context, model, prompt string) (string, error) {
	apiURL := "https://openrouter.ai/api/v1/chat/completions"

	payload := OpenRouterRequest{
		Model: model,
		Messages: []OpenRouterMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.cfg.OpenRouterKey)
	req.Header.Set("HTTP-Referer", "https://github.com/ai-bot-reviewer")
	req.Header.Set("X-OpenRouter-Title", "Go GitHub Code Reviewer")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openrouter error status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var orResp OpenRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&orResp); err != nil {
		return "", err
	}

	if len(orResp.Choices) > 0 {
		return orResp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("empty choice array returned from AI model")
}
