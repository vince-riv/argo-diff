package gitlab

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/vince-riv/argo-diff/internal/comment"
	"github.com/vince-riv/argo-diff/internal/scm"
)

// testProject is the captured fixtures' project, as the API addresses it.
const testProject = "vrivellino%2Fargo-diff"

// fixtureMarker is the identifier marker in the captured notes. serveFixture
// swaps it for comment.Identifier(), which init() derives from the
// environment, so tests find the captured argo-diff note as their own.
const fixtureMarker = "<!-- comment produced by argo-diff[test] -->"

// testRepo is the captured fixtures' project.
var testRepo = scm.RepoRef{Owner: "vrivellino", Name: "argo-diff"}

// apiPath is the escaped API path of a project route, eg:
// apiPath("merge_requests/1").
func apiPath(route string) string {
	return "/api/v4/projects/" + testProject + "/" + route
}

// fixtureRoute answers one request with a captured API fixture:
// gitlab_testdata/api/<Fixture>.json, with the status line and headers of
// <Fixture>.headers when that file exists.
type fixtureRoute struct {
	Method  string
	Path    string // escaped path, eg: apiPath("merge_requests/1")
	Query   string // when set, the request's raw query must contain it
	Fixture string
}

// recordedRequest is one request the fixture server received.
type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   []byte
}

// fixtureServer serves routes and records every request. A request no route
// matches fails the test.
type fixtureServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

func (s *fixtureServer) Requests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

// newFixtureServer starts a server for routes and points the package client
// at it for the duration of the test. The cached current user is reset too.
func newFixtureServer(t *testing.T, routes ...fixtureRoute) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fs.mu.Lock()
		fs.requests = append(fs.requests, recordedRequest{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, Body: body})
		fs.mu.Unlock()
		for _, rt := range routes {
			if rt.Method == r.Method && rt.Path == r.URL.EscapedPath() && strings.Contains(r.URL.RawQuery, rt.Query) {
				serveFixture(t, w, rt.Fixture, fs.URL)
				return
			}
		}
		t.Errorf("unexpected request %s %s?%s", r.Method, r.URL.EscapedPath(), r.URL.RawQuery)
		http.Error(w, "no fixture", http.StatusNotImplemented)
	}))
	t.Cleanup(fs.Close)

	c, err := gitlab.NewClient("test-token", gitlab.WithBaseURL(fs.URL), gitlab.WithoutRetries())
	if err != nil {
		t.Fatal(err)
	}
	origClient, origWeb := client, webBaseURL
	client, webBaseURL = c, fs.URL
	userMu.Lock()
	origUser := currentUser
	currentUser = ""
	userMu.Unlock()
	t.Cleanup(func() {
		client, webBaseURL = origClient, origWeb
		userMu.Lock()
		currentUser = origUser
		userMu.Unlock()
	})
	return fs
}

// serveFixture writes gitlab_testdata/api/<name>.json, with the status and
// headers from <name>.headers (gitlab.com URLs in them point at serverURL) and
// fixtureMarker replaced by comment.Identifier().
// Without a headers file the status is 200, or 400 for fixtures that hold only
// an error body.
func serveFixture(t *testing.T, w http.ResponseWriter, name, serverURL string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("gitlab_testdata", "api", name+".json"))
	if err != nil {
		t.Errorf("reading fixture %s: %v", name, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body = bytes.ReplaceAll(body, []byte(fixtureMarker), []byte(comment.Identifier()))
	status := http.StatusOK
	headers, err := os.ReadFile(filepath.Join("gitlab_testdata", "api", name+".headers"))
	switch {
	case err == nil:
		sc := bufio.NewScanner(bytes.NewReader(headers))
		if sc.Scan() { // status line, eg: "HTTP/2 201"
			if f := strings.Fields(sc.Text()); len(f) >= 2 {
				status, _ = strconv.Atoi(f[1])
			}
		}
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), ":")
			if !ok {
				continue
			}
			v = strings.ReplaceAll(strings.TrimSpace(v), "https://gitlab.com", serverURL)
			w.Header().Set(k, v)
		}
	case strings.HasPrefix(name, "note-create-too-long"):
		status = http.StatusBadRequest
		w.Header().Set("Content-Type", "application/json")
	default:
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
