package cmd

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"

	"github.com/google/go-github/v60/github"
	"github.com/shipperizer/orbo-mate/pkg/config"
	"github.com/spf13/cobra"
)

var (
	prURL     string
	modelName string
	serverURL string
	customMsg string
)

var mockWebhookCmd = &cobra.Command{
	Use:   "mock-webhook",
	Short: "Mimic a GitHub webhook request",
	Long:  `Craft and send a signed GitHub webhook payload to your local server to test the review logic.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load config to access WebhookSecret and BotName
		cfg, err := config.Load()
		if err != nil {
			// Fallback to manual loading of required signature fields if full server config is absent
			secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
			if secret == "" {
				return fmt.Errorf("GITHUB_WEBHOOK_SECRET is required to sign the webhook payload")
			}
			cfg = &config.Config{
				WebhookSecret: secret,
				BotName:       os.Getenv("BOT_NAME"),
			}
			if cfg.BotName == "" {
				cfg.BotName = "@ai-bot"
			}
		}

		owner, repo, prNum, err := parsePRURL(prURL)
		if err != nil {
			return err
		}

		commentBody := customMsg
		if commentBody == "" {
			commentBody = fmt.Sprintf("%s review with %s", cfg.BotName, modelName)
		}

		// Create mock issue comment event payload
		event := github.IssueCommentEvent{
			Action: github.String("created"),
			Issue: &github.Issue{
				Number:           github.Int(prNum),
				PullRequestLinks: &github.PullRequestLinks{}, // ensures IsPullRequest() returns true
			},
			Comment: &github.IssueComment{
				Body: github.String(commentBody),
			},
			Repo: &github.Repository{
				Name: github.String(repo),
				Owner: &github.User{
					Login: github.String(owner),
				},
			},
		}

		payloadBytes, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		// Calculate HMAC signature
		sig := computeHMAC256(payloadBytes, cfg.WebhookSecret)

		req, err := http.NewRequest("POST", serverURL, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GitHub-Event", "issue_comment")
		req.Header.Set("X-Hub-Signature-256", sig)

		fmt.Printf("Sending mock webhook event to %s...\n", serverURL)
		fmt.Printf("PR URL: %s (Owner: %s, Repo: %s, PR: #%d)\n", prURL, owner, repo, prNum)
		fmt.Printf("Comment body: %q\n", commentBody)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		fmt.Printf("Server Response Status: %s\n", resp.Status)
		if len(respBody) > 0 {
			fmt.Printf("Server Response Body: %s\n", string(respBody))
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("server returned non-OK status: %s", resp.Status)
		}

		fmt.Println("Mock webhook triggered successfully! Check server logs for review details.")
		return nil
	},
}

func parsePRURL(urlStr string) (owner string, repo string, prNumber int, err error) {
	re := regexp.MustCompile(`https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) < 4 {
		return "", "", 0, fmt.Errorf("invalid GitHub PR URL format: must match https://github.com/owner/repo/pull/number")
	}
	num, err := strconv.Atoi(matches[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to parse PR number: %w", err)
	}
	return matches[1], matches[2], num, nil
}

func computeHMAC256(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func init() {
	mockWebhookCmd.Flags().StringVarP(&prURL, "pr-url", "u", "", "The GitHub PR URL (required)")
	mockWebhookCmd.Flags().StringVarP(&modelName, "model", "m", "meta-llama/llama-3-70b-instruct", "The target review model")
	mockWebhookCmd.Flags().StringVarP(&serverURL, "server-url", "s", "http://localhost:8080/webhook", "The local server webhook URL")
	mockWebhookCmd.Flags().StringVarP(&customMsg, "comment", "c", "", "Use a custom comment body instead of the default review command")

	mockWebhookCmd.MarkFlagRequired("pr-url")
	rootCmd.AddCommand(mockWebhookCmd)
}
