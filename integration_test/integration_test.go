package integration_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func computeHMAC256(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestIntegration_OrboMateContainer(t *testing.T) {
	// Set up environment variables to support rootless Podman out of the box
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	if os.Getenv("DOCKER_HOST") == "" {
		// Set default user-level rootless Podman socket path
		os.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	}

	ctx := context.Background()

	// Spin up the local orbo-mate application using the Dockerfile
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../",
			Dockerfile: "Dockerfile",
		},
		Cmd:          []string{"server"}, // Runs the 'server' cobra command
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"GITHUB_WEBHOOK_SECRET": "secret-key",
			"GITHUB_TOKEN":          "gh-token",
			"OPENROUTER_API_KEY":    "or-key",
			"ALLOWED_ORGS":          "test-org",
			"PORT":                  "8080",
		},
		WaitingFor: wait.ForLog("Server starting on port 8080...").WithStartupTimeout(120 * time.Second),
	}

	orboC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start orbo-mate container: %v", err)
	}

	defer func() {
		if err := orboC.Terminate(ctx); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	endpoint, err := orboC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}

	t.Logf("orbo-mate container successfully running at endpoint: %s", endpoint)

	// Make a POST webhook request with invalid signature to verify the container server is up and routing requests correctly
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post("http://"+endpoint+"/webhook", "application/json", bytes.NewBufferString("{}"))
	if err != nil {
		t.Fatalf("Failed to make POST request to orbo-mate webhook: %v", err)
	}
	defer resp.Body.Close()

	// We expect 401 Unauthorized because we passed no / invalid signature
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status code 401, got %d", resp.StatusCode)
	}
}

func TestIntegration_PullRequestEvent(t *testing.T) {
	// Set up environment variables to support rootless Podman out of the box
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	if os.Getenv("DOCKER_HOST") == "" {
		// Set default user-level rootless Podman socket path
		os.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	}

	ctx := context.Background()

	// Load anonymized pull request payload from testdata file
	payloadBytes, err := os.ReadFile("testdata/pull_request_event.json")
	if err != nil {
		t.Fatalf("Failed to read testdata payload: %v", err)
	}

	// Spin up the local orbo-mate application using the Dockerfile, allowing 'dummy-org' matching our anonymized payload
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../",
			Dockerfile: "Dockerfile",
		},
		Cmd:          []string{"server"}, // Runs the 'server' cobra command
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"GITHUB_WEBHOOK_SECRET": "secret-key",
			"GITHUB_TOKEN":          "gh-token",
			"OPENROUTER_API_KEY":    "or-key",
			"ALLOWED_ORGS":          "dummy-org",
			"PORT":                  "8080",
		},
		WaitingFor: wait.ForLog("Server starting on port 8080...").WithStartupTimeout(120 * time.Second),
	}

	orboC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start orbo-mate container: %v", err)
	}

	defer func() {
		if err := orboC.Terminate(ctx); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	endpoint, err := orboC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}

	t.Logf("orbo-mate container successfully running at endpoint: %s", endpoint)

	// Calculate correct signature for the payload
	signature := computeHMAC256(payloadBytes, "secret-key")

	// Post the valid signed webhook payload representing a Pull Request event to the container
	client := &http.Client{Timeout: 5 * time.Second}
	reqURL := "http://" + endpoint + "/webhook"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-GitHub-Event", "pull_request")
	httpReq.Header.Set("X-Hub-Signature-256", signature)

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to execute request against orbo-mate webhook: %v", err)
	}
	defer resp.Body.Close()

	// Verify that the server returns 200 OK since the signature matches and org 'dummy-org' is allowed
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200 OK, got %d", resp.StatusCode)
	}
}

