package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

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
	if event == nil || event.GetIssue() == nil {
		return
	}
	commentBody := event.GetComment().GetBody()
	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := event.GetIssue().GetNumber()

	botRegexName := regexp.QuoteMeta(r.cfg.BotName)
	if !strings.HasPrefix(botRegexName, "@") {
		botRegexName = "@?" + botRegexName
	} else {
		botRegexName = "@?" + botRegexName[1:]
	}

	solvePattern := fmt.Sprintf(`(?i)%s\s+(?:try\s+to\s+solve|find\s+a\s+solution|solve).*?(?:with\s+model|using|with)\s+`+"`"+`?([a-zA-Z0-9\-\.\/:_]+)`+"`"+`?`, botRegexName)
	solveRe := regexp.MustCompile(solvePattern)
	solveMatches := solveRe.FindStringSubmatch(commentBody)

	pattern := fmt.Sprintf(`%s\s+review\s+with\s+([a-zA-Z0-9\-\/:_]+)`, regexp.QuoteMeta(r.cfg.BotName))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(commentBody)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: r.cfg.GitHubToken})
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, r.httpClient)
	tc := oauth2.NewClient(ctxWithClient, ts)
	ghClient := github.NewClient(tc)

	targetModel := r.cfg.DefaultModel
	isSolveCmd := false
	if len(solveMatches) >= 2 {
		targetModel = solveMatches[1]
		isSolveCmd = true
	} else if len(matches) >= 2 {
		targetModel = matches[1]
	}

	if isSolveCmd {
		logger.Infof("Triggered solve response for issue #%d using model: %s", prNumber, targetModel)
		var prompt string
		if event.GetIssue().IsPullRequest() {
			diffData, err := r.FetchPRDiff(ctx, tc, repoOwner, repoName, prNumber)
			if err != nil {
				logger.Errorf("Error fetching PR diff: %v", err)
				r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, "❌ Failed to fetch PR diff for solve context.")
				return
			}
			prompt = fmt.Sprintf("You are an expert AI assistant tasked with finding a solution for this issue/PR.\n\nIssue #%d: %s\n\nDescription:\n%s\n\nComment request: %s\n\nPR Diff:\n```diff\n%s\n```\n\nPlease reply directly to suggest a solution.", prNumber, event.GetIssue().GetTitle(), event.GetIssue().GetBody(), commentBody, diffData)
		} else {
			prompt = fmt.Sprintf("You are an expert AI assistant tasked with finding a solution for this issue.\n\nIssue #%d: %s\n\nDescription:\n%s\n\nComment request: %s\n\nPlease reply directly to suggest a solution.", prNumber, event.GetIssue().GetTitle(), event.GetIssue().GetBody(), commentBody)
		}

		response, err := r.Chat(ctx, targetModel, prompt)
		if err != nil {
			logger.Errorf("OpenRouter Chat error: %v", err)
			r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, userFriendlyError(err, "❌ Failed to generate response for the comment."))
			return
		}

		responseBlock := fmt.Sprintf("### 🤖 Automated Response by %s\n\n%s", r.cfg.BotName, response)
		r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, responseBlock)
		return
	}

	if event.GetIssue().IsPullRequest() {
		if len(matches) >= 2 {
			logger.Infof("Triggered review for PR #%d using model: %s", prNumber, targetModel)

			diffData, err := r.FetchPRDiff(ctx, tc, repoOwner, repoName, prNumber)
			if err != nil {
				logger.Errorf("Error fetching PR diff: %v", err)
				r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, "❌ Failed to fetch PR diff for review.")
				return
			}

			reviewOutput, err := r.GetOpenRouterReview(ctx, targetModel, diffData)
			if err != nil {
				logger.Errorf("OpenRouter API error: %v", err)
				r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, userFriendlyError(err, "❌ Failed to generate review from OpenRouter."))
				return
			}

			responseBlock := fmt.Sprintf("### 🤖 Automated Review by %s\n*Model used: `%s`*\n\n%s", r.cfg.BotName, targetModel, reviewOutput)
			r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, responseBlock)
		} else {
			logger.Infof("Triggered chat response for PR #%d using model: %s", prNumber, targetModel)
			prompt := fmt.Sprintf("A user tagged you in a comment on PR #%d.\nTitle: %s\nComment: %s\n\nPlease reply directly to their query.", prNumber, event.GetIssue().GetTitle(), commentBody)
			response, err := r.Chat(ctx, targetModel, prompt)
			if err != nil {
				logger.Errorf("OpenRouter Chat error: %v", err)
				r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, userFriendlyError(err, "❌ Failed to generate response for the comment."))
				return
			}

			responseBlock := fmt.Sprintf("### 🤖 Automated Response by %s\n\n%s", r.cfg.BotName, response)
			r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, responseBlock)
		}
	} else {
		logger.Infof("Triggered issue comment response for issue #%d using model: %s", prNumber, targetModel)

		prompt := fmt.Sprintf("A user tagged you in a comment on issue #%d.\nTitle: %s\nComment: %s\n\nPlease reply directly to their query.", prNumber, event.GetIssue().GetTitle(), commentBody)
		response, err := r.Chat(ctx, targetModel, prompt)
		if err != nil {
			logger.Errorf("OpenRouter Chat error: %v", err)
			r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, userFriendlyError(err, "❌ Failed to generate response for the comment."))
			return
		}

		responseBlock := fmt.Sprintf("### 🤖 Automated Response by %s\n\n%s", r.cfg.BotName, response)
		r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, responseBlock)
	}
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
	Model     string              `json:"model"`
	Messages  []OpenRouterMessage `json:"messages"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
}

type OpenRouterResponse struct {
	Choices []struct {
		Message OpenRouterMessage `json:"message"`
	} `json:"choices"`
}

type OpenRouterErrorResponse struct {
	Error struct {
		Message  string      `json:"message"`
		Code     interface{} `json:"code"`
		Metadata interface{} `json:"metadata,omitempty"`
	} `json:"error"`
}

// logThoroughOpenRouterError logs a thorough report of an OpenRouter API error response,
// including status codes, headers, and the full request payload/context.
func logThoroughOpenRouterError(ctx context.Context, resp *http.Response, requestPayload []byte) error {
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		bodyBytes = []byte(fmt.Sprintf("<failed to read body: %v>", err))
	}

	// 1. Redact Authorization Header in request for safety
	reqHeaders := make(map[string]string)
	if resp.Request != nil {
		for k, v := range resp.Request.Header {
			if strings.ToLower(k) == "authorization" {
				reqHeaders[k] = "Bearer <redacted>"
			} else {
				reqHeaders[k] = strings.Join(v, ", ")
			}
		}
	}

	// 2. Format Response Headers
	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		respHeaders[k] = strings.Join(v, ", ")
	}

	// 3. Attempt to Parse JSON Error response
	var parsedError string
	var errResp OpenRouterErrorResponse
	if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Error.Message != "" {
		parsedError = fmt.Sprintf("Code: %v, Message: %q, Metadata: %+v", errResp.Error.Code, errResp.Error.Message, errResp.Error.Metadata)
	} else {
		parsedError = "<could not parse structured JSON error>"
	}

	// 4. Log everything in a highly thorough format
	logger.Errorf("=== OpenRouter API Error Report ===")
	if resp.Request != nil && resp.Request.URL != nil {
		logger.Errorf("Request URL: %s", resp.Request.URL.String())
	}
	logger.Errorf("Request Headers: %+v", reqHeaders)
	logger.Errorf("Request Payload: %s", string(requestPayload))
	logger.Errorf("Response Status: %s (Code: %d)", resp.Status, resp.StatusCode)
	logger.Errorf("Response Headers: %+v", respHeaders)
	logger.Errorf("Raw Response Body: %s", string(bodyBytes))
	logger.Errorf("Parsed OpenRouter Error: %s", parsedError)
	logger.Errorf("==================================")

	// Return a clean error representing the failure
	if errResp.Error.Message != "" {
		return fmt.Errorf("openrouter error status %d (code %v): %s", resp.StatusCode, errResp.Error.Code, errResp.Error.Message)
	}
	return fmt.Errorf("openrouter error status %d: %s", resp.StatusCode, string(bodyBytes))
}

// ProcessIssueAssigned processes assigned issue events by sending the description to OpenRouter Chat and replying on the thread.
func (r *Reviewer) ProcessIssueAssigned(ctx context.Context, event *github.IssuesEvent) {
	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	issueNumber := event.GetIssue().GetNumber()
	issueTitle := event.GetIssue().GetTitle()
	issueBody := event.GetIssue().GetBody()

	logger.Infof("Triggered response for assigned issue #%d", issueNumber)

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: r.cfg.GitHubToken})
	ctxWithClient := context.WithValue(ctx, oauth2.HTTPClient, r.httpClient)
	tc := oauth2.NewClient(ctxWithClient, ts)
	ghClient := github.NewClient(tc)

	prompt := fmt.Sprintf("You have been assigned to this GitHub issue.\nTitle: %s\nDescription: %s\n\nPlease suggest how to solve or address this issue.", issueTitle, issueBody)
	response, err := r.Chat(ctx, r.cfg.DefaultModel, prompt)
	if err != nil {
		logger.Errorf("OpenRouter Chat error: %v", err)
		r.PostComment(ctx, ghClient, repoOwner, repoName, issueNumber, userFriendlyError(err, "❌ Failed to generate response for assigned issue."))
		return
	}

	responseBlock := fmt.Sprintf("### 🤖 Automated Response by %s\n\n%s", r.cfg.BotName, response)
	r.PostComment(ctx, ghClient, repoOwner, repoName, issueNumber, responseBlock)
}

// ProcessPRAssigned automatically reviews a PR with the default model when assigned.
func (r *Reviewer) ProcessPRAssigned(ctx context.Context, event *github.PullRequestEvent) {
	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := event.GetPullRequest().GetNumber()

	logger.Infof("Triggered review for assigned PR #%d using default model: %s", prNumber, r.cfg.DefaultModel)

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

	reviewOutput, err := r.GetOpenRouterReview(ctx, r.cfg.DefaultModel, diffData)
	if err != nil {
		logger.Errorf("OpenRouter API error: %v", err)
		r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, userFriendlyError(err, "❌ Failed to generate review from OpenRouter."))
		return
	}

	responseBlock := fmt.Sprintf("### 🤖 Automated Review by %s\n*Model used: `%s` (Default)*\n\n%s", r.cfg.BotName, r.cfg.DefaultModel, reviewOutput)
	r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, responseBlock)
}

// ProcessPRReviewComment processes PR review comments by querying Chat with full diff/comment context.
func (r *Reviewer) ProcessPRReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent) {
	commentBody := event.GetComment().GetBody()
	repoOwner := event.GetRepo().GetOwner().GetLogin()
	repoName := event.GetRepo().GetName()
	prNumber := event.GetPullRequest().GetNumber()

	pattern := fmt.Sprintf(`%s\s+review\s+with\s+([a-zA-Z0-9\-\/:_]+)`, regexp.QuoteMeta(r.cfg.BotName))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(commentBody)

	var targetModel string
	if len(matches) >= 2 {
		targetModel = matches[1]
	} else {
		targetModel = r.cfg.DefaultModel
	}

	logger.Infof("Triggered PR review comment response for PR #%d using model: %s", prNumber, targetModel)

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

	prompt := fmt.Sprintf("A user asked: \"%s\" on the following diff. Please answer their question or review the code:\n\n%s", commentBody, diffData)
	response, err := r.Chat(ctx, targetModel, prompt)
	if err != nil {
		logger.Errorf("OpenRouter Chat error: %v", err)
		r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, userFriendlyError(err, "❌ Failed to generate response from OpenRouter."))
		return
	}

	responseBlock := fmt.Sprintf("### 🤖 Automated Response by %s\n\n%s", r.cfg.BotName, response)
	r.PostComment(ctx, ghClient, repoOwner, repoName, prNumber, responseBlock)
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
		MaxTokens: r.cfg.MaxTokens,
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
		err = logThoroughOpenRouterError(ctx, resp, jsonData)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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

// userFriendlyError parses common OpenRouter errors and returns a styled detailed markdown message.
func userFriendlyError(err error, fallback string) string {
	if err == nil {
		return fallback
	}

	errStr := err.Error()

	// 1. Invalid Model / Model Not Found
	if strings.Contains(strings.ToLower(errStr), "is not a valid model id") ||
		strings.Contains(strings.ToLower(errStr), "model not found") ||
		strings.Contains(strings.ToLower(errStr), "model_not_found") {
		return fmt.Sprintf("❌ **OpenRouter Error: Invalid Model**\n\nThe model you requested could not be found or is not supported. Please verify the model ID matches a valid ID on OpenRouter (e.g., `meta-llama/llama-3.1-70b-instruct`).\n\n*Error details:* `%s`", errStr)
	}

	// 2. Rate Limit Exceeded
	if strings.Contains(strings.ToLower(errStr), "rate limit") ||
		strings.Contains(strings.ToLower(errStr), "429") ||
		strings.Contains(strings.ToLower(errStr), "too many requests") {
		return fmt.Sprintf("❌ **OpenRouter Error: Rate Limit Exceeded**\n\nRate limit was hit for the OpenRouter API. Please wait a moment before trying again.\n\n*Error details:* `%s`", errStr)
	}

	// 3. Unauthorized / Invalid API Key
	if strings.Contains(strings.ToLower(errStr), "invalid api key") ||
		strings.Contains(strings.ToLower(errStr), "401") ||
		strings.Contains(strings.ToLower(errStr), "unauthorized") {
		return fmt.Sprintf("❌ **OpenRouter Error: Unauthorized**\n\nThe configured OpenRouter API key is invalid or unauthorized. Please check that the API key is correctly configured.\n\n*Error details:* `%s`", errStr)
	}

	// 4. Billing / Insufficient Credits
	if strings.Contains(strings.ToLower(errStr), "insufficient") ||
		strings.Contains(strings.ToLower(errStr), "credit") ||
		strings.Contains(strings.ToLower(errStr), "billing") ||
		strings.Contains(strings.ToLower(errStr), "limit reached") {
		return fmt.Sprintf("❌ **OpenRouter Error: Insufficient Credits / Billing Limit**\n\nThe request failed due to insufficient credits or billing limits on the configured OpenRouter account. Please check your account balance.\n\n*Error details:* `%s`", errStr)
	}

	// Fallback with a more detailed message instead of a generic one
	return fmt.Sprintf("%s\n\n*Error details:* `%s`", fallback, errStr)
}
