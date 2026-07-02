package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-github/v60/github"
	"github.com/shipperizer/orbo-mate/pkg/config"
	"github.com/shipperizer/orbo-mate/pkg/pool"
	"github.com/shipperizer/orbo-mate/pkg/server/mocks"
	"go.uber.org/mock/gomock"
)

func computeHMAC256(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestServer_WebhookSignatureValidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReviewer := mocks.NewMockCommentProcessor(ctrl)

	cfg := &config.Config{
		WebhookSecret: "secret-key",
		GitHubToken:   "token",
		OpenRouterKey: "key",
		BotName:       "@ai-bot",
		AllowedOrgs:   []string{"my-org"},
	}

	p := pool.NewPool(2)
	p.Start()
	defer p.Stop()

	srv := NewServer(cfg, p, mockReviewer)

	bodyBytes, _ := json.Marshal(github.IssueCommentEvent{
		Action: github.String("created"),
	})

	// Test 1: Invalid Signature
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", "invalid")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rr.Code)
	}

	// Test 2: Valid Signature but not matched org (should ignore and return 200)
	sig := computeHMAC256(bodyBytes, "secret-key")
	req, _ = http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", sig)

	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}
}

func TestServer_WebhookAllowedOrgsAndCrossOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReviewer := mocks.NewMockCommentProcessor(ctrl)

	cfg := &config.Config{
		WebhookSecret: "secret-key",
		GitHubToken:   "token",
		OpenRouterKey: "key",
		BotName:       "@ai-bot",
		AllowedOrgs:   []string{"my-org"},
	}

	p := pool.NewPool(2)
	p.Start()
	defer p.Stop()

	srv := NewServer(cfg, p, mockReviewer)

	// Test Case 1: Unauthorized Org
	eventUnauth := github.IssueCommentEvent{
		Action: github.String("created"),
		Issue: &github.Issue{
			Number:           github.Int(42),
			PullRequestLinks: &github.PullRequestLinks{},
		},
		Repo: &github.Repository{
			Owner: &github.User{
				Login: github.String("unauth-org"),
			},
		},
	}
	bodyUnauth, _ := json.Marshal(eventUnauth)
	sigUnauth := computeHMAC256(bodyUnauth, "secret-key")
	reqUnauth, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyUnauth))
	reqUnauth.Header.Set("Content-Type", "application/json")
	reqUnauth.Header.Set("X-GitHub-Event", "issue_comment")
	reqUnauth.Header.Set("X-Hub-Signature-256", sigUnauth)

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, reqUnauth)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Test Case 2: Authorized Org, but Cross-Org/Cross-Repo request (Issue Repository URL != Event Repository URL)
	eventCrossOrg := github.IssueCommentEvent{
		Action: github.String("created"),
		Issue: &github.Issue{
			Number:           github.Int(42),
			PullRequestLinks: &github.PullRequestLinks{},
			RepositoryURL:    github.String("https://api.github.com/repos/my-org/another-repo"),
		},
		Repo: &github.Repository{
			URL: github.String("https://api.github.com/repos/my-org/my-repo"),
			Owner: &github.User{
				Login: github.String("my-org"),
			},
		},
	}
	bodyCross, _ := json.Marshal(eventCrossOrg)
	sigCross := computeHMAC256(bodyCross, "secret-key")
	reqCross, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyCross))
	reqCross.Header.Set("Content-Type", "application/json")
	reqCross.Header.Set("X-GitHub-Event", "issue_comment")
	reqCross.Header.Set("X-Hub-Signature-256", sigCross)

	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, reqCross)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Test Case 3: Valid Authorized Org, matching repository URLs, triggers Reviewer ProcessComment
	eventValid := github.IssueCommentEvent{
		Action: github.String("created"),
		Issue: &github.Issue{
			Number:           github.Int(42),
			PullRequestLinks: &github.PullRequestLinks{},
			RepositoryURL:    github.String("https://api.github.com/repos/my-org/my-repo"),
		},
		Repo: &github.Repository{
			URL: github.String("https://api.github.com/repos/my-org/my-repo"),
			Owner: &github.User{
				Login: github.String("my-org"),
			},
		},
	}
	bodyValid, _ := json.Marshal(eventValid)
	sigValid := computeHMAC256(bodyValid, "secret-key")
	reqValid, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(bodyValid))
	reqValid.Header.Set("Content-Type", "application/json")
	reqValid.Header.Set("X-GitHub-Event", "issue_comment")
	reqValid.Header.Set("X-Hub-Signature-256", sigValid)

	// We expect ProcessComment to be called on our mock reviewer exactly once.
	mockReviewer.EXPECT().ProcessComment(gomock.Any(), gomock.Any()).Times(1)

	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, reqValid)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Wait a tiny bit for the worker pool goroutine to run
	time.Sleep(100 * time.Millisecond)
}

func TestServer_VersionAndHealthz(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReviewer := mocks.NewMockCommentProcessor(ctrl)

	cfg := &config.Config{
		WebhookSecret: "secret-key",
		GitHubToken:   "token",
		OpenRouterKey: "key",
		BotName:       "@ai-bot",
		AllowedOrgs:   []string{"my-org"},
	}

	p := pool.NewPool(2)
	p.Start()
	defer p.Stop()

	srv := NewServer(cfg, p, mockReviewer)

	// Test /version
	req, _ := http.NewRequest("GET", "/version", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var versionResp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&versionResp); err != nil {
		t.Fatalf("Failed to decode version response: %v", err)
	}
	if versionResp["version"] != "0.1.0" {
		t.Errorf("Expected version '0.1.0', got '%s'", versionResp["version"])
	}

	// Test /healthz
	req, _ = http.NewRequest("GET", "/healthz", nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var healthzResp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&healthzResp); err != nil {
		t.Fatalf("Failed to decode healthz response: %v", err)
	}
	if healthzResp["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", healthzResp["status"])
	}
}
