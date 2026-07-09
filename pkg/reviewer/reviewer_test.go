package reviewer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-github/v60/github"
	"github.com/shipperizer/orbo-mate/pkg/config"
)

type mockRoundTripper func(req *http.Request) (*http.Response, error)

func (f mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestReviewer_ProcessComment_NoMatch(t *testing.T) {
	cfg := &config.Config{
		BotName: "@ai-bot",
	}

	// If no-op, httpClient won't be called, so any call would panic/fail if we don't mock it, but we can verify it doesn't do anything.
	r := NewReviewer(cfg, nil)

	event := &github.IssueCommentEvent{
		Comment: &github.IssueComment{
			Body: github.String("just a normal comment"),
		},
	}

	// Should return early and not panic or make requests
	r.ProcessComment(context.Background(), event)
}

func TestReviewer_ProcessComment_Success(t *testing.T) {
	cfg := &config.Config{
		BotName:         "@ai-bot",
		GitHubToken:     "gh-token",
		OpenRouterKey:   "or-key",
		ContextSentence: "Please review the diff below",
	}

	var calledGetDiff, calledOpenRouter, calledPostComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			// 1. Fetch Diff
			if req.Method == "GET" && req.URL.Path == "/repos/my-owner/my-repo/pulls/123" {
				calledGetDiff = true
				if req.Header.Get("Accept") != "application/vnd.github.v3.diff" {
					t.Errorf("Expected Accept header application/vnd.github.v3.diff, got %q", req.Header.Get("Accept"))
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString("diff --git a/main.go b/main.go")),
				}, nil
			}

			// 2. OpenRouter Chat Completion
			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				calledOpenRouter = true
				if auth := req.Header.Get("Authorization"); auth != "Bearer or-key" {
					t.Errorf("Expected Auth header Bearer or-key, got %q", auth)
				}
				
				// Decode the request body to verify it contains the prompt
				var openReq OpenRouterRequest
				_ = json.NewDecoder(req.Body).Decode(&openReq)
				if openReq.Model != "meta-llama/llama-3" {
					t.Errorf("Expected model meta-llama/llama-3, got %q", openReq.Model)
				}

				respPayload := OpenRouterResponse{
					Choices: []struct {
						Message OpenRouterMessage `json:"message"`
					}{
						{
							Message: OpenRouterMessage{
								Role:    "assistant",
								Content: "Code looks good!",
							},
						},
					},
				}
				respBytes, _ := json.Marshal(respPayload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(respBytes)),
				}, nil
			}

			// 3. Post GitHub Comment
			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/123/comments" {
				calledPostComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "### 🤖 Automated Review by @ai-bot\n*Model used: `meta-llama/llama-3`*\n\nCode looks good!"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}

			t.Errorf("Unexpected request: %s %s", req.Method, req.URL.String())
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(bytes.NewBufferString("")),
			}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.IssueCommentEvent{
		Comment: &github.IssueComment{
			Body: github.String("@ai-bot review with meta-llama/llama-3"),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
		Issue: &github.Issue{
			Number:           github.Int(123),
			PullRequestLinks: &github.PullRequestLinks{},
		},
	}

	r.ProcessComment(context.Background(), event)

	if !calledGetDiff {
		t.Error("Expected FetchPRDiff to be called")
	}
	if !calledOpenRouter {
		t.Error("Expected GetOpenRouterReview to be called")
	}
	if !calledPostComment {
		t.Error("Expected PostComment to be called")
	}
}

func TestReviewer_ProcessComment_FetchDiffError(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
	}

	var calledPostFailureComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" {
				return &http.Response{
					StatusCode: http.StatusInternalServerError,
					Body:       io.NopCloser(bytes.NewBufferString("Internal Error")),
				}, nil
			}
			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/123/comments" {
				calledPostFailureComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				if comment.GetBody() != "❌ Failed to fetch PR diff for review." {
					t.Errorf("Expected failure message, got %q", comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}
			return &http.Response{StatusCode: 400, Body: io.NopCloser(bytes.NewBufferString(""))}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.IssueCommentEvent{
		Comment: &github.IssueComment{
			Body: github.String("@ai-bot review with meta-llama/llama-3"),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
		Issue: &github.Issue{
			Number:           github.Int(123),
			PullRequestLinks: &github.PullRequestLinks{},
		},
	}

	r.ProcessComment(context.Background(), event)

	if !calledPostFailureComment {
		t.Error("Expected failure comment to be posted")
	}
}

func TestReviewer_ProcessComment_OpenRouterError(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
	}

	var calledPostFailureComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString("some diff")),
				}, nil
			}
			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Body:       io.NopCloser(bytes.NewBufferString("Unauthorized key")),
				}, nil
			}
			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/123/comments" {
				calledPostFailureComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "❌ **OpenRouter Error: Unauthorized**\n\nThe configured OpenRouter API key is invalid or unauthorized. Please check that the API key is correctly configured.\n\n*Error details:* `openrouter error status 401: Unauthorized key`"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}
			return &http.Response{StatusCode: 400, Body: io.NopCloser(bytes.NewBufferString(""))}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.IssueCommentEvent{
		Comment: &github.IssueComment{
			Body: github.String("@ai-bot review with meta-llama/llama-3"),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
		Issue: &github.Issue{
			Number:           github.Int(123),
			PullRequestLinks: &github.PullRequestLinks{},
		},
	}

	r.ProcessComment(context.Background(), event)

	if !calledPostFailureComment {
		t.Error("Expected failure comment to be posted")
	}
}

