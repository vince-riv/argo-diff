package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/v90/github"
)

const testDataDir = "github_testdata"
const payloadUser = "payload-user.json"
const payloadApp = "payload-app.json"
const payloadPr1Comments = "payload-pr-1-comments.json"
const payloadPr2Comments = "payload-pr-2-comments.json"
const payloadPr3Comments = "payload-pr-3-comments.json"
const payloadPr4Comments = "payload-pr-4-comments.json"
const payloadPr1CreateComment = "payload-pr-1-create-comment.json"
const payloadPr2CreateComment = "payload-pr-2-create-comment.json"

// const payloadPr3UpdateComment = "payload-pr-3-update-comment.json"
const payloadPatchComment = "payload-pr-patch-comment.json"
const payloadPullRequest = "payload-pr-get.json"

const prHeadSha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func readFileToByteArray(fileName string) ([]byte, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("Error getting current working directory: %w", err)
	}

	filePath := filepath.Join(workingDir, testDataDir, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, filePath, fmt.Errorf("error reading file '%s': %w", filePath, err)
	}

	return data, filePath, nil
}

func jsonFieldExtract(srcField string, src []byte, destField string, dest []byte) ([]byte, error) {
	var srcData, destData map[string]interface{}
	err := json.Unmarshal(src, &srcData)
	if err != nil {
		return []byte{}, err
	}
	err = json.Unmarshal(dest, &destData)
	if err != nil {
		return []byte{}, err
	}
	destData[destField] = srcData[srcField]

	ret, err := json.Marshal(destData)
	if err != nil {
		return []byte{}, err
	}
	return ret, nil
}

func newHttpTestServer(t *testing.T) *httptest.Server {
	// TODO - unclutter this mess
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode := http.StatusNotFound
		payload := []byte(`404 Page Not Found`)
		var filePath string
		var err error
		//println("r.URL.Path: " + r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/repos/vince-riv/argo-diff/issues/comments/") {
			if r.Method == "PATCH" {
				var reqBody []byte
				reqBody, err = io.ReadAll(r.Body)
				urlPathParts := strings.Split(r.URL.Path, "/")
				if urlPathParts[6] == "" {
					statusCode = http.StatusInternalServerError
					t.Errorf("Bad URL Path: %s", r.URL.Path)
				} else {
					statusCode = http.StatusOK
					payload, filePath, err = readFileToByteArray(payloadPatchComment)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("readFileToByteArray() failed: %s", err)
						return
					}
					payload = bytes.ReplaceAll(payload, []byte("%%_COMMENT_ID_%%"), []byte(urlPathParts[6]))
					payload, err = jsonFieldExtract("body", reqBody, "body", payload)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("jsonFieldExtract() failed: %s", err)
						return
					}
				}
			} else {
				statusCode = http.StatusMethodNotAllowed
			}
		} else if strings.HasPrefix(r.URL.Path, "/repos/vince-riv/argo-diff/pulls/") {
			if r.Method == "GET" {
				urlPathParts := strings.Split(r.URL.Path, "/")
				if urlPathParts[5] == "" {
					statusCode = http.StatusInternalServerError
					t.Errorf("Bad URL Path: %s", r.URL.Path)
				} else {
					statusCode = http.StatusOK
					payload, filePath, err = readFileToByteArray(payloadPullRequest)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("readFileToByteArray() failed: %s", err)
						return
					}
					payload = bytes.ReplaceAll(payload, []byte("%%_PR_NUM_%%"), []byte(urlPathParts[5]))
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						t.Errorf("jsonFieldExtract() failed: %s", err)
						return
					}
				}
			} else {
				statusCode = http.StatusMethodNotAllowed
			}
		} else {
			switch r.URL.Path {
			case "/user":
				statusCode = http.StatusOK
				payload, filePath, err = readFileToByteArray(payloadUser)
			case "/repos/vince-riv/argo-diff/issues/1/comments":
				switch r.Method {
				case "GET":
					statusCode = http.StatusOK
					payload, filePath, err = readFileToByteArray(payloadPr1Comments)
				case "POST":
					statusCode = http.StatusCreated
					payload, filePath, err = readFileToByteArray(payloadPr1CreateComment)
				default:
					statusCode = http.StatusMethodNotAllowed
				}
			case "/repos/vince-riv/argo-diff/issues/2/comments":
				switch r.Method {
				case "GET":
					statusCode = http.StatusOK
					payload, filePath, err = readFileToByteArray(payloadPr2Comments)
				case "POST":
					statusCode = http.StatusCreated
					payload, filePath, err = readFileToByteArray(payloadPr2CreateComment)
				default:
					statusCode = http.StatusMethodNotAllowed
				}
			case "/repos/vince-riv/argo-diff/issues/3/comments":
				statusCode = http.StatusOK
				payload, filePath, err = readFileToByteArray(payloadPr3Comments)
			case "/repos/vince-riv/argo-diff/issues/4/comments":
				statusCode = http.StatusOK
				payload, filePath, err = readFileToByteArray(payloadPr4Comments)
			default:
				t.Errorf("Mock server not configured to serve path %s", r.URL.Path)
			}
		}
		if err != nil {
			t.Errorf("Failed to load %s: %s", filePath, err)
			statusCode = http.StatusInternalServerError
		}
		w.WriteHeader(statusCode)
		w.Write(payload)
	}))
	return newServer
}

