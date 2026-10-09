package gitlab

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/vince-riv/argo-diff/internal/scm"
)

func TestGetChangeRequest(t *testing.T) {
	fs := newFixtureServer(t, fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1"), Fixture: "mr"})
	got, err := Provider{}.GetChangeRequest(context.Background(), testRepo, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := scm.ChangeRequest{
		Number:  1,
		HeadSHA: "b27a067108f25866d65fe6412ac9ab66c22fe731",
		HeadRef: "spike/phase0-gitlab",
		BaseRef: "main",
	}
	if got != want {
		t.Errorf("GetChangeRequest() = %+v, want %+v", got, want)
	}
	if n := len(fs.Requests()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

// A project in nested groups is addressed by its URL-encoded full path.
func TestGetChangeRequestNestedGroup(t *testing.T) {
	fs := newFixtureServer(t, fixtureRoute{Method: "GET", Path: "/api/v4/projects/group%2Fsub%2Fproject/merge_requests/7", Fixture: "mr"})
	repo := scm.RepoRef{Owner: "group/sub", Name: "project"}
	if _, err := (Provider{}).GetChangeRequest(context.Background(), repo, 7); err != nil {
		t.Fatal(err)
	}
	if n := len(fs.Requests()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestGetChangeRequestError(t *testing.T) {
	newFixtureServer(t, fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1"), Fixture: "status-badstate"})
	if _, err := (Provider{}).GetChangeRequest(context.Background(), testRepo, 1); err == nil {
		t.Error("GetChangeRequest() on a 400 = nil error")
	}
}

// The diffs span two pages; page 1 holds a rename, which yields both paths.
func TestListChangedFiles(t *testing.T) {
	fs := newFixtureServer(t,
		fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1/diffs"), Query: "page=2", Fixture: "mr-diffs-p2"},
		fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1/diffs"), Fixture: "mr-diffs-p1"},
	)
	got, err := Provider{}.ListChangedFiles(context.Background(), testRepo, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"post-local.sh", "scripts/post-local.sh",
		"spike/a.txt", "spike/b.txt",
		"spike/c.txt", "spike/d.txt", ".gitlab-ci.yml",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListChangedFiles() = %v, want %v", got, want)
	}
	reqs := fs.Requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2 (one per page)", len(reqs))
	}
	if !strings.Contains(reqs[0].Query, "per_page=100") {
		t.Errorf("first request query %q does not ask for 100 per page", reqs[0].Query)
	}
	if !strings.Contains(reqs[1].Query, "page=2") {
		t.Errorf("second request query %q does not ask for page 2", reqs[1].Query)
	}
}

func TestListChangedFilesError(t *testing.T) {
	newFixtureServer(t, fixtureRoute{Method: "GET", Path: apiPath("merge_requests/1/diffs"), Fixture: "status-badstate"})
	if _, err := (Provider{}).ListChangedFiles(context.Background(), testRepo, 1); err == nil {
		t.Error("ListChangedFiles() on a 400 = nil error")
	}
}

func TestMergeRequestNoClient(t *testing.T) {
	orig := client
	client = nil
	t.Cleanup(func() { client = orig })
	if _, err := (Provider{}).GetChangeRequest(context.Background(), testRepo, 1); err == nil {
		t.Error("GetChangeRequest() without a client = nil error")
	}
	if _, err := (Provider{}).ListChangedFiles(context.Background(), testRepo, 1); err == nil {
		t.Error("ListChangedFiles() without a client = nil error")
	}
}
