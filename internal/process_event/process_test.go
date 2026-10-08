package process_event

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/comment"
	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

const headSha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type statusCall struct {
	state scm.Status
	desc  string
	sha   string
}

// fakeProvider is an in-memory scm.Provider. It records the statuses and
// comments ProcessCodeChange sends, and keeps posted comments so a second run
// can find and reuse them.
type fakeProvider struct {
	mu       sync.Mutex
	dialect  comment.Dialect
	cr       scm.ChangeRequest
	crErr    error
	files    []string
	statuses []statusCall
	comments []scm.Comment
	nextID   int64
	creates  int
	updates  int
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		dialect: comment.GitHub,
		cr:      scm.ChangeRequest{Number: 7, HeadSHA: headSha, HeadRef: "feature", BaseRef: "main"},
		files:   []string{"apps/web/values.yaml"},
	}
}

func (f *fakeProvider) Name() string             { return "fake" }
func (f *fakeProvider) Dialect() comment.Dialect { return f.dialect }

func (f *fakeProvider) GetChangeRequest(context.Context, scm.RepoRef, int) (scm.ChangeRequest, error) {
	return f.cr, f.crErr
}

func (f *fakeProvider) ListChangedFiles(context.Context, scm.RepoRef, int) ([]string, error) {
	return f.files, nil
}

func (f *fakeProvider) SetStatus(_ context.Context, _ scm.RepoRef, sha string, state scm.Status, desc string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses = append(f.statuses, statusCall{state: state, desc: desc, sha: sha})
	return nil
}

func (f *fakeProvider) ListComments(context.Context, scm.RepoRef, int) ([]scm.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]scm.Comment(nil), f.comments...), nil
}

func (f *fakeProvider) CreateComment(_ context.Context, _ scm.RepoRef, _ int, body string) (scm.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.creates++
	c := scm.Comment{ID: f.nextID, Body: body, Author: "argo-bot"}
	f.comments = append(f.comments, c)
	return c, nil
}

func (f *fakeProvider) UpdateComment(_ context.Context, _ scm.RepoRef, _ int, id int64, body string) (scm.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	for i := range f.comments {
		if f.comments[i].ID == id {
			f.comments[i].Body = body
			return f.comments[i], nil
		}
	}
	return scm.Comment{}, fmt.Errorf("no comment %d", id)
}

func (f *fakeProvider) CurrentUser(context.Context) (string, error) { return "argo-bot", nil }

func (f *fakeProvider) lastStatus(t *testing.T) statusCall {
	t.Helper()
	if len(f.statuses) == 0 {
		t.Fatal("no commit status was set")
	}
	return f.statuses[len(f.statuses)-1]
}

// fakeArgo replaces the argocd seams for one test.
func fakeArgo(t *testing.T, apps []argocd.ApplicationResourcesWithChanges, notDiffed []string, err error) {
	t.Helper()
	origGet, origHas := getApplicationChanges, hasMatchingApplications
	t.Cleanup(func() { getApplicationChanges, hasMatchingApplications = origGet, origHas })
	getApplicationChanges = func(context.Context, webhook.EventInfo) ([]argocd.ApplicationResourcesWithChanges, []string, error) {
		return apps, notDiffed, err
	}
	hasMatchingApplications = func(context.Context, webhook.EventInfo) (bool, error) {
		return len(apps) > 0, err
	}
}

func app(name string, resources ...argocd.AppResource) argocd.ApplicationResourcesWithChanges {
	a := &argocd.Application{}
	a.Name = name
	a.Status.Sync.Status = "OutOfSync"
	a.Status.Health.Status = "Healthy"
	return argocd.ApplicationResourcesWithChanges{ArgoApp: a, ChangedResources: resources}
}

func deployment(name, diff string) argocd.AppResource {
	return argocd.AppResource{Group: "apps", Kind: "Deployment", Namespace: "prod", Name: name, DiffStr: diff}
}

const smallDiff = "--- live\n+++ new\n@@ -1 +1 @@\n-  replicas: 1\n+  replicas: 2\n"

func prEvent() webhook.EventInfo {
	return webhook.EventInfo{
		RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main",
		Sha: headSha, PrNum: 7, ChangeRef: "feature", BaseRef: "main",
	}
}

func run(p scm.Provider, evt webhook.EventInfo) error {
	var err error
	var wg sync.WaitGroup
	wg.Add(1)
	ProcessCodeChange(p, evt, false, &wg, &err)
	wg.Wait()
	return err
}