func TestCommentNoExistingComments(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 1)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) > 0 {
		t.Error("Expected no existing comment")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 1, prHeadSha, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 1111111111 {
		t.Error("Comment ID doesn't match")
	}
}

func TestCommentExistingDifferentUser(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 2)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) > 0 {
		t.Error("Expected no existing comment")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 2, prHeadSha, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 2222222222 {
		t.Error("Comment ID doesn't match")
	}
}

// TestCommentExistingDifferentUserBypass mirrors TestCommentExistingDifferentUser
// but with ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS=github set. This is the regression
// case for PR #287: an install token that 403s on GET /user should still let
// argo-diff find and update its own prior comment, matching by marker alone
// (same as isGithubAction), instead of posting a duplicate.
func TestCommentExistingDifferentUserBypass(t *testing.T) {
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "github")

	server := newHttpTestServer(t)
	defer server.Close()
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 2)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) != 1 {
		t.Fatalf("Expected 1 existing comment matched by marker, got %d", len(c))
	}
	if *c[0].ID != 1828467122 {
		t.Errorf("Comment ID doesn't match: got %d, want 1828467122", *c[0].ID)
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 2, prHeadSha, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 1828467122 {
		t.Errorf("Comment() should have updated the existing comment, got ID %d", *comments[0].ID)
	}
}

func TestCommentExisting(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 3)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) != 1 {
		t.Error("Could not find existing comment")
	} else if *c[0].ID != 3333333333 {
		t.Error("Comment ID doesn't match")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 3, prHeadSha, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 3333333333 {
		t.Error("Comment ID doesn't match")
	}
}

func TestCommentExistingMulti(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 4)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) != 2 {
		t.Error("Could not find existing comment")
	} else if *c[0].ID != 4444444222 && *c[1].ID != 4444444333 {
		t.Error("Unexpected issue commit IDs")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 4, prHeadSha, []string{"argo-diff test comment update"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 4444444222 {
		t.Errorf("1st Comment ID doesn't match 4444444222: %d", *comments[0].ID)
	}
	if !strings.Contains(*comments[0].Body, "argo-diff test comment update") {
		t.Errorf("1st Comment body doesn't match 'argo-diff test comment update': %s", *comments[0].Body)
	}
	if *comments[1].ID != 4444444333 {
		t.Errorf("2nd Comment ID doesn't match 4444444333: %d", *comments[1].ID)
	}
	if !strings.Contains(*comments[1].Body, "[Outdated argo-diff content]") {
		t.Errorf("1st Comment body doesn't match '[Outdated argo-diff content]': %s", *comments[1].Body)
	}
}

func TestCommentExistingMultiNoComment(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}

	c, err := getExistingComments(context.Background(), "vince-riv", "argo-diff", 4)
	if err != nil {
		t.Errorf("getExistingComments() failed: %s", err)
	}
	if len(c) != 2 {
		t.Error("Could not find existing comment")
	} else if *c[0].ID != 4444444222 && *c[1].ID != 4444444333 {
		t.Error("Unexpected issue commit IDs")
	}

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 4, prHeadSha, []string{})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if *comments[0].ID != 4444444222 {
		t.Errorf("1st Comment ID doesn't match 4444444222: %d", *comments[0].ID)
	}
	if !strings.Contains(*comments[0].Body, "[Outdated argo-diff content]") {
		t.Errorf("1st Comment body doesn't match '[Outdated argo-diff content]': %s", *comments[1].Body)
	}
	if *comments[1].ID != 4444444333 {
		t.Errorf("2nd Comment ID doesn't match 4444444333: %d", *comments[1].ID)
	}
	if !strings.Contains(*comments[1].Body, "[Outdated argo-diff content]") {
		t.Errorf("2nd Comment body doesn't match '[Outdated argo-diff content]': %s", *comments[1].Body)
	}
}