func TestReviewer_Chat_SuccessAndError(t *testing.T) {
	cfg := &config.Config{
		OpenRouterKey: "or-key",
	}

	var mode string // "success" or "error"

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if mode == "success" {
				respPayload := OpenRouterResponse{
					Choices: []struct {
						Message OpenRouterMessage `json:"message"`
					}{
						{
							Message: OpenRouterMessage{
								Role:    "assistant",
								Content: "Hello world!",
							},
						},
					},
				}
				respBytes, _ := json.Marshal(respPayload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(respBytes)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewBufferString("Server Error")),
			}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	// Test 1: Chat Success
	mode = "success"
	resp, err := r.Chat(context.Background(), "my-model", "Hello")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if resp != "Hello world!" {
		t.Errorf("Expected 'Hello world!', got %q", resp)
	}

	// Test 2: Chat Error
	mode = "error"
	_, err = r.Chat(context.Background(), "my-model", "Hello")
	if err == nil {
		t.Fatal("Expected error from HTTP 500, got nil")
	}
}

func TestReviewer_ProcessIssueAssigned(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
		DefaultModel:  "meta-llama/llama-3.1-70b-instruct",
	}

	var calledOpenRouter, calledPostComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				calledOpenRouter = true
				var openReq OpenRouterRequest
				_ = json.NewDecoder(req.Body).Decode(&openReq)
				if openReq.Model != "meta-llama/llama-3.1-70b-instruct" {
					t.Errorf("Expected model %s, got %q", cfg.DefaultModel, openReq.Model)
				}

				respPayload := OpenRouterResponse{
					Choices: []struct {
						Message OpenRouterMessage `json:"message"`
					}{
						{
							Message: OpenRouterMessage{
								Role:    "assistant",
								Content: "Here is how to solve it",
							},
						},
					},
				}
				respBytes, _ := json.Marshal(respPayload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(respBytes)),
				}, nil
			}

			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/42/comments" {
				calledPostComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "### 🤖 Automated Response by @ai-bot\n\nHere is how to solve it"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}

			t.Errorf("Unexpected request: %s %s", req.Method, req.URL.String())
			return &http.Response{StatusCode: 400}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.IssuesEvent{
		Action: github.String("assigned"),
		Issue: &github.Issue{
			Number: github.Int(42),
			Title:  github.String("Bug: crash"),
			Body:   github.String("It crashed on start"),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
	}

	r.ProcessIssueAssigned(context.Background(), event)

	if !calledOpenRouter {
		t.Error("Expected OpenRouter Chat to be called")
	}
	if !calledPostComment {
		t.Error("Expected PostComment to be called")
	}
}

func TestReviewer_ProcessPRAssigned(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
		DefaultModel:  "meta-llama/llama-3.1-70b-instruct",
	}

	var calledGetDiff, calledOpenRouter, calledPostComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" && req.URL.Path == "/repos/my-owner/my-repo/pulls/100" {
				calledGetDiff = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString("diff --git a/main.go b/main.go")),
				}, nil
			}

			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				calledOpenRouter = true
				var openReq OpenRouterRequest
				_ = json.NewDecoder(req.Body).Decode(&openReq)

				respPayload := OpenRouterResponse{
					Choices: []struct {
						Message OpenRouterMessage `json:"message"`
					}{
						{
							Message: OpenRouterMessage{
								Role:    "assistant",
								Content: "PR looks fine",
							},
						},
					},
				}
				respBytes, _ := json.Marshal(respPayload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(respBytes)),
				}, nil
			}

			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/100/comments" {
				calledPostComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "### 🤖 Automated Review by @ai-bot\n*Model used: `meta-llama/llama-3.1-70b-instruct` (Default)*\n\nPR looks fine"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}

			t.Errorf("Unexpected request: %s %s", req.Method, req.URL.String())
			return &http.Response{StatusCode: 400}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.PullRequestEvent{
		Action: github.String("assigned"),
		PullRequest: &github.PullRequest{
			Number: github.Int(100),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
	}

	r.ProcessPRAssigned(context.Background(), event)

	if !calledGetDiff {
		t.Error("Expected FetchPRDiff to be called")
	}
	if !calledOpenRouter {
		t.Error("Expected OpenRouter Review to be called")
	}
	if !calledPostComment {
		t.Error("Expected PostComment to be called")
	}
}

