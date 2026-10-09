package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/vince-riv/argo-diff/internal/scm"
)

const statusSHA = "bb6a46bd76030008e38a5a986cfd7c67f48a8687"

func TestStatusState(t *testing.T) {
	tests := map[scm.Status]string{
		scm.StatusPending: "pending",
		scm.StatusSuccess: "success",
		scm.StatusFailure: "failed",
		scm.StatusError:   "failed",
	}
	for in, want := range tests {
		got, err := statusState(in)
		if err != nil || string(got) != want {
			t.Errorf("statusState(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := statusState("failure-ish"); err == nil {
		t.Error("statusState() of an unknown state = nil error")
	}
}

func TestTruncateDescription(t *testing.T) {
	if got := truncateDescription("short"); got != "short" {
		t.Errorf("truncateDescription(short) = %q", got)
	}
	exact := strings.Repeat("a", statusDescriptionMaxLen)
	if got := truncateDescription(exact); got != exact {
		t.Error("a 255-character description was cut")
	}
	// 300 two-byte characters: the limit counts characters, not bytes
	got := truncateDescription(strings.Repeat("é", 300))
	if n := utf8.RuneCountInString(got); n != statusDescriptionMaxLen {
		t.Errorf("truncated to %d characters, want %d", n, statusDescriptionMaxLen)
	}
	if !strings.HasSuffix(got, "...") || !utf8.ValidString(got) {
		t.Errorf("truncated description %q lacks the ... suffix or is not valid UTF-8", got)
	}
}

// statusRequest is the form GitLab receives for a commit status.
type statusRequest struct {
	State       string `json:"state"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func TestSetStatus(t *testing.T) {
	origName := statusName
	t.Cleanup(func() { statusName = origName })
	statusName = "argo-diff/ctx"
	fs := newFixtureServer(t, fixtureRoute{Method: "POST", Path: apiPath("statuses/" + statusSHA), Fixture: "status-failed"})

	desc := strings.Repeat("d", 300)
	if err := (Provider{}).SetStatus(context.Background(), testRepo, statusSHA, scm.StatusFailure, desc, false); err != nil {
		t.Fatal(err)
	}
	var got statusRequest
	if err := json.Unmarshal(fs.Requests()[0].Body, &got); err != nil {
		t.Fatal(err)
	}
	if got.State != "failed" || got.Name != "argo-diff/ctx" || len(got.Description) != statusDescriptionMaxLen {
		t.Errorf("request = %+v (description %d chars); want failed, argo-diff/ctx, 255 chars", got, len(got.Description))
	}
}

// GitLab's 400 for a bad state comes back as an error.
func TestSetStatusAPIError(t *testing.T) {
	newFixtureServer(t, fixtureRoute{Method: "POST", Path: apiPath("statuses/" + statusSHA), Fixture: "status-badstate"})
	err := Provider{}.SetStatus(context.Background(), testRepo, statusSHA, scm.StatusSuccess, "ok", false)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("SetStatus() = %v, want the 400", err)
	}
}

func TestSetStatusDryRun(t *testing.T) {
	fs := newFixtureServer(t)
	if err := (Provider{}).SetStatus(context.Background(), testRepo, statusSHA, scm.StatusPending, "diff in progress", true); err != nil {
		t.Errorf("SetStatus() dry run = %v", err)
	}
	if n := len(fs.Requests()); n != 0 {
		t.Errorf("%d requests in a dry run, want 0", n)
	}
}

func TestSetStatusUnknownState(t *testing.T) {
	fs := newFixtureServer(t)
	if err := (Provider{}).SetStatus(context.Background(), testRepo, statusSHA, "bogus", "", false); err == nil {
		t.Error("SetStatus() with an unknown state = nil error")
	}
	if n := len(fs.Requests()); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

// GitLab answers 400 when a pending status is set on a SHA whose status of the
// same name is still pending (review on #359). Setting pending again is then
// a no-op, not an error; any other state keeps the error. The body is derived
// from the message GitLab's CommitStatus state machine raises, not captured.
func TestSetStatusAlreadyPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Cannot transition status via :enqueue from :pending (Reason(s): Status cannot transition via \"enqueue\")"}`))
	}))
	defer srv.Close()
	c, err := gitlab.NewClient("tok", gitlab.WithBaseURL(srv.URL), gitlab.WithoutRetries())
	if err != nil {
		t.Fatal(err)
	}
	orig := client
	client = c
	t.Cleanup(func() { client = orig })

	if err := (Provider{}).SetStatus(context.Background(), testRepo, statusSHA, scm.StatusPending, "", false); err != nil {
		t.Errorf("SetStatus(pending) on an already-pending status = %v, want nil", err)
	}
	if err := (Provider{}).SetStatus(context.Background(), testRepo, statusSHA, scm.StatusSuccess, "", false); err == nil {
		t.Error("SetStatus(success) on the same 400 = nil, want the error")
	}
}