func TestIntegration_IssueAssignedEvent(t *testing.T) {
	// Set up environment variables to support rootless Podman out of the box
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	if os.Getenv("DOCKER_HOST") == "" {
		// Set default user-level rootless Podman socket path
		os.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	}

	ctx := context.Background()

	// Load anonymized issue assigned payload from testdata file
	payloadBytes, err := os.ReadFile("testdata/issue_assigned_event.json")
	if err != nil {
		t.Fatalf("Failed to read testdata payload: %v", err)
	}

	// Spin up the local orbo-mate application using the Dockerfile, allowing 'dummy-org' matching our anonymized payload
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../",
			Dockerfile: "Dockerfile",
		},
		Cmd:          []string{"server"}, // Runs the 'server' cobra command
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"GITHUB_WEBHOOK_SECRET": "secret-key",
			"GITHUB_TOKEN":          "gh-token",
			"OPENROUTER_API_KEY":    "or-key",
			"ALLOWED_ORGS":          "dummy-org",
			"PORT":                  "8080",
			"BOT_NAME":              "@ai-bot",
		},
		WaitingFor: wait.ForLog("Server starting on port 8080...").WithStartupTimeout(120 * time.Second),
	}

	orboC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start orbo-mate container: %v", err)
	}

	defer func() {
		if err := orboC.Terminate(ctx); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	endpoint, err := orboC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}

	t.Logf("orbo-mate container successfully running at endpoint: %s", endpoint)

	// Calculate correct signature for the payload
	signature := computeHMAC256(payloadBytes, "secret-key")

	// Post the valid signed webhook payload representing an Issues event to the container
	client := &http.Client{Timeout: 5 * time.Second}
	reqURL := "http://" + endpoint + "/webhook"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-GitHub-Event", "issues")
	httpReq.Header.Set("X-Hub-Signature-256", signature)

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to execute request against orbo-mate webhook: %v", err)
	}
	defer resp.Body.Close()

	// Verify that the server returns 200 OK since the signature matches and org 'dummy-org' is allowed
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200 OK, got %d", resp.StatusCode)
	}
}

func TestIntegration_IssueCommentEvent(t *testing.T) {
	// Set up environment variables to support rootless Podman out of the box
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	if os.Getenv("DOCKER_HOST") == "" {
		// Set default user-level rootless Podman socket path
		os.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	}

	ctx := context.Background()

	// Load anonymized issue comment payload from testdata file
	payloadBytes, err := os.ReadFile("testdata/issue_comment_event.json")
	if err != nil {
		t.Fatalf("Failed to read testdata payload: %v", err)
	}

	// Spin up the local orbo-mate application using the Dockerfile, allowing 'dummy-org' matching our anonymized payload
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../",
			Dockerfile: "Dockerfile",
		},
		Cmd:          []string{"server"}, // Runs the 'server' cobra command
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"GITHUB_WEBHOOK_SECRET": "secret-key",
			"GITHUB_TOKEN":          "gh-token",
			"OPENROUTER_API_KEY":    "or-key",
			"ALLOWED_ORGS":          "dummy-org",
			"PORT":                  "8080",
			"BOT_NAME":              "@ai-bot",
		},
		WaitingFor: wait.ForLog("Server starting on port 8080...").WithStartupTimeout(120 * time.Second),
	}

	orboC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start orbo-mate container: %v", err)
	}

	defer func() {
		if err := orboC.Terminate(ctx); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	endpoint, err := orboC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}

	t.Logf("orbo-mate container successfully running at endpoint: %s", endpoint)

	// Calculate correct signature for the payload
	signature := computeHMAC256(payloadBytes, "secret-key")

	// Post the valid signed webhook payload representing an issue comment event to the container
	client := &http.Client{Timeout: 5 * time.Second}
	reqURL := "http://" + endpoint + "/webhook"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-GitHub-Event", "issue_comment")
	httpReq.Header.Set("X-Hub-Signature-256", signature)

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to execute request against orbo-mate webhook: %v", err)
	}
	defer resp.Body.Close()

	// Verify that the server returns 200 OK since the signature matches and org 'dummy-org' is allowed
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200 OK, got %d", resp.StatusCode)
	}
}

