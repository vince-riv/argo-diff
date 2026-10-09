package gitlab

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

// Provider implements argo-diff's provider primitives on top of the GitLab
// API. It holds no state: the client is a package var built in init(), which
// tests swap for an httptest-backed one.
type Provider struct{}

var _ scm.Provider = Provider{}

// providerName is GitLab's name in the scm registry.
const providerName = "gitlab"

func (Provider) Name() string { return providerName }

// Enabled reports whether GITLAB_TOKEN is set.
func (Provider) Enabled() bool {
	return os.Getenv("GITLAB_TOKEN") != ""
}

// ValidateConfig builds the API client, reporting why it cannot be built
// (eg: a malformed GITLAB_BASE_URL or an unreadable CA file). cmd/main.go
// calls it once at startup, only when gitlab is enabled.
func (Provider) ValidateConfig() error {
	c, err := newClient(os.Getenv("GITLAB_TOKEN"), webBaseURL, caFile())
	if err != nil {
		return err
	}
	client = c
	return nil
}

// CredentialsHint names the variable that enables this provider.
func (Provider) CredentialsHint() string { return "GITLAB_TOKEN" }

func (Provider) ConnectivityCheck() error { return ConnectivityCheck() }

// RepoHosts is the configured instance's host, plus its relative URL root if
// it has one (eg: "example.com/gitlab"), for matching ArgoCD application
// sources.
func (Provider) RepoHosts() []string {
	u, err := url.Parse(webBaseURL)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	return []string{u.Hostname() + strings.TrimRight(u.Path, "/")}
}

// DetectCI is false until GitLab CI support lands (issue #160, Phase 3).
func (Provider) DetectCI() bool { return false }

// EventFromCIEnv is not supported yet; DetectCI() never selects it.
func (Provider) EventFromCIEnv() (webhook.EventInfo, error) {
	return webhook.EventInfo{}, errors.New("gitlab CI is not supported yet")
}

func (Provider) WebhookHandler() scm.WebhookHandler { return WebhookHandler{} }

// errWebhooksUnsupported rejects every GitLab webhook request.
var errWebhooksUnsupported = errors.New("gitlab webhooks are not supported yet")

// WebhookHandler is a placeholder until GitLab webhook support lands (issue
// #160, Phase 4). No route serves GitLab webhooks yet; should one reach it,
// Verify() rejects the request.
type WebhookHandler struct{}

// CheckConfig warns that GitLab merge requests can only be processed from
// event files (-f) or /dev for now. It does not fail, so a server with
// GITLAB_TOKEN set still starts for GitHub.
func (WebhookHandler) CheckConfig() error {
	log.Warn().Msg("GitLab webhooks are not supported yet - process GitLab merge requests with -f event files")
	return nil
}

func (WebhookHandler) EventName(h http.Header) string { return h.Get("X-Gitlab-Event") }

func (WebhookHandler) Verify(http.Header, []byte) error { return errWebhooksUnsupported }

func (WebhookHandler) Parse(h http.Header, _ []byte) (scm.WebhookEvent, error) {
	return scm.WebhookEvent{Name: h.Get("X-Gitlab-Event"), Kind: scm.WebhookIgnored}, errWebhooksUnsupported
}
