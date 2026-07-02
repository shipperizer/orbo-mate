package cmd

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shipperizer/orbo-mate/pkg/config"
	"github.com/shipperizer/orbo-mate/pkg/pool"
	"github.com/shipperizer/orbo-mate/pkg/reviewer"
	"github.com/shipperizer/orbo-mate/pkg/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the webhook server",
	Long:  `Start the Go HTTP webhook server to listen for GitHub events.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		// Create goroutine worker pool with maximum of 100 concurrent workers
		workerPool := pool.NewPool(100)
		workerPool.Start()
		defer workerPool.Stop()

		rev := reviewer.NewReviewer(cfg, nil)
		srv := server.NewServer(cfg, workerPool, rev)

		httpServer := &http.Server{
			Addr:    ":" + cfg.Port,
			Handler: srv,
		}

		// Server run context
		serverCtx, serverStopCtx := context.WithCancel(context.Background())

		// Listen for syscall signals for graceful shutdown
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		go func() {
			<-sig

			// Shutdown signal with grace period of 30 seconds
			shutdownCtx, cancel := context.WithTimeout(serverCtx, 30*time.Second)
			defer cancel()

			go func() {
				<-shutdownCtx.Done()
				if shutdownCtx.Err() == context.DeadlineExceeded {
					log.Fatal("graceful shutdown timed out.. forcing exit.")
				}
			}()

			// Trigger graceful shutdown
			err := httpServer.Shutdown(shutdownCtx)
			if err != nil {
				log.Fatal(err)
			}
			serverStopCtx()
		}()

		log.Printf("Server starting on port %s...", cfg.Port)
		err = httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}

		// Wait for server context to be completed
		<-serverCtx.Done()
		log.Println("Server stopped gracefully.")
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
