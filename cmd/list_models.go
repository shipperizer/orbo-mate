package cmd

import (
	"context"
	"fmt"
	"log"

	"os"

	"github.com/shipperizer/orbo-mate/pkg/config"
	"github.com/shipperizer/orbo-mate/pkg/reviewer"
	"github.com/spf13/cobra"
)

var lang string

var listModelsCmd = &cobra.Command{
	Use:   "list-models",
	Short: "List top 10 best value for money models on OpenRouter for reviewing code",
	Run: func(cmd *cobra.Command, args []string) {
		// Provide dummy values for non-essential required variables if they are not set
		if os.Getenv("GITHUB_WEBHOOK_SECRET") == "" {
			os.Setenv("GITHUB_WEBHOOK_SECRET", "dummy-value")
			defer os.Unsetenv("GITHUB_WEBHOOK_SECRET")
		}
		if os.Getenv("GITHUB_TOKEN") == "" {
			os.Setenv("GITHUB_TOKEN", "dummy-value")
			defer os.Unsetenv("GITHUB_TOKEN")
		}
		if os.Getenv("ALLOWED_ORGS") == "" {
			os.Setenv("ALLOWED_ORGS", "dummy-value")
			defer os.Unsetenv("ALLOWED_ORGS")
		}

		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}

		if lang != "python" && lang != "typescript" && lang != "golang" {
			log.Fatalf("Language must be one of: python, typescript, golang")
		}

		rev := reviewer.NewReviewer(cfg, nil)

		prompt := fmt.Sprintf("You are an expert AI consultant. List the top 10 best value for money models available on OpenRouter specifically for reviewing %s code. Justify briefly based on their pricing, context window, and coding capabilities.", lang)

		fmt.Printf("Querying OpenRouter for the best %s models...\n", lang)
		output, err := rev.Chat(context.Background(), cfg.DefaultModel, prompt)
		if err != nil {
			log.Fatalf("Failed to get model recommendations: %v", err)
		}

		fmt.Println("\n" + output)
	},
}

func init() {
	listModelsCmd.Flags().StringVarP(&lang, "lang", "l", "golang", "Language to review (python, typescript, golang)")
	rootCmd.AddCommand(listModelsCmd)
}
