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

// Returns first 7 characters of a string (to produce a short commit sha)
func shortSha(str string) string {
	v := []rune(str)
	if len(v) <= 7 {
		return str
	}
	return string(v[:7])
}

// Processes github webhook event data by getting a list of matching argo applications & their manifests and generating diffs
// Sets Github status checks for the relevant commit sha and posts a Github comment it is a pull-request event
// Designed to run within a gorouting to decouple from the webhook response
func processEvent(eventInfo webhook.EventInfo) {
	defer wg.Done()
	// Don't take longer than 3 minutes to execute
	// TODO update internal/argocd to use ctx to gracefully handle timeouts
	// TODO figure out how to call github.Status() with an error status when there's a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	isPr := eventInfo.PrNum > 0

	// set commit status to PENDING
	github.Status(ctx, isPr, github.StatusPending, "", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)

	// get a list of ArgoCD applications and their manifests whose git URLs match the webhook event
	appResList, err := argocd.GetApplicationChanges(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.RepoDefaultRef, eventInfo.Sha, eventInfo.ChangeRef)
	if err != nil {
		github.Status(ctx, isPr, github.StatusError, err.Error(), eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)
		log.Error().Err(err).Msg("argocd.GetApplicationChanges() failed")
		return // we're done due to a processing error

	}

	errorCount := 0   // keep track of the number of errors
	changeCount := 0  // how many apps have changes
	unknownCount := 0 // how many apps we can't determine if there's changes (usually when we can new manifests but not current ones)
	firstError := ""  // string of the first error we receive - used in commit status message
	markdown := ""    // markdown for pull request comment
	for _, a := range appResList {
		appName := a.ArgoApp.ObjectMeta.Name
		appSyncStatus := a.ArgoApp.Status.Sync.Status
		appHealthStatus := a.ArgoApp.Status.Health.Status
		appHealthMsg := a.ArgoApp.Status.Health.Message
		// appNs := a.ArgoApp.ObjectMeta.Namespance
		if a.WarnStr != "" {
			errorCount++
			markdown += github.AppMarkdownStart(appName, "Error: "+a.WarnStr, appSyncStatus, appHealthStatus, appHealthMsg)
			markdown += github.AppMarkdownEnd()
			if firstError == "" {
				firstError = a.WarnStr
			}
		} else {
			if len(a.ChangedResources) > 0 {
				changeCount++
				markdown += github.AppMarkdownStart(appName, "", appSyncStatus, appHealthStatus, appHealthMsg)
				for _, ar := range a.ChangedResources {
					diffStr := gendiff.UnifiedDiff("live.yaml", fmt.Sprintf("%s.yaml", shortSha(eventInfo.Sha)), ar.YamlCur, ar.YamlNew)
					markdown += github.ResourceDiffMarkdown(ar.ApiVersion, ar.Kind, ar.Name, ar.Namespace, diffStr)
				}
				markdown += github.AppMarkdownEnd()
			}
		}
	}

	newStatus := github.StatusError // commit status is currently pending, newStatus will be the updated status (default to error)
	statusDescription := "Unknown"
	changeCountStr := fmt.Sprintf("%d of %d apps with changes", changeCount, len(appResList))
	if unknownCount > 0 {
		changeCountStr += fmt.Sprintf(" [%d apps unknown]", unknownCount)
	}
	markdownStart := changeCountStr // markdownStart is the pre-amble of the github comment

	if errorCount > 0 {
		// if we had errors, commit status should be a failure
		newStatus = github.StatusFailure
		statusDescription = fmt.Sprintf("%s; %d had an error; first error: %s", changeCountStr, errorCount, firstError)
	} else if firstError != "" {
		// if we had a recoverable error, commit status can be a success (but let's give them the first error)
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s; diff generator failed; first error: %s", changeCountStr, firstError)
	} else {
		// else everything is happy - commit status success
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s - no errors", changeCountStr)
	}
	// send the commit status
	github.Status(ctx, isPr, newStatus, statusDescription, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)

	if isPr && (changeCount > 0 || errorCount > 0 || unknownCount > 0) {
		// if it's a pull-request event, only comment when something has happened
		t := time.Now()
		tStr := t.Format("3:04PM MST, 2 Jan 2006")
		markdownStart += " compared to live state\n"
		markdownStart += "\n" + tStr + "\n"
		// markdownStart += fmt.Sprintf(" as compared to live state in [%s](https://github.com/%s/%s/tree/%s) as of _%s_", eventInfo.BaseRef, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.BaseRef, tStr)
		_, _ = github.Comment(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum, markdownStart+"\n\n"+markdown)
		return
	}
}

// HTTP handler for github webhook events
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
		return // we're done when it's a ping event
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
		return // we're done when it's an event we don't know about
	}
	if eventInfo.Ignore {
		log.Info().Msgf("Ignoring %s event. Event Info: %v", event, eventInfo)
		_, err := io.WriteString(w, fmt.Sprintf("%s event ignored\n%v\n", event, eventInfo))
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done when it's a PR/PUSH event we don't care about
	}

	// call processEvent in a new gorouting and send a 200 OK back to Github
	wg.Add(1)
	go processEvent(eventInfo)
	_, err = io.WriteString(w, "event accepted for processing\n")
	if err != nil {
		log.Error().Err(err).Msg("io.WriteString() failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// HTTP Handler for futzing around locally
func devHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug().Str("method", r.Method).Str("url", r.URL.String()).Msg("dev endpoint")
	evt := webhook.EventInfo{
		Ignore:         false,
		RepoOwner:      "vince-riv",
		RepoName:       "argo-diff-config",
		RepoDefaultRef: "main",
		Sha:            "06cddc3a36c3b7758349104a429488901923c6c4",
		PrNum:          2,
		ChangeRef:      "annotation",
		BaseRef:        "main",
	}
	wg.Add(1)
	go processEvent(evt)
}

// HTTP Handler for health checks
func healthZ(w http.ResponseWriter, r *http.Request) {
	//fmt.Sprintln("EVENT [%s]: %s", event, payload)
	log.Debug().Str("method", r.Method).Str("url", r.URL.String()).Msg("healthz endpoint")
	_, err := io.WriteString(w, "healthy\n")
	if err != nil {
		http.Error(w, "io.WriteString() failed", http.StatusInternalServerError)
		return
	}
}

// HTTP Handler for development - receive github webhook events and log them out
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

	// git revision for logger - there's probably a better way to do this at build time
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

	// make sure critical secrets are set in the environment
	if os.Getenv("ARGOCD_AUTH_TOKEN") == "" {
		log.Fatal().Msg("ARGOCD_AUTH_TOKEN environment variable not set")
	}
	if os.Getenv("ARGOCD_BASE_URL") == "" {
		log.Fatal().Msg("ARGOCD_BASE_URL environment variable not set")
	}
	if os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") == "" {
		log.Fatal().Msg("GITHUB_PERSONAL_ACCESS_TOKEN environment variable not set")
	}
	githubWebhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	if githubWebhookSecret == "" {
		log.Fatal().Msg("GITHUB_WEBHOOK_SECRET environment variable not set")
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
	// Wait for all processEvent() goroutines to finish
	wg.Wait()
	log.Info().Msg("Server gracefully stopped")
}
