package gitlab

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"
)

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name, base, ci, want string
	}{
		{"default", "", "", "https://gitlab.com"},
		{"ci server url", "", "https://gitlab.ci.example.com", "https://gitlab.ci.example.com"},
		{"base url wins", "https://gitlab.example.com/", "https://gitlab.ci.example.com", "https://gitlab.example.com"},
		{"relative root", " https://example.com/gitlab/ ", "", "https://example.com/gitlab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITLAB_BASE_URL", tc.base)
			t.Setenv("CI_SERVER_URL", tc.ci)
			if got := baseURL(); got != tc.want {
				t.Errorf("baseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCAFile(t *testing.T) {
	t.Setenv("GITLAB_CA_FILE", "")
	t.Setenv("CI_SERVER_TLS_CA_FILE", "/ci/ca.pem")
	if got := caFile(); got != "/ci/ca.pem" {
		t.Errorf("caFile() = %q, want the CI value", got)
	}
	t.Setenv("GITLAB_CA_FILE", "/etc/ca.pem")
	if got := caFile(); got != "/etc/ca.pem" {
		t.Errorf("caFile() = %q, want GITLAB_CA_FILE", got)
	}
}

func TestNewClientRejectsBadConfig(t *testing.T) {
	for _, base := range []string{"gitlab.com", "ftp://gitlab.com", "https://", "://bad"} {
		if _, err := newClient("tok", base, ""); err == nil {
			t.Errorf("newClient(base %q) = nil error, want one", base)
		}
	}
	if _, err := newClient("tok", "https://gitlab.com", filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Error("newClient() with a missing CA file = nil error, want one")
	}
	notPEM := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(notPEM, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newClient("tok", "https://gitlab.com", notPEM); err == nil {
		t.Error("newClient() with a CA file holding no PEM = nil error, want one")
	}
}

// A self-managed instance with a private CA is reachable once GITLAB_CA_FILE
// names that CA, and not before.
func TestNewClientTrustsCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 1, "username": "argo-diff-bot"}`))
	}))
	defer srv.Close()
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	withCA, err := newClient("tok", srv.URL, caPath)
	if err != nil {
		t.Fatal(err)
	}
	if u, _, err := withCA.Users.CurrentUser(); err != nil || u.Username != "argo-diff-bot" {
		t.Errorf("CurrentUser() with the CA = %v, %v; want argo-diff-bot", u, err)
	}

	withoutCA, err := newClient("tok", srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := withoutCA.Users.CurrentUser(gitlab.WithContext(context.Background())); err == nil {
		t.Error("CurrentUser() without the CA = nil error, want a TLS error")
	}
}

func TestConnectivityCheck(t *testing.T) {
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "")
	fs := newFixtureServer(t, fixtureRoute{Method: "GET", Path: "/api/v4/user", Fixture: "user"})
	if err := ConnectivityCheck(); err != nil {
		t.Fatalf("ConnectivityCheck() = %v", err)
	}
	if login, err := getCurrentUser(context.Background()); login != "brushmtnai" || err != nil {
		t.Errorf("getCurrentUser() = %q, %v; want brushmtnai", login, err)
	}
	if n := len(fs.Requests()); n != 1 {
		t.Errorf("%d API requests, want 1: the user is cached", n)
	}
	if got := fs.Requests()[0]; got.Method != "GET" || got.Path != "/api/v4/user" {
		t.Errorf("request = %s %s, want GET /api/v4/user", got.Method, got.Path)
	}
}

func TestConnectivityCheckBypass(t *testing.T) {
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "gitlab")
	fs := newFixtureServer(t)
	if err := ConnectivityCheck(); err != nil {
		t.Errorf("ConnectivityCheck() with the bypass = %v, want nil", err)
	}
	if n := len(fs.Requests()); n != 0 {
		t.Errorf("%d API requests with the bypass, want 0", n)
	}
}

func TestConnectivityCheckNoClient(t *testing.T) {
	orig := client
	client = nil
	t.Cleanup(func() { client = orig })
	if err := ConnectivityCheck(); err == nil {
		t.Error("ConnectivityCheck() without a client = nil, want an error")
	}
}

// A fine-grained token missing a permission gets a 403 that names it; the
// connectivity check fails and the description is extracted for the log.
func TestConnectivityCheckInsufficientScope(t *testing.T) {
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "")
	const desc = "This endpoint requires a fine-grained personal access token with the following permissions: [User: Read]."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"insufficient_granular_scope","error_description":"` + desc + `"}`))
	}))
	defer srv.Close()
	c, err := gitlab.NewClient("tok", gitlab.WithBaseURL(srv.URL), gitlab.WithoutRetries())
	if err != nil {
		t.Fatal(err)
	}
	origClient := client
	client = c
	userMu.Lock()
	origUser := currentUser
	currentUser = ""
	userMu.Unlock()
	t.Cleanup(func() {
		client = origClient
		userMu.Lock()
		currentUser = origUser
		userMu.Unlock()
	})

	err = ConnectivityCheck()
	if err == nil {
		t.Fatal("ConnectivityCheck() = nil, want the 403")
	}
	if got := errorDescription(err); got != desc {
		t.Errorf("errorDescription() = %q, want %q", got, desc)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not name the status", err)
	}
}

