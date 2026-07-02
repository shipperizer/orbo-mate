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

	"github.com/google/go-github/v60/github"
	"github.com/shipperizer/orbo-mate/pkg/config"
	"github.com/shipperizer/orbo-mate/pkg/pool"
	"github.com/shipperizer/orbo-mate/pkg/reviewer"
)

func computeHMAC256(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestServer_WebhookSignatureValidation(t *testing.T) {
	cfg := &config.Config{
		WebhookSecret: "secret-key",
		GitHubToken:   "token",
		OpenRouterKey: "key",
		BotName:       "@ai-bot",
	}

	p := pool.NewPool(2)
	p.Start()
	defer p.Stop()

	rev := reviewer.NewReviewer(cfg, nil)
	srv := NewServer(cfg, p, rev)

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

	// Test 2: Valid Signature
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
