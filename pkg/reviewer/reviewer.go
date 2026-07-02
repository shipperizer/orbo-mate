package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"

	"github.com/google/go-github/v60/github"
	"github.com/shipperizer/orbo-mate/pkg/config"
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

	log.Printf("Triggered review for PR #%d using model: %s", prNumber, targetModel)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: r.cfg.GitHubToken})
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, r.httpClient)
	tc := oauth2.NewClient(ctxWithClient, ts)
	ghClient := github.NewClient(tc)

	diffData, err := r.FetchPRDiff(ctx, tc, repoOwner, repoName, prNumber)
	if err != nil {
		log.Printf("Error fetching PR diff: %v", err)
		r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, "❌ Failed to fetch PR diff for review.")
		return
	}

	reviewOutput, err := r.GetOpenRouterReview(ctx, targetModel, diffData)
	if err != nil {
		log.Printf("OpenRouter API error: %v", err)
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
	apiURL := "https://openrouter.ai/api/v1/chat/completions"

	prompt := fmt.Sprintf("%s\n\n```diff\n%s\n```", r.cfg.ContextSentence, diff)

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

// PostComment posts a comment to the specified PR thread.
func (r *Reviewer) PostComment(ctx context.Context, client *github.Client, owner, repo string, prNumber int, message string) {
	comment := &github.IssueComment{Body: github.String(message)}
	_, _, err := client.Issues.CreateComment(ctx, owner, repo, prNumber, comment)
	if err != nil {
		log.Printf("Failed to post comment to PR #%d: %v", prNumber, err)
	}
}
