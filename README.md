# 🤖 Orbo-Mate

Orbo-Mate is an automated GitHub Pull Request reviewer bot written in Go. It listens for webhook events from GitHub, fetches PR diffs, and uses AI models via OpenRouter to provide automated code reviews directly on the PR thread.

## 🚀 Features

*   **Webhook Server:** Handles multi-event webhook dispatching for:
    *   **Issue Comments:** Responds to comments like `@ai-bot review with <model>` on PRs or issue queries on standard threads.
    *   **Issue Assignment:** Auto-recommends solutions when the bot is assigned to an Issue.
    *   **PR Assignment:** Runs an automated code review on PRs when the bot is assigned to a PR.
    *   **PR Review Comments:** Responds to review-specific thread comments mentioning the bot.
*   **JSON-Only Payload Enforcement:** Enforces JSON content-types (`application/json`) and strict schema validations (`json.Valid`) to keep the server secure and robust.
*   **Thorough API Diagnostics:** Intercepts and parses OpenRouter structured errors, printing thorough diagnostic reports while redacting authorization tokens automatically.
*   **AI Code Review:** Integrates with OpenRouter to review code diffs using various LLMs (e.g., Llama 3, GPT-4, Claude 3, etc.).
*   **Concurrent Processing:** Uses a goroutine worker pool (max 100 concurrent workers) for handling multiple reviews asynchronously.
*   **Security Restrictions:** Limits triggers to allowed organizations and prevents cross-organization attacks by validating comment origins.
*   **Configurable:** Highly customizable via environment variables, including runtime log level configurations.

## 🛠️ Prerequisites

*   Go 1.22+ (preferably 1.26+)
*   A GitHub Webhook configured for your repository (Content type: `application/json`, Events: `Issue comments`, `Issues`, `Pull requests`, `Pull request reviews`, `Pull request review comments`).
*   A GitHub Personal Access Token (PAT) with repository read and pull request comment write permissions.
*   An OpenRouter API Key.

## ⚙️ Configuration

The application is configured using environment variables.

| Variable | Description | Required | Default |
| :--- | :--- | :--- | :--- |
| `GITHUB_WEBHOOK_SECRET` | Secret used to sign and verify GitHub webhook payloads. | Yes | - |
| `GITHUB_TOKEN` | GitHub PAT for fetching diffs and posting comments. | Yes | - |
| `OPENROUTER_API_KEY` | Your OpenRouter API key. | Yes | - |
| `ALLOWED_ORGS` | Comma-separated list of GitHub organizations allowed to trigger the bot (e.g., `my-org,another-org`). Prevents unauthorized or cross-org webhook execution. | **Yes** | - |
| `DEFAULT_MODEL` | Default AI model to use if none is specified. | No | `meta-llama/llama-3.1-70b-instruct` |
| `BOT_NAME` | The mention handle that triggers the bot. | No | `@ai-bot` |
| `PORT` | Port for the webhook server to listen on. | No | `8080` |
| `CONTEXT_SENTENCE` | Custom system prompt for the AI reviewer (max 500 chars). | No | *Standard code review prompt* |
| `LOG_LEVEL` | Dynamic log level configuration (`debug`, `info`, `warn`, `error`). | No | `info` |

## 🏃 Running the Application

### Using the Makefile

We include a robust `Makefile` for standard Go development workflows:

```bash
# Build the application for your host platform
make build

# Build the application for linux/arm64 architecture
make build-arm64

# Build for any custom platform via environment variables
GOOS=darwin GOARCH=arm64 make build

# Run unit tests and integration tests with the race detector
make test

# Run tests in short mode
make test-short

# Vet the source code
make vet

# Format the codebase
make fmt
```

### 1. Build the CLI

```bash
go build -o orbo-mate main.go
```

### 2. Start the Server

Set the required environment variables and start the server:

```bash
export GITHUB_WEBHOOK_SECRET="your-secret"
export GITHUB_TOKEN="ghp_your-token"
export OPENROUTER_API_KEY="sk-or-v1-your-key"
export ALLOWED_ORGS="your-github-org"

./orbo-mate server
```

The server will start listening on port `8080` (or your configured `PORT`) at the `/webhook` endpoint.

## 📋 Additional CLI Commands

### List Recommended Models

To consult OpenRouter for the top 10 best value-for-money AI models categorized under programming:

```bash
./orbo-mate list-models
```


---

## 🧪 Testing Locally

### Mock Webhook Trigger

You can test the bot's end-to-end routing and processing logic locally without setting up an actual GitHub organization webhook by using the `mock-webhook` command:

1. Ensure your `orbo-mate server` is running locally.
2. In a new terminal, export matching webhook secrets:

```bash
export GITHUB_WEBHOOK_SECRET="your-secret"

# Mimics an issue comment trigger on a pull request
./orbo-mate mock-webhook --pr-url "https://github.com/your-github-org/your-repo/pull/123"
```

#### Mock Webhook Flags

*   `-u`, `--pr-url` (Required): The GitHub Pull Request URL. Must match an organization listed in `ALLOWED_ORGS`.
*   `-m`, `--model`: The AI model to use (default: `meta-llama/llama-3.1-70b-instruct`).
*   `-s`, `--server-url`: URL of your local running server (default: `http://localhost:8080/webhook`).
*   `-c`, `--comment`: Custom comment body (default: `@ai-bot review with <model>`).

---

## 🏗️ Generating Mocks for Testing

This project uses `go.uber.org/mock/mockgen` for dependency mocking. Unit test mock files are **excluded** from version control. If you add or modify interface signatures, regenerate them on-the-fly:

```bash
go generate ./...
```
