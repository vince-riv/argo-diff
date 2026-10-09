// Package gitlab is the GitLab provider: Provider implements scm.Provider on
// top of the GitLab REST API (gitlab.com/gitlab-org/api/client-go).
package gitlab

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/vince-riv/argo-diff/internal/config"
)

// defaultBaseURL is used when neither GITLAB_BASE_URL nor CI_SERVER_URL is set.
const defaultBaseURL = "https://gitlab.com"

var (
	// client is built in init() when GITLAB_TOKEN is set. Tests swap it for
	// one pointed at an httptest server.
	client *gitlab.Client
	// clientErr is why client could not be built; ValidateConfig() reports it.
	clientErr error
	// webBaseURL is the instance's web URL without a trailing slash (eg:
	// "https://gitlab.com"), for RepoHosts() and log links.
	webBaseURL string

	userMu      sync.Mutex
	currentUser string // cached username of the token's user
)

func init() {
	webBaseURL = baseURL()
	token := os.Getenv("GITLAB_TOKEN")
	if token == "" {
		return
	}
	client, clientErr = newClient(token, webBaseURL, caFile())
	if clientErr != nil {
		log.Error().Err(clientErr).Msg("Failed to create gitlab client")
	}
}

// baseURL is GITLAB_BASE_URL, else CI_SERVER_URL (set in GitLab CI jobs), else
// gitlab.com, without a trailing slash.
func baseURL() string {
	for _, e := range []string{"GITLAB_BASE_URL", "CI_SERVER_URL"} {
		if v := strings.TrimSpace(os.Getenv(e)); v != "" {
			return strings.TrimRight(v, "/")
		}
	}
	return defaultBaseURL
}

// caFile is a PEM bundle of extra CAs to trust: GITLAB_CA_FILE, else
// CI_SERVER_TLS_CA_FILE (set in GitLab CI jobs for instances with a private
// CA). Empty means the system pool alone.
func caFile() string {
	if v := strings.TrimSpace(os.Getenv("GITLAB_CA_FILE")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("CI_SERVER_TLS_CA_FILE"))
}

// newClient builds an API client for the instance at base, trusting the CAs in
// caPath on top of the system pool when caPath is set.
func newClient(token, base, caPath string) (*gitlab.Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid gitlab base URL %q: %w", base, err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("invalid gitlab base URL %q: want http(s)://host[/path]", base)
	}
	opts := []gitlab.ClientOptionFunc{gitlab.WithBaseURL(base)}
	if caPath != "" {
		httpClient, err := httpClientWithCA(caPath)
		if err != nil {
			return nil, err
		}
		opts = append(opts, gitlab.WithHTTPClient(httpClient))
	}
	return gitlab.NewClient(token, opts...)
}

// httpClientWithCA returns an HTTP client trusting the system CAs plus every
// certificate in the PEM file at caPath.
func httpClientWithCA(caPath string) (*http.Client, error) {
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("reading gitlab CA file: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no PEM certificates found in gitlab CA file %s", caPath)
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return &http.Client{Transport: tr}, nil
}

// bypassGitlabCheck reports whether the GitLab connectivity check and
// comment-author matching are bypassed via ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS.
// Read at call time so tests can t.Setenv it.
func bypassGitlabCheck() bool {
	return config.BypassConnectivityCheck(config.ComponentGitlab)
}

// ConnectivityCheck confirms the token works by resolving its user (GET /user).
func ConnectivityCheck() error {
	if client == nil {
		return errors.New("gitlab client is not initialized")
	}
	if bypassGitlabCheck() {
		log.Warn().Msgf("Skipping Gitlab connectivity test per %s", config.BypassEnvVar)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Info().Msgf("Calling Gitlab API at %s for a connectivity test", webBaseURL)
	_, err := getCurrentUser(ctx)
	return err
}

// getCurrentUser returns the username of the token's user, calling GET /user
// once and caching the result. Project and group access tokens act as bot
// users, which have a username like any other.
func getCurrentUser(ctx context.Context) (string, error) {
	if client == nil {
		return "", errors.New("no gitlab client")
	}
	// held across the call so concurrent first callers make one request
	userMu.Lock()
	defer userMu.Unlock()
	if currentUser != "" {
		return currentUser, nil
	}
	user, resp, err := client.Users.CurrentUser(gitlab.WithContext(ctx))
	logResponse(resp, "Users.CurrentUser()")
	if err != nil {
		logAPIError(err, "Unable to determine my gitlab user")
		return "", err
	}
	if user == nil || user.Username == "" {
		return "", errors.New("gitlab user resolved to an empty username")
	}
	currentUser = user.Username
	log.Info().Msgf("Gitlab comment user name: %s", currentUser)
	return currentUser, nil
}

// logResponse logs the HTTP status of an API call, like the GitHub provider.
func logResponse(resp *gitlab.Response, call string) {
	if resp != nil && resp.Response != nil {
		log.Info().Msgf("%s received when calling %s via client-go", resp.Status, call)
	}
}

// logAPIError logs err. For a fine-grained token that lacks a permission,
// GitLab answers 403 insufficient_granular_scope and names the missing
// permission in error_description; that text is logged as-is, since it tells
// the operator exactly what to grant.
func logAPIError(err error, msg string) {
	ev := log.Error().Err(err)
	if desc := errorDescription(err); desc != "" {
		ev = ev.Str("error_description", desc)
	}
	ev.Msg(msg)
}

// errorDescription returns the error_description of a GitLab API error body,
// or "".
func errorDescription(err error) string {
	var er *gitlab.ErrorResponse
	if !errors.As(err, &er) || len(er.Body) == 0 {
		return ""
	}
	var body struct {
		Description string `json:"error_description"`
	}
	if json.Unmarshal(er.Body, &body) != nil {
		return ""
	}
	return body.Description
}
