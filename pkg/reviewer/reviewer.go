package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/google/go-github/v60/github"
	"github.com/shipperizer/orbo-mate/pkg/config"
	"github.com/shipperizer/orbo-mate/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/oauth2"
)

// Reviewer processes the issue comments to trigger code reviews.
type Reviewer struct {
	cfg        *config.Config
	httpClient *http.Client
}

// NewReviewer creates a new Reviewer with the specified configuration and HTTP client.
func NewReviewer(cfg *config.Config, httpClient *http.Client) *Reviewer {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Reviewer{
		cfg:        cfg,
		httpClient: httpClient,
	}
}

// ProcessComment checks if a comment triggers the review bot and runs the review process.
func (r *Reviewer) ProcessComment(ctx context.Context, event *github.IssueCommentEvent) {
	commentBody := event.GetComment().GetBody()

	pattern := fmt.Sprintf(`%s\s+review\s+with\s+([a-zA-Z0-9\-\/:_]+)`, regexp.QuoteMeta(r.cfg.BotName))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(commentBody)

	if len(matches) < 2 {
		return
	}

	targetModel := matches[1]
	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := event.GetIssue().GetNumber()

	logger.Infof("Triggered review for PR #%d using model: %s", prNumber, targetModel)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: r.cfg.GitHubToken})
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, r.httpClient)
	tc := oauth2.NewClient(ctxWithClient, ts)
	ghClient := github.NewClient(tc)

	diffData, err := r.FetchPRDiff(ctx, tc, repoOwner, repoName, prNumber)
	if err != nil {
		logger.Errorf("Error fetching PR diff: %v", err)
		r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, "❌ Failed to fetch PR diff for review.")
		return
	}

	reviewOutput, err := r.GetOpenRouterReview(ctx, targetModel, diffData)
	if err != nil {
		logger.Errorf("OpenRouter API error: %v", err)
		r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, "❌ Failed to generate review from OpenRouter.")
		return
	}

	responseBlock := fmt.Sprintf("### 🤖 Automated Review by %s\n*Model used: `%s`*\n\n%s", r.cfg.BotName, targetModel, reviewOutput)
	r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, responseBlock)
}

// FetchPRDiff retrieves the unified diff from the GitHub API.
func (r *Reviewer) FetchPRDiff(ctx context.Context, client *http.Client, owner, repo string, prNumber int) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch diff, status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// OpenRouter API structures
type OpenRouterMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenRouterRequest struct {
	Model    string              `json:"model"`
	Messages []OpenRouterMessage `json:"messages"`
}

type OpenRouterResponse struct {
	Choices []struct {
		Message OpenRouterMessage `json:"message"`
	} `json:"choices"`
}

// GetOpenRouterReview calls OpenRouter API to get code review recommendations.
func (r *Reviewer) GetOpenRouterReview(ctx context.Context, model, diff string) (string, error) {
	tracer := otel.Tracer("orbo-mate")
	ctx, span := tracer.Start(ctx, "GetOpenRouterReview")
	defer span.End()

	span.SetAttributes(
		attribute.String("model", model),
		attribute.Int("diff_length", len(diff)),
	)

	apiURL := "https://openrouter.ai/api/v1/chat/completions"

	logger.Infof("Sending request to OpenRouter (Model: %s, Diff Length: %d bytes)...", model, len(diff))

	prompt := fmt.Sprintf("%s\n\n```diff\n%s\n```", r.cfg.ContextSentence, diff)

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
		logger.Errorf("Failed to marshal OpenRouter payload: %v", err)
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("Failed to create OpenRouter HTTP request: %v", err)
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
		logger.Errorf("OpenRouter HTTP request failed: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	logger.Infof("Received response from OpenRouter (Status: %s, Code: %d)", resp.Status, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("openrouter error status %d: %s", resp.StatusCode, string(bodyBytes))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("OpenRouter API request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
		return "", err
	}

	var orResp OpenRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&orResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Errorf("Failed to decode OpenRouter response JSON: %v", err)
		return "", err
	}

	if len(orResp.Choices) > 0 {
		content := orResp.Choices[0].Message.Content
		logger.Infof("OpenRouter API call completed successfully. Received review (length: %d characters)", len(content))
		span.SetStatus(codes.Ok, "Review fetched successfully")
		return content, nil
	}

	err = fmt.Errorf("empty choice array returned from AI model")
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	logger.Errorf("OpenRouter API error: %v", err)
	return "", err
}

// PostComment posts a comment to the specified PR thread.
func (r *Reviewer) PostComment(ctx context.Context, client *github.Client, owner, repo string, prNumber int, message string) {
	comment := &github.IssueComment{Body: github.String(message)}
	_, _, err := client.Issues.CreateComment(ctx, owner, repo, prNumber, comment)
	if err != nil {
		logger.Errorf("Failed to post comment to PR #%d: %v", prNumber, err)
	}
}
