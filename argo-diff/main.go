package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"argo-diff/webhook"

	"github.com/google/go-github/v56/github" // Ensure to get the latest version
	"golang.org/x/oauth2"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	githubWebhookSecret string
	githubApiToken      string
)

const commandToRun = "YOUR_COMMAND_HERE"

const gitRevTxt = "git-rev.txt"

const sigHeaderName = "X-Hub-Signature-256"

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get(sigHeaderName)
	if !webhook.VerifySignature(payload, signature, githubWebhookSecret) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	if event == "ping" {
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msg("ping event received")
	}
	if event == "pull_request" {
		var prEvent github.PullRequestEvent
		if err := json.Unmarshal(payload, &prEvent); err != nil {
			http.Error(w, "Error unmarshalling JSON", http.StatusInternalServerError)
			return
		}

		if *prEvent.Action == "opened" || *prEvent.Action == "synchronize" {
			cmd := exec.Command("/bin/sh", "-c", commandToRun)
			var out bytes.Buffer
			cmd.Stdout = &out
			err := cmd.Run()

			// Setting up GitHub client
			ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: githubApiToken})
			tc := oauth2.NewClient(r.Context(), ts)
			client := github.NewClient(tc)

			// Determine command status and post commit status to GitHub
			var status string
			var description string
			if err != nil {
				status = "failure"
				description = "Command execution failed."
			} else {
				status = "success"
				description = "Command executed successfully."
			}

			repoStatus := &github.RepoStatus{State: &status, Description: &description, Context: github.String("continuous-integration/my-ci")}
			_, _, err = client.Repositories.CreateStatus(r.Context(), *prEvent.Repo.Owner.Login, *prEvent.Repo.Name, *prEvent.PullRequest.Head.SHA, repoStatus)
			if err != nil {
				http.Error(w, "Error setting commit status on GitHub", http.StatusInternalServerError)
				return
			}

			comment := &github.IssueComment{Body: github.String(out.String())}
			_, _, err = client.Issues.CreateComment(r.Context(), *prEvent.Repo.Owner.Login, *prEvent.Repo.Name, *prEvent.PullRequest.Number, comment)
			if err != nil {
				http.Error(w, "Error creating comment on GitHub", http.StatusInternalServerError)
				return
			}
		}
	}
}

func healthZ(w http.ResponseWriter, r *http.Request) {
	//fmt.Sprintln("EVENT [%s]: %s", event, payload)
	log.Debug().Str("method", r.Method).Str("url", r.URL.String()).Msg("healthz endpoint")
	_, err := io.WriteString(w, "healthy\n")
	if err != nil {
		http.Error(w, "io.WriteString() failed", http.StatusInternalServerError)
		return
	}
}

func printWebHook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Msg("Failed to read request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	log.Info().Str("method", r.Method).Str("url", r.URL.String()).Str("event", event).Msg(string(payload))

	signature := r.Header.Get(sigHeaderName)
	if !webhook.VerifySignature(payload, signature, githubWebhookSecret) {
		log.Warn().Msg("Invalid signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
}

func init() {
	// Load GitHub secrets from env and setup logger
	debug := true // TODO: switch to env var
	gitRev := "UNKNOWN"
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	data, err := os.ReadFile(gitRevTxt)
	if err != nil {
		log.Info().Msg(fmt.Sprintf("Cannot open %s; assuming we're in local development", gitRevTxt))
	} else {
		lines := strings.Split(string(data), "\n")
		gitRev = strings.TrimSpace(lines[0])
		if gitRev == "" {
			log.Warn().Msg(fmt.Sprintf("%s must be empty?", gitRevTxt))
			gitRev = "EMPTY"
		}
	}

	log.Logger = log.With().Str("service", "argo-diff").Str("version", gitRev).Caller().Logger()

	githubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	if githubWebhookSecret == "" {
		log.Fatal().Msg("GITHUB_WEBHOOK_SECRET environment variable not set")
	}

	githubApiToken = os.Getenv("GITHUB_API_TOKEN")
	if githubApiToken == "" {
		log.Fatal().Msg("GITHUB_API_TOKEN environment variable not set")
	}
}

func main() {
	log.Info().Msg("Setting up listener on port 8080")
	//http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/webhook", printWebHook)
	http.HandleFunc("/healthz", healthZ)
	log.Error().Err(http.ListenAndServe(":8080", nil)).Msg("")
}
