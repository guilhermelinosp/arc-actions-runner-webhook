package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	namespace  = env("NAMESPACE", "arc-actions")
	owner      = env("OWNER", "guilhermelinosp")
	runnerImg  = env("RUNNER_IMAGE", "ghcr.io/guilhermelinosp/arc-runner:latest")
	webhookSec = env("WEBHOOK_SECRET", "")
	gitToken   = env("GITHUB_TOKEN", "")
	port       = env("PORT", "8080")
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	slog.Info("starting runner-webhook",
		"port", port, "namespace", namespace, "owner", owner,
	)

	// Kubernetes client (in-cluster)
	config, err := rest.InClusterConfig()
	if err != nil {
		slog.Error("in-cluster config", "error", err)
		os.Exit(1)
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("k8s clientset", "error", err)
		os.Exit(1)
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		slog.Error("dynamic client", "error", err)
		os.Exit(1)
	}

	kc := &k8sController{
		dynClient: dynClient,
		k8sClient: k8sClient,
		namespace: namespace,
		runnerImg: runnerImg,
	}

	// Metrics
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	go func() {
		slog.Info("metrics server", "addr", ":9090")
		if err := http.ListenAndServe(":9090", metricsMux); err != nil {
			slog.Error("metrics server", "error", err)
		}
	}()

	// Webhook server
	mux := http.NewServeMux()
	mux.HandleFunc("/", newWebhookHandler(kc))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("webhook server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
