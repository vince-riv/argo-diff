package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vince-riv/argo-diff/internal/process_event"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

const sigHeaderName = "X-Hub-Signature-256"

type WebhookProcessor struct {
	GithubWebhookSecret string
	DevMode             bool
	Wg                  sync.WaitGroup
}

// HTTP Handler for futzing around locally
func (wp *WebhookProcessor) devHandler(w http.ResponseWriter, r *http.Request) {
	log.Debug().Str("method", r.Method).Str("url", r.URL.String()).Msg("dev endpoint")
	if r.Method != "POST" {
		http.Error(w, "Only POSTs allowed", http.StatusMethodNotAllowed)
		return
	}
	var evt webhook.EventInfo
	err := json.NewDecoder(r.Body).Decode(&evt)
	if err != nil {
		http.Error(w, "Cannot unmarshal POST'ed json to webhook.EventInfo struct", http.StatusBadRequest)
		return
	}
	wp.Wg.Add(1)
	var ignoredError error
	go process_event.ProcessCodeChange(evt, wp.DevMode, &wp.Wg, &ignoredError)
	_, _ = io.WriteString(w, "Event dispatched to process_event.ProcessCodeChange()\n")
}

// HTTP handler for github webhook events
func (wp *WebhookProcessor) handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	if wp.DevMode {
		log.Info().Msg("Running in dev mode - skipping signature validation")
	} else {
		signature := r.Header.Get(sigHeaderName)
		if !webhook.VerifySignature(payload, signature, wp.GithubWebhookSecret) {
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
	case "issue_comment":
		eventInfo, err = webhook.ProcessComment(payload)
		if err != nil {
			http.Error(w, "Could not process issue comment data", http.StatusInternalServerError)
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
	wp.Wg.Add(1)
	var ignoredError error
	go process_event.ProcessCodeChange(eventInfo, wp.DevMode, &wp.Wg, &ignoredError)
	_, err = io.WriteString(w, "event accepted for processing\n")
	if err != nil {
		log.Error().Err(err).Msg("io.WriteString() failed")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// HTTP Handler for health checks
func (wp *WebhookProcessor) healthZ(w http.ResponseWriter, r *http.Request) {
	//fmt.Sprintln("EVENT [%s]: %s", event, payload)
	log.Debug().Str("method", r.Method).Str("url", r.URL.String()).Msg("healthz endpoint")
	_, err := io.WriteString(w, "healthy\n")
	if err != nil {
		http.Error(w, "io.WriteString() failed", http.StatusInternalServerError)
		return
	}
}

// HTTP Handler for development - receive github webhook events and log them out
func (wp *WebhookProcessor) printWebHook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Msg("Failed to read request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	log.Info().Str("method", r.Method).Str("url", r.URL.String()).Str("event", event).Msg(string(payload))

	signature := r.Header.Get(sigHeaderName)
	if !webhook.VerifySignature(payload, signature, wp.GithubWebhookSecret) {
		log.Warn().Msg("Invalid signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
}

func StartWebhookProcessor(addr string, webhook_secret string, devMode bool) {
	log.Info().Msgf("Setting up listener on %s", addr)
	if devMode {
		log.Warn().Msg("Dev Mode is enabled - signature validations are disabled!")
		log.Warn().Msg("Dev Mode is enabled - commit status updates are disabled!")
	}

	wp := WebhookProcessor{
		GithubWebhookSecret: webhook_secret,
		DevMode:             devMode,
	}

	srv := &http.Server{Addr: addr}
	http.HandleFunc("/webhook", wp.handleWebhook)
	http.HandleFunc("/webhook_log", wp.printWebHook)
	http.HandleFunc("/healthz", wp.healthZ)
	if devMode {
		http.HandleFunc("/dev", wp.devHandler)
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
	wp.Wg.Wait()
	log.Info().Msg("Server gracefully stopped")
}