func TestProcessCodeChangeRejectsNonPR(t *testing.T) {
	fakeArgo(t, nil, nil, nil)
	f := newFakeProvider()
	evt := prEvent()
	evt.PrNum = 0
	if err := run(f, evt); err == nil {
		t.Error("ProcessCodeChange() accepted an event with no PR number")
	}
	if len(f.statuses) != 0 || f.creates != 0 {
		t.Error("ProcessCodeChange() called the provider for a non-PR event")
	}
}

func TestProcessCodeChangeWithChanges(t *testing.T) {
	fakeArgo(t, []argocd.ApplicationResourcesWithChanges{app("web", deployment("web", smallDiff)), app("quiet")}, nil, nil)
	f := newFakeProvider()
	if err := run(f, prEvent()); err != nil {
		t.Fatalf("ProcessCodeChange() err'd: %v", err)
	}
	if len(f.statuses) != 2 || f.statuses[0].state != scm.StatusPending {
		t.Errorf("statuses = %+v, want pending then a final one", f.statuses)
	}
	last := f.lastStatus(t)
	if last.state != scm.StatusSuccess || !strings.Contains(last.desc, "1 of 2 apps with changes") || last.sha != headSha {
		t.Errorf("final status = %+v, want success for 1 of 2 apps on %s", last, headSha)
	}
	if f.creates != 1 {
		t.Fatalf("created %d comments, want 1", f.creates)
	}
	body := f.comments[0].Body
	for _, want := range []string{"1 of 2 apps with changes", "web", "replicas: 2", comment.Identifier()} {
		if !strings.Contains(body, want) {
			t.Errorf("comment is missing %q:\n%s", want, body)
		}
	}

	// a second run reuses the comment rather than adding another
	if err := run(f, prEvent()); err != nil {
		t.Fatalf("second ProcessCodeChange() err'd: %v", err)
	}
	if f.creates != 1 || f.updates != 1 {
		t.Errorf("second run: %d creates, %d updates; want the existing comment edited", f.creates, f.updates)
	}
}

func TestProcessCodeChangeRefresh(t *testing.T) {
	fakeArgo(t, []argocd.ApplicationResourcesWithChanges{app("web", deployment("web", smallDiff))}, nil, nil)
	f := newFakeProvider()
	evt := prEvent()
	evt.Refresh, evt.Sha, evt.ChangeRef, evt.BaseRef = true, "", "", ""
	if err := run(f, evt); err != nil {
		t.Fatalf("ProcessCodeChange() err'd: %v", err)
	}
	// the head SHA came from GetChangeRequest()
	if last := f.lastStatus(t); last.sha != headSha {
		t.Errorf("status set on %q, want the refreshed head %q", last.sha, headSha)
	}
	if f.creates != 1 {
		t.Errorf("created %d comments, want 1", f.creates)
	}

	f = newFakeProvider()
	f.cr.BaseRef = ""
	if err := run(f, evt); err == nil || !strings.Contains(err.Error(), "empty branch information") {
		t.Errorf("ProcessCodeChange() with no base ref = %v, want an empty branch information error", err)
	}

	f = newFakeProvider()
	f.crErr = errors.New("404")
	if err := run(f, evt); err == nil {
		t.Error("ProcessCodeChange() swallowed a GetChangeRequest() error")
	}
	if len(f.statuses) != 0 {
		t.Error("ProcessCodeChange() set a status after a failed refresh")
	}
}

func TestProcessCodeChangeNoChangesClearsComments(t *testing.T) {
	fakeArgo(t, []argocd.ApplicationResourcesWithChanges{app("web", deployment("web", smallDiff))}, nil, nil)
	f := newFakeProvider()
	if err := run(f, prEvent()); err != nil {
		t.Fatalf("ProcessCodeChange() err'd: %v", err)
	}

	fakeArgo(t, []argocd.ApplicationResourcesWithChanges{app("web")}, nil, nil)
	if err := run(f, prEvent()); err != nil {
		t.Fatalf("ProcessCodeChange() err'd: %v", err)
	}
	if f.creates != 1 {
		t.Errorf("created %d comments, want no new one when nothing changed", f.creates)
	}
	if !strings.Contains(f.comments[0].Body, "[Outdated argo-diff content]") {
		t.Errorf("earlier comment = %q, want it outdated", f.comments[0].Body)
	}
	if last := f.lastStatus(t); last.state != scm.StatusSuccess {
		t.Errorf("final status = %+v, want success", last)
	}
}

