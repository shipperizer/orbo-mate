package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var lang string

type OpenRouterModelsResponse struct {
	Data []Model `json:"data"`
}

type Model struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	ContextLength int          `json:"context_length"`
	Pricing       ModelPricing `json:"pricing"`
}

type ModelPricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

var listModelsCmd = &cobra.Command{
	Use:   "list-models",
	Short: "List top 10 best value for money models on OpenRouter for reviewing code",
	Run: func(cmd *cobra.Command, args []string) {
		if lang != "python" && lang != "typescript" && lang != "golang" {
			log.Fatalf("Language must be one of: python, typescript, golang")
		}

		fmt.Printf("Fetching available models from OpenRouter for %s...\n", lang)

		ctx := context.Background()
		req, err := http.NewRequestWithContext(ctx, "GET", "https://openrouter.ai/api/v1/models", nil)
		if err != nil {
			log.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatalf("Failed to retrieve models: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("OpenRouter models API returned status: %s", resp.Status)
		}

		var openRouterResp OpenRouterModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&openRouterResp); err != nil {
			log.Fatalf("Failed to decode JSON response: %v", err)
		}

		var filteredModels []Model
		for _, m := range openRouterResp.Data {
			if m.ContextLength < 16384 {
				continue
			}

			pPrompt := parsePrice(m.Pricing.Prompt)
			pCompletion := parsePrice(m.Pricing.Completion)
			if pPrompt < 0 || pCompletion < 0 {
				continue
			}

			if isCodingModel(m.ID, m.Name, m.Description, lang) {
				filteredModels = append(filteredModels, m)
			}
		}

		if len(filteredModels) == 0 {
			fmt.Println("No matching models found on OpenRouter.")
			return
		}

		// Sort filtered models:
		// 1. Prompt price ascending (cheaper first)
		// 2. Completion price ascending
		// 3. Context window descending (larger first)
		sort.Slice(filteredModels, func(i, j int) bool {
			pI := parsePrice(filteredModels[i].Pricing.Prompt)
			pJ := parsePrice(filteredModels[j].Pricing.Prompt)

			if pI != pJ {
				return pI < pJ
			}

			cI := parsePrice(filteredModels[i].Pricing.Completion)
			cJ := parsePrice(filteredModels[j].Pricing.Completion)
			if cI != cJ {
				return cI < cJ
			}

			return filteredModels[i].ContextLength > filteredModels[j].ContextLength
		})

		// Get top 10
		limit := 10
		if len(filteredModels) < limit {
			limit = len(filteredModels)
		}
		topModels := filteredModels[:limit]

		fmt.Println("\nTop 10 Best Value-for-Money Models on OpenRouter:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "MODEL ID\tNAME\tCONTEXT\tPROMPT/1M\tCOMPLETION/1M")
		fmt.Fprintln(w, "--------\t----\t-------\t---------\t-------------")

		for _, m := range topModels {
			promptFormatted := formatPrice(m.Pricing.Prompt)
			completionFormatted := formatPrice(m.Pricing.Completion)
			ctxFormatted := formatContext(m.ContextLength)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", m.ID, m.Name, ctxFormatted, promptFormatted, completionFormatted)
		}
		w.Flush()
	},
}

func parsePrice(priceStr string) float64 {
	if priceStr == "" {
		return 0
	}
	p, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return -1
	}
	return p
}

func formatPrice(priceStr string) string {
	if priceStr == "" || priceStr == "0" {
		return "Free"
	}
	p, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return "N/A"
	}
	if p == 0 {
		return "Free"
	}
	return fmt.Sprintf("$%.4f", p*1000000)
}

func formatContext(ctx int) string {
	if ctx >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(ctx)/1000000.0)
	}
	if ctx >= 1000 {
		return fmt.Sprintf("%dk", ctx/1000)
	}
	return fmt.Sprintf("%d", ctx)
}

func isCodingModel(id, name, desc string, lang string) bool {
	idLower := strings.ToLower(id)
	nameLower := strings.ToLower(name)
	descLower := strings.ToLower(desc)

	// Exclude specialized models
	excluded := []string{"safety", "guard", "moderation", "image", "vision", "audio", "voice", "embed", "embedding", "clip", "whisper", "tts"}
	for _, k := range excluded {
		if strings.Contains(idLower, k) || strings.Contains(nameLower, k) || strings.Contains(descLower, k) {
			return false
		}
	}

	// Include models matching keywords
	keywords := []string{"code", "coder", "instruct", "llama", "gemini", "claude", "qwen", "gpt", "deepseek", strings.ToLower(lang)}
	for _, k := range keywords {
		if strings.Contains(idLower, k) || strings.Contains(nameLower, k) || strings.Contains(descLower, k) {
			return true
		}
	}

	return false
}

func init() {
	listModelsCmd.Flags().StringVarP(&lang, "lang", "l", "golang", "Language to review (python, typescript, golang)")
	rootCmd.AddCommand(listModelsCmd)
}
