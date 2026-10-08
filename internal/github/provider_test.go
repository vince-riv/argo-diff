package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-github/v92/github"
)

// Outside Actions and the bypass, an empty login must be an error: to
// scm.ExistingComments it means "match any author".
func TestCurrentUserRejectsEmptyLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"login": "", "id": 1}`))
	}))
	defer server.Close()

	origClient, origIsApp, origAction := commentClient, commentClientIsApp, isGithubAction
	mux.Lock()
	origLogin := commentLogin
	commentLogin = ""
	mux.Unlock()
	t.Cleanup(func() {
		commentClient, commentClientIsApp, isGithubAction = origClient, origIsApp, origAction
		mux.Lock()
		commentLogin = origLogin
		mux.Unlock()
	})
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "")
	baseURL := server.URL + "/"
	var err error
	commentClient, err = github.NewClient(github.WithAuthToken("test1234"), github.WithURLs(&baseURL, &baseURL))
	if err != nil {
		t.Fatal(err)
	}
	commentClientIsApp, isGithubAction = false, false

	if login, err := (Provider{}).CurrentUser(context.Background()); err == nil {
		t.Errorf("CurrentUser() = %q, nil; want an error for an empty login", login)
	}

	// the marker-only cases still return "" with no error
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "github")
	if login, err := (Provider{}).CurrentUser(context.Background()); login != "" || err != nil {
		t.Errorf("CurrentUser() with the bypass = %q, %v; want \"\", nil", login, err)
	}
}
