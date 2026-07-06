package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("X-Hub-Signature-256")
	event := r.Header.Get("X-GitHub-Event")

	// signature verification
	if webhookSec != "" && sig != "" {
		mac := hmac.New(sha256.New, []byte(webhookSec))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(expected), []byte(sig)) {
			log.Printf("invalid signature from %s", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	switch event {
	case "ping":
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return
	case "repository":
		// handle below
	default:
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return
	}

	var payload struct {
		Action     string `json:"action"`
		Repository *struct {
			FullName string `json:"full_name"`
			Name     string `json:"name"`
			Owner    *struct {
				Login string `json:"login"`
			} `json:"owner"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("json decode error: %v", err)
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
		log.Printf("skipping repo %s: owner %s != %s", repo.FullName, repo.Owner.Login, owner)
		writeJSON(w, http.StatusOK, runnerResponse{OK: true})
		return
	}

	fullName := repo.FullName
	log.Printf("webhook: repo created → %s", fullName)

	// check if runner already exists
	exists, err := runnerExists(fullName)
	if err != nil {
		log.Printf("error checking runner: %v", err)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if exists {
		log.Printf("runner already exists for %s", fullName)
		writeJSON(w, http.StatusOK, runnerResponse{OK: true, Exists: fullName})
		return
	}

	if err := createRunner(fullName, repo.Name); err != nil {
		log.Printf("error creating runner for %s: %v", fullName, err)
		http.Error(w, "create error", http.StatusInternalServerError)
		return
	}

	log.Printf("runner created for %s", fullName)
	writeJSON(w, http.StatusOK, runnerResponse{OK: true, Runner: fullName})
}

func runnerExists(fullName string) (bool, error) {
	cmd := exec.Command("kubectl", "get", "runnerdeployment",
		"-n", namespace,
		"-o", "jsonpath={.items[*].spec.template.spec.repository}")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("kubectl get runnerdeployment: %w", err)
	}
	repos := strings.Fields(string(out))
	for _, r := range repos {
		if r == fullName {
			return true, nil
		}
	}
	return false, nil
}

func hasWorkflows(fullName string) (bool, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return true, nil // skip check if no token
	}

	req, err := http.NewRequest("GET",
		fmt.Sprintf("https://api.github.com/repos/%s/contents/.github/workflows", fullName), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "runner-controller")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

func createRunner(fullName, repoName string) error {
	safeName := strings.NewReplacer(".", "-", "_", "-").Replace(strings.ToLower(repoName))
	manifest := fmt.Sprintf(`---
apiVersion: actions.summerwind.dev/v1alpha1
kind: RunnerDeployment
metadata:
  name: runner-%s
  namespace: %s
spec:
  replicas: 1
  template:
    spec:
      repository: %s
      image: %s
      dockerdWithinRunnerContainer: false
      resources:
        limits:
          cpu: "1"
          memory: 2Gi
        requests:
          cpu: 100m
          memory: 256Mi
---
apiVersion: actions.summerwind.dev/v1alpha1
kind: HorizontalRunnerAutoscaler
metadata:
  name: runner-%s-autoscaler
  namespace: %s
spec:
  scaleTargetRef:
    name: runner-%s
    kind: RunnerDeployment
  minReplicas: 0
  maxReplicas: 5
  metrics:
    - type: TotalNumberOfQueuedAndInProgressWorkflowRuns
      repositoryNames:
        - %s
`, safeName, namespace, fullName, runnerImg,
		safeName, namespace, safeName, fullName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply: %w\n%s", err, out)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
