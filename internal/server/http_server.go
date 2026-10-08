package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vince-riv/argo-diff/internal/process_event"
	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

type WebhookProcessor struct {
	DevMode bool
	Wg      sync.WaitGroup
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
	p, err := scm.Lookup(evt.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	wp.Wg.Add(1)
	var ignoredError error
	go process_event.ProcessCodeChange(p, evt, wp.DevMode, &wp.Wg, &ignoredError)
	_, _ = io.WriteString(w, "Event dispatched to process_event.ProcessCodeChange()\n")
}

// webhookHandler serves webhook requests for the provider registered as
// providerName ("" is the default provider).
func (wp *WebhookProcessor) webhookHandler(providerName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := scm.Lookup(providerName)
		if err != nil {
			log.Error().Err(err).Msg("No provider for webhook route")
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}
		wp.handleWebhook(p, w, r)
	}
}

// HTTP handler for provider webhook events
func (wp *WebhookProcessor) handleWebhook(p scm.Provider, w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	handler := p.WebhookHandler()
	if wp.DevMode {
		log.Info().Msg("Running in dev mode - skipping signature validation")
	} else if err := handler.Verify(r.Header, payload); err != nil {
		log.Warn().Err(err).Msgf("Rejecting %s webhook", p.Name())
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	evt, err := handler.Parse(r.Header, payload)
	switch {
	case evt.Kind == scm.WebhookPing:
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msg("ping event received")
		_, err := io.WriteString(w, "ping event processed\n")
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done when it's a ping event
	case evt.Kind == scm.WebhookIgnored:
		log.Info().Str("method", r.Method).Str("url", r.URL.String()).Msgf("Ignoring %s event %s", p.Name(), evt.Name)
		_, err := io.WriteString(w, "event ignored\n")
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done when it's an event we don't know about
	case err != nil:
		http.Error(w, fmt.Sprintf("Could not process %s event data", html.EscapeString(evt.Name)), http.StatusInternalServerError)
		return
	}
	eventInfo := evt.Info
	if eventInfo.Ignore {
		log.Info().Msgf("Ignoring %s event. Event Info: %v", evt.Name, eventInfo)
		_, err := io.WriteString(w, fmt.Sprintf("%s event ignored\n%v\n", html.EscapeString(evt.Name), eventInfo))
		if err != nil {
			log.Error().Err(err).Msg("io.WriteString() failed")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return // we're done when it's a PR/PUSH event we don't care about
	}

	// call processEvent in a new gorouting and send a 200 OK back to the provider
	wp.Wg.Add(1)
	var ignoredError error
	go process_event.ProcessCodeChange(p, eventInfo, wp.DevMode, &wp.Wg, &ignoredError)
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

// HTTP Handler for development - receive webhook events for the default
// provider and log them out
func (wp *WebhookProcessor) printWebHook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Msg("Failed to read request body")
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}

	p, err := scm.Lookup("")
	if err != nil {
		log.Error().Err(err).Msg("No provider for webhook route")
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	handler := p.WebhookHandler()
	event := handler.EventName(r.Header)
	log.Info().Str("method", r.Method).Str("url", r.URL.String()).Str("event", event).Msg(string(payload))

	if err := handler.Verify(r.Header, payload); err != nil {
		log.Warn().Msg("Invalid signature")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
}

func StartWebhookProcessor(addr string, devMode bool) {
	log.Info().Msgf("Setting up listener on %s", addr)
	if devMode {
		log.Warn().Msg("Dev Mode is enabled - signature validations are disabled!")
		log.Warn().Msg("Dev Mode is enabled - commit status updates are disabled!")
	}

	wp := WebhookProcessor{
		DevMode: devMode,
	}

	srv := &http.Server{Addr: addr}
	http.HandleFunc("/webhook", wp.webhookHandler(""))
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