func TestRepoHosts(t *testing.T) {
	orig := webBaseURL
	t.Cleanup(func() { webBaseURL = orig })
	tests := []struct {
		base string
		want []string
	}{
		{"https://gitlab.com", []string{"gitlab.com"}},
		{"https://gitlab.example.com:8443", []string{"gitlab.example.com"}},
		{"https://example.com/gitlab", []string{"example.com/gitlab"}},
		{"not a url", nil},
	}
	for _, tc := range tests {
		webBaseURL = tc.base
		if got := (Provider{}).RepoHosts(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("RepoHosts() for %q = %v, want %v", tc.base, got, tc.want)
		}
	}
}

func TestEnabled(t *testing.T) {
	t.Setenv("GITLAB_TOKEN", "")
	if (Provider{}).Enabled() {
		t.Error("Enabled() without GITLAB_TOKEN = true")
	}
	t.Setenv("GITLAB_TOKEN", "glpat-test")
	if !(Provider{}).Enabled() {
		t.Error("Enabled() with GITLAB_TOKEN = false")
	}
}

// ValidateConfig builds the client; until then there is none, so an unlisted
// GitLab never builds one.
func TestValidateConfigBuildsClient(t *testing.T) {
	origClient, origWeb := client, webBaseURL
	t.Cleanup(func() { client, webBaseURL = origClient, origWeb })
	t.Setenv("GITLAB_TOKEN", "glpat-test")
	t.Setenv("GITLAB_CA_FILE", "")
	t.Setenv("CI_SERVER_TLS_CA_FILE", "")

	client, webBaseURL = nil, "https://gitlab.example.com"
	if err := (Provider{}).ValidateConfig(); err != nil || client == nil {
		t.Fatalf("ValidateConfig() = %v, client %v; want a client", err, client)
	}
	if got := client.BaseURL().String(); got != "https://gitlab.example.com/api/v4/" {
		t.Errorf("client base URL = %q", got)
	}

	client, webBaseURL = nil, "ftp://gitlab.example.com"
	if err := (Provider{}).ValidateConfig(); err == nil || client != nil {
		t.Errorf("ValidateConfig() with a bad base URL = %v, client %v; want an error and no client", err, client)
	}
	t.Setenv("GITLAB_CA_FILE", filepath.Join(t.TempDir(), "missing.pem"))
	webBaseURL = "https://gitlab.example.com"
	if err := (Provider{}).ValidateConfig(); err == nil || client != nil {
		t.Errorf("ValidateConfig() with a missing CA file = %v, client %v; want an error and no client", err, client)
	}
}
