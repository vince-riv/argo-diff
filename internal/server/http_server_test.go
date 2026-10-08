package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

// stubHandler returns a canned verification result and event.
type stubHandler struct {
	verifyErr error
	evt       scm.WebhookEvent
	parseErr  error
}

func (h stubHandler) CheckConfig() error           { return nil }
func (h stubHandler) EventName(http.Header) string { return h.evt.Name }
func (h stubHandler) Verify(http.Header, []byte) error {
	return h.verifyErr
}
func (h stubHandler) Parse(http.Header, []byte) (scm.WebhookEvent, error) {
	return h.evt, h.parseErr
}

// stubProvider is an scm.Provider whose only working part is its webhook
// handler; the embedded nil Provider panics if the server reaches anything
// else for these requests.
type stubProvider struct {
	scm.Provider
	handler stubHandler
}

func (p stubProvider) Name() string                       { return scm.DefaultProvider }
func (p stubProvider) WebhookHandler() scm.WebhookHandler { return p.handler }

func TestHandleWebhookResponses(t *testing.T) {
	ignored := webhook.NewEventInfo()
	cases := []struct {
		name     string
		devMode  bool
		handler  stubHandler
		wantCode int
		wantBody string
	}{
		{"bad signature", false, stubHandler{verifyErr: errors.New("invalid signature")}, http.StatusUnauthorized, "Invalid signature"},
		{"dev mode skips verification", true, stubHandler{verifyErr: errors.New("invalid signature"), evt: scm.WebhookEvent{Name: "ping", Kind: scm.WebhookPing}}, http.StatusOK, "ping event processed"},
		{"ping", false, stubHandler{evt: scm.WebhookEvent{Name: "ping", Kind: scm.WebhookPing}}, http.StatusOK, "ping event processed"},
		{"unhandled event type", false, stubHandler{evt: scm.WebhookEvent{Name: "push", Kind: scm.WebhookIgnored}}, http.StatusOK, "event ignored"},
		{"parse error", false, stubHandler{evt: scm.WebhookEvent{Name: "pull_request", Kind: scm.WebhookChange}, parseErr: errors.New("bad json")}, http.StatusInternalServerError, "Could not process pull_request event data"},
		{"ignored change", false, stubHandler{evt: scm.WebhookEvent{Name: "pull_request", Kind: scm.WebhookChange, Info: ignored}}, http.StatusOK, "pull_request event ignored"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scm.Register(stubProvider{handler: tc.handler})
			wp := &WebhookProcessor{DevMode: tc.devMode}
			rec := httptest.NewRecorder()
			wp.webhookHandler("")(rec, httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("{}")))
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("got %d %q, want %d containing %q", rec.Code, rec.Body.String(), tc.wantCode, tc.wantBody)
			}
		})
	}
}

func TestWebhookHandlerUnknownProvider(t *testing.T) {
	wp := &WebhookProcessor{}
	rec := httptest.NewRecorder()
	wp.webhookHandler("bitbucket")(rec, httptest.NewRequest(http.MethodPost, "/webhook/bitbucket", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404 for an unregistered provider", rec.Code)
	}
}
