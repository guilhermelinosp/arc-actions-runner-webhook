package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	namespace  = env("NAMESPACE", "arc-actions")
	owner      = env("OWNER", "guilhermelinosp")
	runnerImg  = env("RUNNER_IMAGE", "ghcr.io/guilhermelinosp/arc-runner:latest")
	webhookSec = env("WEBHOOK_SECRET", "")
	port       = env("PORT", "8080")
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type runnerResponse struct {
	OK      bool   `json:"ok"`
	Exists  string `json:"exists,omitempty"`
	Skipped string `json:"skipped,omitempty"`
	Runner  string `json:"runner,omitempty"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("starting runner-webhook on :%s (owner=%s, ns=%s)", port, owner, namespace)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)

	srv := &http.Server{Addr: fmt.Sprintf(":%s", port), Handler: mux}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("runner-controller: ok"))

	case http.MethodPost:
		handleWebhook(w, r)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
