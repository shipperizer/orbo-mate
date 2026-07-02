#!/bin/bash

# Ensure we're not committing the built binary
echo "orbo-mate" >> .gitignore
git add .gitignore
git commit -m "chore: add binary to gitignore"

# 1. CLI Entrypoint
git add main.go cmd/root.go cmd/server.go
git commit -m "feat(cli): introduce cobra-cli for application entrypoint"

# 2. Config
git add pkg/config/
git commit -m "feat(config): integrate envconfig for environment variable management"

# 3. Worker Pool
git add pkg/pool/
git commit -m "feat(pool): implement goroutine worker pool for concurrent processing"

# 4. Reviewer Package
git add pkg/reviewer/
git commit -m "feat(reviewer): add OpenRouter integration and PR diff fetching"

# 5. Server Package
git add pkg/server/
git commit -m "feat(server): setup webhook routing with go-chi"

# 6. Mock Webhook Command
git add cmd/mock_webhook.go
git commit -m "feat(cli): add mock-webhook command for local testing"

# 7. List Models Command
git add cmd/list_models.go
git commit -m "feat(cli): add list-models command with OpenRouter integration"

# 8. OpenAPI Spec
git add api/
git commit -m "docs(api): add OpenAPI specification for webhook endpoint"

# 9. Integration Tests
git add integration_test/
git commit -m "test(integration): add testcontainers setup for podman"

# 10. Dependencies
git add go.mod go.sum
git commit -m "chore(deps): update module dependencies"

# 11. Readme
git add README.md
git commit -m "docs: prepare readme with running instructions"

# 12. Skaffold and Kustomize
git add k8s/ Dockerfile structure-tests.yaml skaffold.yaml
git commit -m "feat(deploy): add skaffold setup with kustomize and structure-tests"