func TestReviewer_ProcessPRReviewComment(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
		DefaultModel:  "meta-llama/llama-3.1-70b-instruct",
	}

	var calledGetDiff, calledOpenRouter, calledPostComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" && req.URL.Path == "/repos/my-owner/my-repo/pulls/200" {
				calledGetDiff = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString("diff --git a/foo b/foo")),
				}, nil
			}

			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				calledOpenRouter = true
				var openReq OpenRouterRequest
				_ = json.NewDecoder(req.Body).Decode(&openReq)
				if openReq.Model != "anthropic/claude-3" {
					t.Errorf("Expected model anthropic/claude-3, got %q", openReq.Model)
				}

				respPayload := OpenRouterResponse{
					Choices: []struct {
						Message OpenRouterMessage `json:"message"`
					}{
						{
							Message: OpenRouterMessage{
								Role:    "assistant",
								Content: "This looks like a comment reply",
							},
						},
					},
				}
				respBytes, _ := json.Marshal(respPayload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(respBytes)),
				}, nil
			}

			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/200/comments" {
				calledPostComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "### 🤖 Automated Response by @ai-bot\n\nThis looks like a comment reply"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}

			t.Errorf("Unexpected request: %s %s", req.Method, req.URL.String())
			return &http.Response{StatusCode: 400}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.PullRequestReviewCommentEvent{
		Action: github.String("created"),
		Comment: &github.PullRequestComment{
			Body: github.String("@ai-bot review with anthropic/claude-3"),
		},
		PullRequest: &github.PullRequest{
			Number: github.Int(200),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
	}

	r.ProcessPRReviewComment(context.Background(), event)

	if !calledGetDiff {
		t.Error("Expected FetchPRDiff to be called")
	}
	if !calledOpenRouter {
		t.Error("Expected OpenRouter Chat to be called")
	}
	if !calledPostComment {
		t.Error("Expected PostComment to be called")
	}
}


func TestReviewer_ProcessComment_Solve(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
		DefaultModel:  "meta-llama/llama-3.1-70b-instruct",
	}

	var calledOpenRouter, calledPostComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				calledOpenRouter = true
				var openReq OpenRouterRequest
				_ = json.NewDecoder(req.Body).Decode(&openReq)
				if openReq.Model != "z.ai/glm-5.2" {
					t.Errorf("Expected model z.ai/glm-5.2, got %q", openReq.Model)
				}

				respPayload := OpenRouterResponse{
					Choices: []struct {
						Message OpenRouterMessage `json:"message"`
					}{
						{
							Message: OpenRouterMessage{
								Role:    "assistant",
								Content: "Here is the solution!",
							},
						},
					},
				}
				respBytes, _ := json.Marshal(respPayload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(respBytes)),
				}, nil
			}

			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/456/comments" {
				calledPostComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "### 🤖 Automated Response by @ai-bot\n\nHere is the solution!"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}

			t.Errorf("Unexpected request: %s %s", req.Method, req.URL.String())
			return &http.Response{StatusCode: 400}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.IssueCommentEvent{
		Comment: &github.IssueComment{
			Body: github.String("@ai-bot try to solve this with model z.ai/glm-5.2"),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
		Issue: &github.Issue{
			Number: github.Int(456),
			Title:  github.String("Centralize test infrastructure"),
			Body:   github.String("Copy pasted setup helpers are bad."),
		},
	}

	r.ProcessComment(context.Background(), event)

	if !calledOpenRouter {
		t.Error("Expected OpenRouter Chat to be called")
	}
	if !calledPostComment {
		t.Error("Expected PostComment to be called")
	}
}