func TestProcessCodeChangeAppError(t *testing.T) {
	broken := app("broken")
	broken.WarnStr = "rpc error: manifest generation failed"
	fakeArgo(t, []argocd.ApplicationResourcesWithChanges{broken, app("web", deployment("web", smallDiff))}, nil, nil)
	f := newFakeProvider()
	err := run(f, prEvent())
	if err == nil || !strings.Contains(err.Error(), "manifest generation failed") {
		t.Errorf("ProcessCodeChange() = %v, want the app's error", err)
	}
	if last := f.lastStatus(t); last.state != scm.StatusFailure {
		t.Errorf("final status = %+v, want failure", last)
	}
	if f.creates != 1 || !strings.Contains(f.comments[0].Body, "[!CAUTION]") {
		t.Errorf("want one comment carrying a CAUTION alert, got %d: %+v", f.creates, f.comments)
	}
}

func TestProcessCodeChangeDiffError(t *testing.T) {
	fakeArgo(t, nil, nil, errors.New("argocd unreachable"))
	f := newFakeProvider()
	if err := run(f, prEvent()); err == nil {
		t.Error("ProcessCodeChange() swallowed a GetApplicationChanges() error")
	}
	if last := f.lastStatus(t); last.state != scm.StatusError || !strings.Contains(last.desc, "argocd unreachable") {
		t.Errorf("final status = %+v, want error naming the cause", last)
	}
	if f.creates != 0 {
		t.Error("ProcessCodeChange() commented after a processing error")
	}
}

func TestProcessCodeChangeNotDiffed(t *testing.T) {
	fakeArgo(t, []argocd.ApplicationResourcesWithChanges{app("web", deployment("web", smallDiff))}, []string{"slow-app"}, nil)
	f := newFakeProvider()
	if err := run(f, prEvent()); err == nil {
		t.Error("ProcessCodeChange() reported success on a partial diff")
	}
	if last := f.lastStatus(t); last.state != scm.StatusFailure {
		t.Errorf("final status = %+v, want failure", last)
	}
	if f.creates != 1 || !strings.Contains(f.comments[0].Body, "slow-app") {
		t.Errorf("want one comment naming the app that was not diffed: %+v", f.comments)
	}
}

func TestProcessCodeChangeRequireAppMatch(t *testing.T) {
	t.Setenv("ARGO_DIFF_REQUIRE_APP_MATCH", "true")
	fakeArgo(t, nil, nil, nil)
	f := newFakeProvider()
	if err := run(f, prEvent()); err != nil {
		t.Errorf("ProcessCodeChange() err'd: %v", err)
	}
	if len(f.statuses) != 0 || f.creates != 0 {
		t.Error("ProcessCodeChange() reported on a change no application matches")
	}

	// a refresh request bypasses the match check
	evt := prEvent()
	evt.Refresh = true
	_ = run(f, evt)
	if len(f.statuses) == 0 {
		t.Error("ProcessCodeChange() skipped a refresh request under ARGO_DIFF_REQUIRE_APP_MATCH")
	}
}

// The provider's dialect decides the comment budget.
func TestProcessCodeChangeUsesProviderDialect(t *testing.T) {
	t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", "")
	var apps []argocd.ApplicationResourcesWithChanges
	for i := range 4 {
		apps = append(apps, app(fmt.Sprintf("app-%d", i), deployment("web", smallDiff+strings.Repeat("+  padding: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n", 40))))
	}
	fakeArgo(t, apps, nil, nil)

	f := newFakeProvider()
	_ = run(f, prEvent())
	if f.creates != 1 {
		t.Errorf("GitHub dialect: created %d comments, want 1", f.creates)
	}

	f = newFakeProvider()
	f.dialect = comment.Dialect{Name: "tiny", HardMax: 6000}
	_ = run(f, prEvent())
	if f.creates < 2 {
		t.Errorf("tiny dialect: created %d comments, want the diffs split across several", f.creates)
	}
	for _, c := range f.comments {
		if len(c.Body) > 6000 {
			t.Errorf("comment %d is %d bytes, over the dialect's 6000", c.ID, len(c.Body))
		}
	}
}
