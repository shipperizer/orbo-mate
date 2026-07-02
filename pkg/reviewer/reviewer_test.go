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
			Number: github.Int(123),
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
			Number: github.Int(123),
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
				if comment.GetBody() != "❌ Failed to generate review from OpenRouter." {
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
			Number: github.Int(123),
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