func TestReviewer_ProcessComment_Solve_PR(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
		DefaultModel:  "meta-llama/llama-3.1-70b-instruct",
	}

	var calledGetDiff, calledOpenRouter, calledPostComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "GET" && req.URL.Path == "/repos/my-owner/my-repo/pulls/789" {
				calledGetDiff = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString("diff --git a/foo b/foo")),
				}, nil
			}

			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				calledOpenRouter = true
				var openReq OpenRouterRequest
				_ = json.NewDecoder(req.Body).Decode(&openReq)
				if openReq.Model != "z.ai/glm-5.2" {
					t.Errorf("Expected model z.ai/glm-5.2, got %q", openReq.Model)
				}

				respPayload := OpenRouterResponse{
					Choices: []struct {
						Message OpenRouterMessage `json:"message"`
					}{
						{
							Message: OpenRouterMessage{
								Role:    "assistant",
								Content: "Here is the PR solution!",
							},
						},
					},
				}
				respBytes, _ := json.Marshal(respPayload)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBuffer(respBytes)),
				}, nil
			}

			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/789/comments" {
				calledPostComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "### 🤖 Automated Response by @ai-bot\n\nHere is the PR solution!"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}

			t.Errorf("Unexpected request: %s %s", req.Method, req.URL.String())
			return &http.Response{StatusCode: 400}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.IssueCommentEvent{
		Comment: &github.IssueComment{
			Body: github.String("@ai-bot try to solve this with model z.ai/glm-5.2"),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
		Issue: &github.Issue{
			Number:           github.Int(789),
			Title:            github.String("Implement changes"),
			Body:             github.String("Implement changes requested in issues."),
			PullRequestLinks: &github.PullRequestLinks{},
		},
	}

	r.ProcessComment(context.Background(), event)

	if !calledGetDiff {
		t.Error("Expected FetchPRDiff to be called")
	}
	if !calledOpenRouter {
		t.Error("Expected OpenRouter Chat to be called")
	}
	if !calledPostComment {
		t.Error("Expected PostComment to be called")
	}
}


func TestReviewer_ProcessComment_InvalidModelError(t *testing.T) {
	cfg := &config.Config{
		BotName:       "@ai-bot",
		GitHubToken:   "gh-token",
		OpenRouterKey: "or-key",
		DefaultModel:  "meta-llama/llama-3.1-70b-instruct",
	}

	var calledOpenRouter, calledPostComment bool

	client := &http.Client{
		Transport: mockRoundTripper(func(req *http.Request) (*http.Response, error) {
			if req.Method == "POST" && req.URL.Host == "openrouter.ai" {
				calledOpenRouter = true
				errorJSON := `{"error":{"message":"z.ai/glm-5 is not a valid model ID","code":400}}`
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewBufferString(errorJSON)),
					Request:    req,
				}, nil
			}

			if req.Method == "POST" && req.URL.Path == "/repos/my-owner/my-repo/issues/456/comments" {
				calledPostComment = true
				var comment github.IssueComment
				_ = json.NewDecoder(req.Body).Decode(&comment)
				expectedBody := "❌ **OpenRouter Error: Invalid Model**\n\nThe model you requested could not be found or is not supported. Please verify the model ID matches a valid ID on OpenRouter (e.g., `meta-llama/llama-3.1-70b-instruct`).\n\n*Error details:* `openrouter error status 400 (code 400): z.ai/glm-5 is not a valid model ID`"
				if comment.GetBody() != expectedBody {
					t.Errorf("Expected body %q, got %q", expectedBody, comment.GetBody())
				}
				return &http.Response{
					StatusCode: http.StatusCreated,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
				}, nil
			}

			t.Errorf("Unexpected request: %s %s", req.Method, req.URL.String())
			return &http.Response{StatusCode: 400}, nil
		}),
	}

	r := NewReviewer(cfg, client)

	event := &github.IssueCommentEvent{
		Comment: &github.IssueComment{
			Body: github.String("@ai-bot try to solve this with model z.ai/glm-5"),
		},
		Repo: &github.Repository{
			Name: github.String("my-repo"),
			Owner: &github.User{
				Login: github.String("my-owner"),
			},
		},
		Issue: &github.Issue{
			Number: github.Int(456),
			Title:  github.String("Centralize test infrastructure"),
			Body:   github.String("Copy pasted setup helpers are bad."),
		},
	}

	r.ProcessComment(context.Background(), event)

	if !calledOpenRouter {
		t.Error("Expected OpenRouter Chat to be called")
	}
	if !calledPostComment {
		t.Error("Expected PostComment to be called")
	}
}

func TestReviewer_ThoroughErrorLogging(t *testing.T) {

	// Craft a mock request to get headers redacted
	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer secret-or-key")
	req.Header.Set("Content-Type", "application/json")

	// Craft mock response
	errorJSON := `{"error":{"message":"Model not found","code":404,"metadata":{"id":"model_xyz"}}}`
	resp := &http.Response{
		Status:     "404 Not Found",
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(errorJSON)),
		Request:    req,
	}
	resp.Header.Set("X-RateLimit-Limit", "100")

	err := logThoroughOpenRouterError(context.Background(), resp, []byte(`{"model": "xyz"}`))
	if err == nil {
		t.Fatal("Expected logThoroughOpenRouterError to return an error, got nil")
	}

	expectedErrorStr := "openrouter error status 404 (code 404): Model not found"
	if err.Error() != expectedErrorStr {
		t.Errorf("Expected error string %q, got %q", expectedErrorStr, err.Error())
	}
}

