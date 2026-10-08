package gitlab

import (
	"net/url"
	"os"
	"strings"
)

// Provider implements argo-diff's provider primitives on top of the GitLab
// API. It holds no state: the client is a package var built in init(), which
// tests swap for an httptest-backed one.
type Provider struct{}

// providerName is GitLab's name in the scm registry.
const providerName = "gitlab"

func (Provider) Name() string { return providerName }

// Enabled reports whether GITLAB_TOKEN is set.
func (Provider) Enabled() bool {
	return os.Getenv("GITLAB_TOKEN") != ""
}

// ValidateConfig reports why the client could not be built (eg: a malformed
// GITLAB_BASE_URL or an unreadable CA file).
func (Provider) ValidateConfig() error { return clientErr }

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
