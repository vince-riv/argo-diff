package scm

import (
	"net/http"

	"github.com/vince-riv/argo-diff/internal/webhook"
)

// WebhookKind classifies a parsed webhook request.
type WebhookKind int

const (
	// WebhookIgnored is an event type argo-diff does not handle.
	WebhookIgnored WebhookKind = iota
	// WebhookPing is the provider checking the endpoint; it is acknowledged.
	WebhookPing
	// WebhookChange is a change request event. Its Info may still have
	// Ignore set (eg: a closed PR, or a comment that isn't a refresh request).
	WebhookChange
)

// WebhookEvent is a parsed webhook request.
type WebhookEvent struct {
	// Name is the provider's own event type (eg: GitHub's X-GitHub-Event), for
	// logs and responses.
	Name string
	Kind WebhookKind
	// Info is the event for WebhookChange, with Info.Provider set.
	Info webhook.EventInfo
}

// WebhookHandler authenticates and parses one provider's webhook requests. The
// server reads the body once and passes it to both methods.
type WebhookHandler interface {
	// EventName is the event type named by the request headers, for logs.
	EventName(h http.Header) string
	// Verify authenticates the request; an error means reject it.
	Verify(h http.Header, body []byte) error
	// Parse turns an authenticated request into an event.
	Parse(h http.Header, body []byte) (WebhookEvent, error)
}
