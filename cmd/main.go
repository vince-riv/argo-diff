package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/gendiff"
	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/webhook"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	githubWebhookSecret string
	devMode             bool
	wg                  sync.WaitGroup
)

const gitRevTxt = "git-rev.txt"

const sigHeaderName = "X-Hub-Signature-256"

func processEvent(eventInfo webhook.EventInfo) {
	defer wg.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	isPr := eventInfo.PrNum > 0
	github.Status(ctx, isPr, github.StatusPending, "", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)
	appManifests, err := argocd.GetApplicationManifests(eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.RepoDefaultRef, eventInfo.Sha, eventInfo.ChangeRef)
	if err != nil {
		github.Status(ctx, isPr, github.StatusError, err.Error(), eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)
		log.Error().Err(err).Msg("argocd.GetApplicationManifests() failed")
		return // we're done due to a processing error

	}
	log.Trace().Msgf("Received app manifests: %v", appManifests)

	errorCount := 0
	changeCount := 0
	unknownCount := 0
	firstError := ""
	markdown := ""
	for _, am := range appManifests {
		if am.Error != nil {
			if am.Error.Code == argocd.ErrCurAppManifestFetch || am.Error.Code == argocd.ErrCurAppManifestDecode {
				// don't fail the check if just current manifests are busted
				unknownCount++
				markdown += github.AppMarkdownStart(am.ArgoApp.Metadata.Name, "Warning: Unable to fetch base ref manifests to generate diff")
				markdown += github.AppMarkdownEnd()
			} else {
				errorCount++
				markdown += github.AppMarkdownStart(am.ArgoApp.Metadata.Name, "Error: "+am.Error.Message)
				markdown += github.AppMarkdownEnd()
			}
			if firstError == "" {
				firstError = am.Error.Message
			}
		} else {
			k8sDiffs, err := gendiff.K8sAppDiff(am.CurrentManifests.Manifests, am.NewManifests.Manifests)
			if err != nil {
				log.Error().Err(err).Msgf("gendiff.K8sAppDiff() failed for %s; SHA %s" + am.ArgoApp.Metadata.Name)
				if firstError == "" {
					firstError = "gendiff.K8sAppDiff() failed"
				}
				markdown += github.AppMarkdownStart(am.ArgoApp.Metadata.Name, "Warning: Unable to generate diff, but manifests were succesfully fetched")
				markdown += github.AppMarkdownEnd()
			}
			if len(k8sDiffs) > 0 {
				changeCount++
				markdown += github.AppMarkdownStart(am.ArgoApp.Metadata.Name, "")
				for _, k := range k8sDiffs {
					markdown += github.ResourceDiffMarkdown(k.ApiVersion, k.Kind, k.Name, k.Namespace, k.DiffStr)
				}
				markdown += github.AppMarkdownEnd()
			}
		}
	}

	newStatus := github.StatusError
	statusDescription := "Unknown"
	changeCountStr := fmt.Sprintf("%d of %d apps with changes", changeCount, len(appManifests))
	if unknownCount > 0 {
		changeCountStr += fmt.Sprintf(" [%d apps unknown]", unknownCount)
	}
	markdownStart := changeCountStr
	if isPr {
		t := time.Now()
		tStr := t.Format("2006-01-02 15:04:05Z07:00")
		markdownStart += fmt.Sprintf(" as compared to manifests in [%s](https://github.com/%s/%s/tree/%s) as of _%s_", eventInfo.BaseRef, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.BaseRef, tStr)
	}
	if errorCount > 0 {
		newStatus = github.StatusFailure
		statusDescription = fmt.Sprintf("%s; %d had an error; first error: %s", changeCountStr, errorCount, firstError)
	} else if firstError != "" {
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s; diff generator failed; first error: %s", changeCountStr, firstError)
	} else {
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s - no errors", changeCountStr)
	}
	github.Status(ctx, isPr, newStatus, statusDescription, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)

	if eventInfo.PrNum > 0 && (changeCount > 0 || errorCount > 0 || unknownCount > 0) {
		_, _ = github.Comment(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum, markdownStart+"\n\n"+markdown)
		return
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	if devMode {
		log.Info().Msg("Running in dev mode - skipping signature validation")
	} else {
		signature := r.Header.Get(sigHeaderName)
		if !webhook.VerifySignature(payload, signature, githubWebhookSecret) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
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
	// call process event in a new gorouting and send a 200 OK back to Github
	wg.Add(1)
	go processEvent(eventInfo)
	_, err = io.WriteString(w, "event accepted for processing\n")
	if err != nil {
		log.Error().Err(err).Msg("io.WriteString() failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func devHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug().Str("method", r.Method).Str("url", r.URL.String()).Msg("dev endpoint")
	evt := webhook.EventInfo{
		Ignore:         false,
		RepoOwner:      "vince-riv",
		RepoName:       "argo-diff",
		RepoDefaultRef: "main",
		Sha:            "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		PrNum:          2,
		ChangeRef:      "test-branch",
		BaseRef:        "main",
	}
	wg.Add(1)
	go processEvent(evt)
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
	devMode = false
	if os.Getenv("APP_ENV") == "dev" {
		devMode = true
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if devMode {
		// zerolog.SetGlobalLevel(zerolog.TraceLevel)
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else if debug {
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
	srv := &http.Server{Addr: ":8080"}
	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/webhook_log", printWebHook)
	http.HandleFunc("/healthz", healthZ)
	if devMode {
		http.HandleFunc("/dev", devHandler)
	}
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http.ListenAndServe(':8080', nil) failed")
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop // block until TERM or INT is received
	log.Info().Msg("Shutting down...")

	// Shut down the server with a context (for 30s timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Warn().Err(err).Msg("Server forced to shutdown")
	}
	// Wait for all goroutines to finish
	wg.Wait()
	log.Info().Msg("Server gracefully stopped")
}