func TestIntegration_IssueCommentEvent_FindBetterSolution(t *testing.T) {
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	if os.Getenv("DOCKER_HOST") == "" {
		os.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	}

	ctx := context.Background()

	payloadBytes, err := os.ReadFile("testdata/issue_comment_event.json")
	if err != nil {
		t.Fatalf("Failed to read testdata payload: %v", err)
	}

	// Dynamic override of comment body to match "find a better solution using moonshotai/kimi-k2.6"
	var data map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	commentMap, ok := data["comment"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing comment map in JSON structure")
	}
	commentMap["body"] = "@ai-bot find a better solution using moonshotai/kimi-k2.6"
	newPayloadBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal modified JSON: %v", err)
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../",
			Dockerfile: "Dockerfile",
		},
		Cmd:          []string{"server"},
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"GITHUB_WEBHOOK_SECRET": "secret-key",
			"GITHUB_TOKEN":          "gh-token",
			"OPENROUTER_API_KEY":    "or-key",
			"ALLOWED_ORGS":          "dummy-org",
			"PORT":                  "8080",
			"BOT_NAME":              "@ai-bot",
		},
		WaitingFor: wait.ForLog("Server starting on port 8080...").WithStartupTimeout(120 * time.Second),
	}

	orboC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start orbo-mate container: %v", err)
	}
	defer func() {
		_ = orboC.Terminate(ctx)
	}()

	endpoint, err := orboC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}

	signature := computeHMAC256(newPayloadBytes, "secret-key")

	client := &http.Client{Timeout: 5 * time.Second}
	reqURL := "http://" + endpoint + "/webhook"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewBuffer(newPayloadBytes))
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-GitHub-Event", "issue_comment")
	httpReq.Header.Set("X-Hub-Signature-256", signature)

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200 OK, got %d", resp.StatusCode)
	}
}

func TestIntegration_IssueCommentEvent_LLMFallback(t *testing.T) {
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	if os.Getenv("DOCKER_HOST") == "" {
		os.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	}

	ctx := context.Background()

	payloadBytes, err := os.ReadFile("testdata/issue_comment_event.json")
	if err != nil {
		t.Fatalf("Failed to read testdata payload: %v", err)
	}

	// Dynamic override of comment body to conversational format that triggers the fallback
	var data map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &data); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}
	commentMap, ok := data["comment"].(map[string]interface{})
	if !ok {
		t.Fatal("Missing comment map in JSON structure")
	}
	commentMap["body"] = "@ai-bot please try hard to resolve this issue and let it use model moonshotai/kimi-k2.6"
	newPayloadBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal modified JSON: %v", err)
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../",
			Dockerfile: "Dockerfile",
		},
		Cmd:          []string{"server"},
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"GITHUB_WEBHOOK_SECRET": "secret-key",
			"GITHUB_TOKEN":          "gh-token",
			"OPENROUTER_API_KEY":    "or-key",
			"EXTRACTOR_MODEL":       "google/gemma-3-4b-it",
			"ALLOWED_ORGS":          "dummy-org",
			"PORT":                  "8080",
			"BOT_NAME":              "@ai-bot",
		},
		WaitingFor: wait.ForLog("Server starting on port 8080...").WithStartupTimeout(120 * time.Second),
	}

	orboC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start orbo-mate container: %v", err)
	}
	defer func() {
		_ = orboC.Terminate(ctx)
	}()

	endpoint, err := orboC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}

	signature := computeHMAC256(newPayloadBytes, "secret-key")

	client := &http.Client{Timeout: 5 * time.Second}
	reqURL := "http://" + endpoint + "/webhook"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewBuffer(newPayloadBytes))
	if err != nil {
		t.Fatalf("Failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-GitHub-Event", "issue_comment")
	httpReq.Header.Set("X-Hub-Signature-256", signature)

	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200 OK, got %d", resp.StatusCode)
	}
}


