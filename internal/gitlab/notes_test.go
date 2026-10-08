package gitlab

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/vince-riv/argo-diff/internal/comment"
	"github.com/vince-riv/argo-diff/internal/scm"
)

const capturedNoteID = 3974493431

func TestListComments(t *testing.T) {
	fs := newFixtureServer(t, fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1/notes"), Fixture: "mr-notes"})
	got, err := Provider{}.ListComments(context.Background(), testRepo, 1)
	if err != nil {
		t.Fatal(err)
	}
	// the fixture has four user notes and one system note, oldest first
	wantIDs := []int64{3974493431, 3974494261, 3974494396, 3974495256}
	var ids []int64
	for _, c := range got {
		ids = append(ids, c.ID)
		if c.Author != "brushmtnai" {
			t.Errorf("note %d author = %q, want brushmtnai", c.ID, c.Author)
		}
		if strings.HasPrefix(c.Body, "added 1 commit") {
			t.Errorf("system note %d was not left out", c.ID)
		}
	}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Errorf("ListComments() IDs = %v, want %v", ids, wantIDs)
	}
	if want := fs.URL + "/vrivellino/argo-diff/-/merge_requests/1#note_3974493431"; got[0].URL != want {
		t.Errorf("URL = %q, want %q", got[0].URL, want)
	}
	q := fs.Requests()[0].Query
	for _, p := range []string{"order_by=created_at", "sort=asc", "per_page=100"} {
		if !strings.Contains(q, p) {
			t.Errorf("query %q lacks %s", q, p)
		}
	}
}

func TestCreateComment(t *testing.T) {
	fs := newFixtureServer(t, fixtureRoute{Method: "POST", Path: apiPath("merge_requests/1/notes"), Fixture: "note-create"})
	got, err := Provider{}.CreateComment(context.Background(), testRepo, 1, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != capturedNoteID || got.Author != "brushmtnai" {
		t.Errorf("CreateComment() = %+v", got)
	}
	if b := requestBody(t, fs.Requests()[0]); b != "hello" {
		t.Errorf("posted body = %q, want hello", b)
	}
}

func TestUpdateComment(t *testing.T) {
	fs := newFixtureServer(t, fixtureRoute{Method: "PUT", Path: apiPath("merge_requests/1/notes/3974493431"), Fixture: "note-update"})
	got, err := Provider{}.UpdateComment(context.Background(), testRepo, 1, capturedNoteID, "new body")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != capturedNoteID || !strings.HasPrefix(got.Body, "[Outdated argo-diff content]") {
		t.Errorf("UpdateComment() = %+v", got)
	}
	if b := requestBody(t, fs.Requests()[0]); b != "new body" {
		t.Errorf("posted body = %q, want new body", b)
	}
}

// GitLab rejects a note over 1 MiB with a 400; the error comes back.
func TestCreateCommentTooLong(t *testing.T) {
	newFixtureServer(t, fixtureRoute{Method: "POST", Path: apiPath("merge_requests/1/notes"), Fixture: "note-create-too-long"})
	_, err := Provider{}.CreateComment(context.Background(), testRepo, 1, "x")
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Errorf("CreateComment() = %v, want GitLab's too long error", err)
	}
}

func TestCurrentUser(t *testing.T) {
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "")
	newFixtureServer(t, fixtureRoute{Method: "GET", Path: "/api/v4/user", Fixture: "user"})
	if login, err := (Provider{}).CurrentUser(context.Background()); login != "brushmtnai" || err != nil {
		t.Errorf("CurrentUser() = %q, %v; want brushmtnai", login, err)
	}
}

// With the gitlab check bypassed, notes are matched by marker alone.
func TestCurrentUserBypass(t *testing.T) {
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "gitlab")
	fs := newFixtureServer(t)
	if login, err := (Provider{}).CurrentUser(context.Background()); login != "" || err != nil {
		t.Errorf("CurrentUser() = %q, %v; want \"\", nil", login, err)
	}
	if n := len(fs.Requests()); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestDialect(t *testing.T) {
	if d := (Provider{}).Dialect(); d != comment.GitLab || d.HardMax != 1048576 {
		t.Errorf("Dialect() = %+v, want comment.GitLab with a 1 MiB HardMax", d)
	}
}

// scm.PostComments over the GitLab primitives: the head check passes, the
// captured argo-diff note is edited with the first body, and the second body
// is created.
func TestPostComments(t *testing.T) {
	t.Setenv("ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS", "")
	fs := newFixtureServer(t,
		fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1"), Fixture: "mr"},
		fixtureRoute{Method: "GET", Path: "/api/v4/user", Fixture: "user"},
		fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1/notes"), Fixture: "mr-notes"},
		fixtureRoute{Method: "PUT", Path: apiPath("merge_requests/1/notes/3974493431"), Fixture: "note-update"},
		fixtureRoute{Method: "POST", Path: apiPath("merge_requests/1/notes"), Fixture: "note-create"},
	)
	posted, err := scm.PostComments(context.Background(), Provider{}, testRepo, 1, "b27a067108f25866d65fe6412ac9ab66c22fe731", []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if len(posted) != 2 {
		t.Fatalf("posted %d notes, want 2", len(posted))
	}
	var calls []string
	for _, r := range fs.Requests() {
		calls = append(calls, r.Method+" "+strings.TrimPrefix(r.Path, apiPath("")))
	}
	want := []string{
		"GET merge_requests/1",
		"GET /api/v4/user",
		"GET merge_requests/1/notes",
		"PUT merge_requests/1/notes/3974493431",
		"POST merge_requests/1/notes",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
	reqs := fs.Requests()
	for i, body := range []string{"first", "second"} {
		got := requestBody(t, reqs[3+i])
		if !strings.Contains(got, body) || !strings.Contains(got, comment.Identifier()) {
			t.Errorf("request %d body = %q, want %q wrapped with the marker", 3+i, got, body)
		}
	}
}

// A stale head SHA posts nothing.
func TestPostCommentsNotHead(t *testing.T) {
	fs := newFixtureServer(t, fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1"), Fixture: "mr"})
	posted, err := scm.PostComments(context.Background(), Provider{}, testRepo, 1, "0000000", []string{"first"})
	if err != nil || len(posted) != 0 {
		t.Errorf("PostComments() = %v, %v; want nothing posted", posted, err)
	}
	if n := len(fs.Requests()); n != 1 {
		t.Errorf("%d requests, want only the MR lookup", n)
	}
}

// requestBody decodes the "body" field of a note create/update request.
func requestBody(t *testing.T, r recordedRequest) string {
	t.Helper()
	var v struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal(r.Body, &v); err != nil {
		t.Fatalf("decoding request body %q: %v", r.Body, err)
	}
	return v.Body
}
