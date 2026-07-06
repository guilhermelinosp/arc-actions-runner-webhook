package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type runnerResponse struct {
	OK      bool   `json:"ok"`
	Exists  string `json:"exists,omitempty"`
	Skipped string `json:"skipped,omitempty"`
	Runner  string `json:"runner,omitempty"`
}

type webhookPayload struct {
	Action     string     `json:"action"`
	Repository *repoInfo  `json:"repository"`
}

type repoInfo struct {
	FullName string  `json:"full_name"`
	Name     string  `json:"name"`
	Owner    *ownerInfo `json:"owner"`
}

type ownerInfo struct {
	Login string `json:"login"`
}

var (
	eventsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_events_total",
		Help: "Total webhook events processed.",
	}, []string{"event", "status"})

	creationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "runner_creation_duration_seconds",
		Help:    "Time to create runner resources.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
	})

	runnersActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "runners_active_total",
		Help: "Number of active runners.",
	})
)

func newWebhookHandler(kc *k8sController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("runner-controller: ok"))

		case http.MethodPost:
			handleWebhook(w, r, kc)

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request, kc *k8sController) {
	start := time.Now()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("read body", "error", err)
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	event := r.Header.Get("X-GitHub-Event")

	// HMAC verification
	if webhookSec != "" && sig != "" {
		mac := hmac.New(sha256.New, []byte(webhookSec))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(sig)) {
			slog.Warn("invalid signature", "remote", r.RemoteAddr)
			eventsTotal.WithLabelValues(event, "unauthorized").Inc()
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	switch event {
	case "ping":
		eventsTotal.WithLabelValues("ping", "ok").Inc()
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return

	case "repository":
		// handle below

	default:
		eventsTotal.WithLabelValues(event, "ignored").Inc()
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		slog.Error("json decode", "error", err)
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	if payload.Action != "created" {
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return
	}

	repo := payload.Repository
	if repo == nil || repo.Owner == nil {
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return
	}

	if repo.Owner.Login != owner {
		slog.Info("skipping: owner mismatch",
			"repo", repo.FullName, "owner", repo.Owner.Login, "expected", owner,
		)
		eventsTotal.WithLabelValues("repository", "owner_mismatch").Inc()
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return
	}

	fullName := repo.FullName
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	slog.Info("webhook: repo created", "repo", fullName)

	// Check if runner already exists
	exists, err := kc.runnerExists(ctx, fullName)
	if err != nil {
		slog.Error("check runner", "repo", fullName, "error", err)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if exists {
		slog.Info("runner already exists", "repo", fullName)
		eventsTotal.WithLabelValues("repository", "exists").Inc()
		writeJSON(w, http.StatusOK, runnerResponse{OK: true, Exists: fullName})
		return
	}

	// Create runner
	creationTimer := prometheus.NewTimer(creationDuration)
	err = kc.createRunner(ctx, fullName, repo.Name)
	creationTimer.ObserveDuration()

	if err != nil {
		slog.Error("create runner", "repo", fullName, "error", err)
		eventsTotal.WithLabelValues("repository", "error").Inc()
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}

	runnersActive.Inc()
	eventsTotal.WithLabelValues("repository", "created").Inc()
	slog.Info("runner created", "repo", fullName,
		"duration", time.Since(start).String(),
	)
	writeJSON(w, http.StatusOK, runnerResponse{OK: true, Runner: fullName})
}

func hasWorkflows(fullName string) (bool, error) {
	token := gitToken
	if token == "" {
		return true, nil
	}

	req, err := http.NewRequest("GET",
		fmt.Sprintf("https://api.github.com/repos/%s/contents/.github/workflows", fullName), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "runner-controller")

	// Reuse default client
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}