// TestGetCommentUserConcurrent is the regression test for issue #296: concurrent
// calls to getCommentUser() (as happen when the webhook server processes two PR
// events at once) must populate commentLogin exactly once, and every read of it
// must be race-free under `go test -race`.
func TestGetCommentUserConcurrent(t *testing.T) {
	var userCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("Mock server not configured to serve path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		atomic.AddInt32(&userCalls, 1)
		payload, filePath, err := readFileToByteArray(payloadUser)
		if err != nil {
			t.Errorf("Failed to load %s: %s", filePath, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(payload)
	}))
	defer server.Close()

	origCommentClient := commentClient
	origIsApp := commentClientIsApp
	origCommentLogin := commentLogin
	defer func() {
		commentClient = origCommentClient
		commentClientIsApp = origIsApp
		mux.Lock()
		commentLogin = origCommentLogin
		mux.Unlock()
	}()

	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}
	commentClientIsApp = false

	// Reset the singleton so this test actually exercises the populate-once path,
	// regardless of what earlier tests in this package already cached.
	mux.Lock()
	commentLogin = ""
	mux.Unlock()

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = getCommentUser(context.Background())
			// Concurrent, lock-guarded reads alongside the concurrent writes above -
			// this is what go test -race catches if the guarding regresses.
			_ = getCommentLogin()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("getCommentUser() goroutine %d failed: %s", i, err)
		}
	}
	if got := getCommentLogin(); got == "" {
		t.Error("expected commentLogin to be populated after concurrent getCommentUser() calls")
	}
	if calls := atomic.LoadInt32(&userCalls); calls != 1 {
		t.Errorf("expected exactly 1 call to GET /user, got %d", calls)
	}
}

func TestCommentNotHead(t *testing.T) {
	server := newHttpTestServer(t)
	defer server.Close()
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatalf("Failed to create github client: %s", err)
	}

	prHeadShaOld := "1111111111111111111111111111111111111111"

	comments, err := Comment(context.Background(), "vince-riv", "argo-diff", 1, prHeadShaOld, []string{"argo-diff test comment"})
	if err != nil {
		t.Errorf("Comment() failed: %s", err)
	}
	if len(comments) > 0 {
		t.Error("Not expecting to comment")
	}
}

// TestGetCommentUserApp is the regression case for #290: a GitHub App whose display name
// ("ArgoDiff Prod") isn't slug-shaped must still derive commentLogin from the App's slug
// ("argodiff-prod"), since that's what GitHub actually uses as the bot's login. It also covers
// the slug-empty fallback to name, and the case where both are empty (must error, not cache
// a bare "[bot]" login).
func TestGetCommentUserApp(t *testing.T) {
	cases := []struct {
		name      string
		fixture   string
		wantLogin string
		wantErr   bool
	}{
		{name: "slug present", fixture: payloadApp, wantLogin: "argodiff-prod[bot]"},
		{name: "empty slug falls back to name", fixture: "payload-app-empty-slug.json", wantLogin: "ArgoDiff Prod[bot]"},
		{name: "empty slug and name errors", fixture: "payload-app-empty.json", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/app" {
					t.Errorf("Mock server not configured to serve path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				payload, filePath, err := readFileToByteArray(tc.fixture)
				if err != nil {
					t.Errorf("readFileToByteArray(%s) failed: %s", filePath, err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write(payload)
			}))
			defer appServer.Close()
			baseURL := appServer.URL + "/"

			origCommentClient := commentClient
			origIsApp := commentClientIsApp
			origAppsClient := appsClient
			origCommentLogin := commentLogin
			defer func() {
				commentClient = origCommentClient
				commentClientIsApp = origIsApp
				appsClient = origAppsClient
				mux.Lock()
				commentLogin = origCommentLogin
				mux.Unlock()
			}()

			// getCommentUser() requires commentClient to be set regardless of which path
			// (App or PAT) it takes, so this test must set it independently of test order.
			var err error
			commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
			if err != nil {
				t.Fatalf("Failed to create github client: %s", err)
			}
			appsClient, err = github.NewClient(github.WithURLs(&baseURL, &baseURL))
			if err != nil {
				t.Fatalf("Failed to create github client: %s", err)
			}
			commentClientIsApp = true
			mux.Lock()
			commentLogin = ""
			mux.Unlock()

			err = getCommentUser(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("getCommentUser() succeeded, want error (commentLogin ended up %q)", commentLogin)
				}
				return
			}
			if err != nil {
				t.Fatalf("getCommentUser() failed: %s", err)
			}
			if commentLogin != tc.wantLogin {
				t.Errorf("commentLogin = %q, want %q", commentLogin, tc.wantLogin)
			}
		})
	}
}
