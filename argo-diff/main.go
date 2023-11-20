package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"argo-diff/argocd"
	"argo-diff/gendiff"
	"argo-diff/github"
	"argo-diff/webhook"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	githubWebhookSecret string
)

const gitRevTxt = "git-rev.txt"

const sigHeaderName = "X-Hub-Signature-256"

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get(sigHeaderName)
	if !webhook.VerifySignature(payload, signature, githubWebhookSecret) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	eventInfo := webhook.NewEventInfo()
	switch event {
	case "ping":
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msg("ping event received")
		_, err := io.WriteString(w, "ping event processed\n")
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done with a ping event
	case "pull_request":
		eventInfo, err = webhook.ProcessPullRequest(payload)
		if err != nil {
			http.Error(w, "Could not process pull request event data", http.StatusInternalServerError)
			return
		}
	case "push":
		eventInfo, err = webhook.ProcessPush(payload)
		if err != nil {
			http.Error(w, "Could not process push event data", http.StatusInternalServerError)
			return
		}
	default:
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msgf("Ignoring X-GitHub-Event %s", event)
		_, err := io.WriteString(w, "event ignored\n")
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done with an event we don't know about
	}
	if eventInfo.Ignore {
		log.Info().Msgf("Ignoring %s event. Event Info: %v", event, eventInfo)
		_, err := io.WriteString(w, fmt.Sprintf("%s event ignored\n%v\n", event, eventInfo))
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done with a PR/PUSH event we don't care about
	}
	github.Status(r.Context(), github.StatusPending, "", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha)
	appManifests, err := argocd.GetApplicationManifests(eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.RepoDefaultRef, eventInfo.Sha, eventInfo.ChangeRef)
	if err != nil {
		github.Status(r.Context(), github.StatusError, err.Error(), eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha)
		log.Error().Err(err).Msg("argocd.GetApplicationManifests() failed")
		_, err := io.WriteString(w, "event accepted; processing failed\n")
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done due to a processing error

	}
	log.Trace().Msgf("Received app manifests: %v", appManifests)

	errorCount := 0
	changeCount := 0
	firstError := ""
	for _, am := range appManifests {
		if am.Error != nil {
			errorCount++
			if firstError == "" {
				firstError = am.Error.Message
			}
			// if PR, gen error markdown: am.Error.Message
		} else {
			diffStr, err := gendiff.K8sAppDiff(am.CurrentManifests.Manifests, am.NewManifests.Manifests)
			if err != nil {
				log.Error().Err(err).Msgf("gendiff.K8sAppDiff() failed for %s; SHA %s" + am.ArgoApp.Metadata.Name)
				if firstError == "" {
					firstError = "gendiff.K8sAppDiff() failed"
				}
			}
			if diffStr != "" {
				changeCount++
			}
			// generate diff'ed manifests
			log.Debug().Msg("TODO: Generate markdown for PR comment")
			// if PR, gen diff markdown
		}
	}

	newStatus := github.StatusError
	statusDescription := "Unknown"
	if errorCount > 0 {
		newStatus = github.StatusFailure
		statusDescription = fmt.Sprintf("%d of %d apps with changes; %d had an error; first error: %s", changeCount, len(appManifests), errorCount, firstError)
	} else if firstError != "" {
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%d of %d apps with changes; diff generator had failure; first error: %s", changeCount, len(appManifests), firstError)
	} else {
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%d of %d apps with changes - no errors", changeCount, len(appManifests))
	}
	github.Status(r.Context(), newStatus, statusDescription, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha)

	//		comment := &github.IssueComment{Body: github.String(out.String())}
	//		_, _, err = client.Issues.CreateComment(r.Context(), *prEvent.Repo.Owner.Login, *prEvent.Repo.Name, *prEvent.PullRequest.Number, comment)
	//		if err != nil {
	//			http.Error(w, "Error creating comment on GitHub", http.StatusInternalServerError)
	//			return
	//		}
	//	}
	//}
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

	if os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") == "" {
		log.Fatal().Msg("GITHUB_PERSONAL_ACCESS_TOKEN environment variable not set")
	}
}

func main() {
	log.Info().Msg("Setting up listener on port 8080")
	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/webhook_log", printWebHook)
	http.HandleFunc("/healthz", healthZ)
	log.Error().Err(http.ListenAndServe(":8080", nil)).Msg("")
}
