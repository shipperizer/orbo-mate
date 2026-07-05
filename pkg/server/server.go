package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/go-github/v60/github"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shipperizer/orbo-mate/pkg/config"
	"github.com/shipperizer/orbo-mate/pkg/logger"
	"github.com/shipperizer/orbo-mate/pkg/pool"
	"github.com/shipperizer/orbo-mate/pkg/telemetry"
	"github.com/shipperizer/orbo-mate/pkg/version"
)

// CommentProcessor defines the interface for processing webhook comments.
//
//go:generate mockgen -source=server.go -destination=mocks/mock_reviewer.go -package=mocks
type CommentProcessor interface {
	ProcessComment(ctx context.Context, event *github.IssueCommentEvent)
	ProcessIssueAssigned(ctx context.Context, event *github.IssuesEvent)
	ProcessPRAssigned(ctx context.Context, event *github.PullRequestEvent)
	ProcessPRReviewComment(ctx context.Context, event *github.PullRequestReviewCommentEvent)
}

// Server sets up the routes and dependencies for the webhook server.
type Server struct {
	cfg      *config.Config
	pool     *pool.Pool
	reviewer CommentProcessor
	router   *chi.Mux
}

// NewServer returns a new configured webhook Server.
func NewServer(cfg *config.Config, p *pool.Pool, rev CommentProcessor) *Server {
	s := &Server{
		cfg:      cfg,
		pool:     p,
		reviewer: rev,
		router:   chi.NewRouter(),
	}
	s.setupRoutes()
	return s
}

// setupRoutes configures the go-chi router middlewares and handlers.
func (s *Server) setupRoutes() {
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(telemetry.TracingMiddleware)
	s.router.Use(telemetry.PrometheusMiddleware)
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)

	s.router.Post("/webhook", s.handleWebhook)
	s.router.Get("/version", s.handleVersion)
	s.router.Get("/healthz", s.handleHealthz)
	s.router.Handle("/metrics", promhttp.Handler())
}

// ServeHTTP delegates the HTTP requests to the chi router.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) isOrgAllowed(org string) bool {
	for _, allowedOrg := range s.cfg.AllowedOrgs {
		if org == allowedOrg {
			return true
		}
	}
	return false
}

func cleanBotName(botName string) string {
	return strings.TrimPrefix(botName, "@")
}

// handleWebhook handles incoming GitHub webhook requests.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// 1. Enforce JSON Payload
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" && !strings.HasPrefix(contentType, "application/json;") {
		logger.Errorf("Invalid Content-Type: %s", contentType)
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}

	payload, err := github.ValidatePayload(r, []byte(s.cfg.WebhookSecret))
	if err != nil {
		logger.Errorf("Webhook signature verification failed: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Validate payload is valid JSON
	if !json.Valid(payload) {
		logger.Error("Webhook payload is not valid JSON")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		logger.Errorf("Could not parse webhook: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	switch e := event.(type) {
	case *github.IssueCommentEvent:
		if e.GetAction() == "created" && e.GetIssue() != nil {
			body := e.GetComment().GetBody()
			isTagged := strings.Contains(body, s.cfg.BotName) || strings.Contains(body, cleanBotName(s.cfg.BotName))
			if isTagged {
				org := e.GetRepo().GetOwner().GetLogin()
				if !s.isOrgAllowed(org) {
					logger.Warnf("Ignored comment from unauthorized org: %s", org)
					w.WriteHeader(http.StatusOK)
					return
				}

				if e.GetIssue().GetRepositoryURL() != e.GetRepo().GetURL() {
					logger.Warnf("Cross-org request detected. Issue Repo: %s, Event Repo: %s", e.GetIssue().GetRepositoryURL(), e.GetRepo().GetURL())
					w.WriteHeader(http.StatusOK)
					return
				}

				s.pool.Submit(func(ctx context.Context) {
					s.reviewer.ProcessComment(ctx, e)
				})
				telemetry.WebhooksProcessedTotal.WithLabelValues("success").Inc()
			}
		}

	case *github.IssuesEvent:
		if e.GetAction() == "assigned" && e.GetAssignee().GetLogin() == cleanBotName(s.cfg.BotName) && e.GetIssue() != nil {
			org := e.GetRepo().GetOwner().GetLogin()
			if !s.isOrgAllowed(org) {
				logger.Warnf("Ignored issue assignment from unauthorized org: %s", org)
				w.WriteHeader(http.StatusOK)
				return
			}

			s.pool.Submit(func(ctx context.Context) {
				s.reviewer.ProcessIssueAssigned(ctx, e)
			})
			telemetry.WebhooksProcessedTotal.WithLabelValues("success").Inc()
		}

	case *github.PullRequestEvent:
		if e.GetAction() == "assigned" && e.GetAssignee().GetLogin() == cleanBotName(s.cfg.BotName) && e.GetPullRequest() != nil {
			org := e.GetRepo().GetOwner().GetLogin()
			if !s.isOrgAllowed(org) {
				logger.Warnf("Ignored pull request assignment from unauthorized org: %s", org)
				w.WriteHeader(http.StatusOK)
				return
			}

			s.pool.Submit(func(ctx context.Context) {
				s.reviewer.ProcessPRAssigned(ctx, e)
			})
			telemetry.WebhooksProcessedTotal.WithLabelValues("success").Inc()
		}

	case *github.PullRequestReviewCommentEvent:
		if e.GetAction() == "created" && e.GetPullRequest() != nil {
			body := e.GetComment().GetBody()
			isTagged := strings.Contains(body, s.cfg.BotName) || strings.Contains(body, cleanBotName(s.cfg.BotName))
			if isTagged {
				org := e.GetRepo().GetOwner().GetLogin()
				if !s.isOrgAllowed(org) {
					logger.Warnf("Ignored PR review comment from unauthorized org: %s", org)
					w.WriteHeader(http.StatusOK)
					return
				}

				s.pool.Submit(func(ctx context.Context) {
					s.reviewer.ProcessPRReviewComment(ctx, e)
				})
				telemetry.WebhooksProcessedTotal.WithLabelValues("success").Inc()
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"version": version.Version})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
