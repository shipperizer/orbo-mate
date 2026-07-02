# 🤖 Orbo-Mate

Orbo-Mate is an automated GitHub Pull Request reviewer bot written in Go. It listens for webhook events from GitHub, fetches PR diffs, and uses AI models via OpenRouter to provide automated code reviews directly on the PR thread.

## 🚀 Features

*   **Webhook Server:** Listens for GitHub Issue Comment events.
*   **Trigger:** Responds to comments like `@ai-bot review with <model>` on PRs.
*   **AI Code Review:** Integrates with OpenRouter to review code diffs using various LLMs (e.g., Llama 3, GPT-4, etc.).
*   **Concurrent Processing:** Uses a goroutine worker pool (max 100 concurrent workers) for handling multiple reviews asynchronously.
*   **Configurable:** Highly customizable via environment variables.

## 🛠️ Prerequisites

*   Go 1.21+
*   A GitHub Webhook configured for your repository (Content type: `application/json`, Event: `Issue comments`).
*   A GitHub Personal Access Token (PAT) with repository read and pull request comment write permissions.
*   An OpenRouter API Key.

## ⚙️ Configuration

The application is configured using environment variables.

| Variable | Description | Required | Default |
| :--- | :--- | :--- | :--- |
| `GITHUB_WEBHOOK_SECRET` | Secret used to sign and verify GitHub webhook payloads. | Yes | - |
| `GITHUB_TOKEN` | GitHub PAT for fetching diffs and posting comments. | Yes | - |
| `OPENROUTER_API_KEY` | Your OpenRouter API key. | Yes | - |
| `DEFAULT_MODEL` | Default AI model to use if none is specified. | No | `meta-llama/llama-3-70b-instruct` |
| `BOT_NAME` | The mention handle that triggers the bot. | No | `@ai-bot` |
| `PORT` | Port for the webhook server to listen on. | No | `8080` |
| `CONTEXT_SENTENCE` | Custom system prompt for the AI reviewer (max 500 chars). | No | *Standard code review prompt* |

## 🏃 Running the Application

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

./orbo-mate server
```

The server will start listening on port `8080` (or your configured `PORT`) at the `/webhook` endpoint.

## 🧪 Testing Locally

You can test the bot's logic without triggering a real GitHub webhook using the built-in `mock-webhook` command.

1.  Ensure your `orbo-mate server` is running.
2.  Open a new terminal window.
3.  Set the `GITHUB_WEBHOOK_SECRET` (it must match the one the server is using).
4.  Run the mock command, providing a real PR URL:

```bash
export GITHUB_WEBHOOK_SECRET="your-secret"

./orbo-mate mock-webhook --pr-url "https://github.com/owner/repo/pull/123"
```

### Mock Webhook Flags

*   `-u`, `--pr-url` (Required): The GitHub PR URL to review.
*   `-m`, `--model`: The AI model to use (default: `meta-llama/llama-3-70b-instruct`).
*   `-s`, `--server-url`: URL of your local server (default: `http://localhost:8080/webhook`).
*   `-c`, `--comment`: Custom comment body (default triggers a review).
