package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shipperizer/orbo-mate/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// Chat sends a raw prompt to OpenRouter and returns the response.
func (r *Reviewer) Chat(ctx context.Context, model, prompt string) (string, error) {
	tracer := otel.Tracer("orbo-mate")
	ctx, span := tracer.Start(ctx, "Chat")
	defer span.End()

	span.SetAttributes(
		attribute.String("model", model),
		attribute.Int("prompt_length", len(prompt)),
	)

	apiURL := "https://openrouter.ai/api/v1/chat/completions"

	logger.Infof("Sending chat request to OpenRouter (Model: %s, Prompt Length: %d bytes)...", model, len(prompt))

	payload := OpenRouterRequest{
		Model: model,
		Messages: []OpenRouterMessage{
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("Failed to marshal OpenRouter chat payload: %v", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("Failed to create OpenRouter chat HTTP request: %v", err)
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.cfg.OpenRouterKey)
	req.Header.Set("HTTP-Referer", "https://github.com/ai-bot-reviewer")
	req.Header.Set("X-OpenRouter-Title", "Go GitHub Code Reviewer")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("OpenRouter chat HTTP request failed: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	logger.Infof("Received chat response from OpenRouter (Status: %s, Code: %d)", resp.Status, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		err = logThoroughOpenRouterError(ctx, resp, jsonData)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	var orResp OpenRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&orResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("Failed to decode OpenRouter chat response JSON: %v", err)
		return "", err
	}

	if len(orResp.Choices) > 0 {
		content := orResp.Choices[0].Message.Content
		logger.Infof("OpenRouter chat completed successfully. Received response (length: %d characters)", len(content))
		span.SetStatus(codes.Ok, "Chat fetched successfully")
		return content, nil
	}

	err = fmt.Errorf("empty choice array returned from AI model")
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	logger.Errorf("OpenRouter chat API error: %v", err)
	return "", err
}
