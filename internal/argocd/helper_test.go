package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	wh "github.com/vince-riv/argo-diff/internal/webhook"
)

const testDataDir = "argocd_testdata"

const payloadAppList = "payload-GET-applications-brief.json"
const appManifestsOutput1 = "output-argocd-app-manifests-argoapps-namespace-only.txt"
const appManifestsOutputArgoApps = "output-argocd-app-manifests-argoapps.txt"

// const payloadAppRefresh = "payload-GET-application-refresh.json"

// const payloadManagedResources = "payload-GET-managed-resources.json"
// const payloadManifests = "payload-GET-manifest-current.json"
// const payloadManifestsChange = "payload-GET-manifest-change-1.json"
// const payloadError = "payload-GET-manifest-bad-kustomize.json"

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

func TestFilterApplications(t *testing.T) {
	var a []Application

	evtInfo := wh.EventInfo{RepoOwner: "o", RepoName: "r", RepoDefaultRef: "m", ChangeRef: "m", BaseRef: ""}
	result, _ := filterApplications(a, evtInfo, false)
	if len(result) != 0 {
		t.Error("Empty param didn't lead to empty result")
	}
	payload, _, err := readFileToByteArray(payloadAppList)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadAppList, err)
	}
	var appList ApplicationList
	if err := json.Unmarshal(payload, &appList); err != nil {
		t.Errorf("Error decoding ApplicationList payload: %v", err)
	}
	a = appList.Items
	if err != nil {
		t.Errorf("decodeApplicationListPayload() failed: %s", err)
	}

	evtInfo = wh.EventInfo{RepoOwner: "o", RepoName: "r", RepoDefaultRef: "m", ChangeRef: "m", BaseRef: ""}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 0 {
		t.Error("Unmatchable params didn't lead to empty result")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "refs/heads/main", BaseRef: ""}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 1 {
		t.Error("Push to main should have matched 1 (auto-sync off)")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "dev", BaseRef: "main"}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 1 {
		t.Error("Push to dev should have matched")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "dev", BaseRef: "not_main"}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 0 {
		t.Error("Non-main baseRef should not have matched")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "refs/heads/main", BaseRef: ""}
	a[0].Spec.Source.TargetRevision = "main"
	a[1].Spec.Source.TargetRevision = "main"
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 1 {
		t.Error("Push to main should have matched (targetRev main) (auto-sync off)")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "dev", BaseRef: "main"}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 1 {
		t.Error("Push to dev should have matched (targetRev main)")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "refs/heads/main", BaseRef: ""}
	a[1].Spec.SyncPolicy = &SyncPolicy{}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 1 {
		t.Error("Push to main should have matched (targetRev main) (auto-sync still off)")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "refs/heads/main", BaseRef: ""}
	a[1].Spec.SyncPolicy.Automated = &SyncPolicyAutomated{}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 0 {
		t.Error("Push to main should NOT have matched (targetRev main) (auto-sync ENABLED)")
	}
}

func TestFilterApplicationsFullyQualifiedRefs(t *testing.T) {
	loadApps := func(t *testing.T) []Application {
		payload, _, err := readFileToByteArray(payloadAppList)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", payloadAppList, err)
		}
		var appList ApplicationList
		if err := json.Unmarshal(payload, &appList); err != nil {
			t.Fatalf("Error decoding ApplicationList payload: %v", err)
		}
		return appList.Items
	}

	// targetRevision = refs/heads/main, PR base main should match even though
	// the PR's change branch is unrelated.
	a := loadApps(t)
	a[1].Spec.Source.TargetRevision = "refs/heads/main"
	evtInfo := wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "dev", BaseRef: "main"}
	result, _ := filterApplications(a, evtInfo, false)
	if len(result) != 1 {
		t.Error("PR against main should have matched (targetRev refs/heads/main)")
	}

	// targetRevision = refs/heads/dev
	a = loadApps(t)
	a[1].Spec.Source.TargetRevision = "refs/heads/dev"
	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "dev", BaseRef: "dev"}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 1 {
		t.Error("PR against dev should have matched (targetRev refs/heads/dev)")
	}
	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "dev", BaseRef: "main"}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 0 {
		t.Error("PR against main should NOT have matched (targetRev refs/heads/dev)")
	}

	// Push to refs/heads/main w/ auto-sync enabled should be filtered out,
	// same as the short-name case.
	a = loadApps(t)
	a[1].Spec.Source.TargetRevision = "refs/heads/main"
	a[1].Spec.SyncPolicy = &SyncPolicy{Automated: &SyncPolicyAutomated{}}
	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "refs/heads/main", BaseRef: ""}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 0 {
		t.Error("Push to main should NOT have matched (targetRev refs/heads/main) (auto-sync ENABLED)")
	}

	// Sanity: tag refs are not treated as branches.
	a = loadApps(t)
	a[1].Spec.Source.TargetRevision = "refs/tags/v1.0.0"
	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "dev", BaseRef: "v1.0.0"}
	result, _ = filterApplications(a, evtInfo, false)
	if len(result) != 0 {
		t.Error("PR against a tag name should NOT have matched (targetRev refs/tags/v1.0.0)")
	}
}

func TestNormalizeBranchRef(t *testing.T) {
	cases := map[string]string{
		"refs/heads/main": "main",
		"HEAD":            "HEAD",
		"refs/tags/v1":    "refs/tags/v1",
		"main":            "main",
	}
	for in, want := range cases {
		if got := normalizeBranchRef(in); got != want {
			t.Errorf("normalizeBranchRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterApplicationsMultiSource(t *testing.T) {
	var a []Application

	evtInfo := wh.EventInfo{RepoOwner: "o", RepoName: "r", RepoDefaultRef: "m", ChangeRef: "m", BaseRef: ""}
	result, _ := filterApplications(a, evtInfo, true)
	if len(result) != 0 {
		t.Error("Empty param didn't lead to empty result")
	}

	payload, _, err := readFileToByteArray(payloadAppList)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadAppList, err)
	}
	var appList ApplicationList
	if err := json.Unmarshal(payload, &appList); err != nil {
		t.Errorf("Error decoding ApplicationList payload: %v", err)
	}
	a = appList.Items
	if err != nil {
		t.Errorf("decodeApplicationListPayload() failed: %s", err)
	}

	evtInfo = wh.EventInfo{RepoOwner: "o", RepoName: "r", RepoDefaultRef: "m", ChangeRef: "m", BaseRef: ""}
	result, _ = filterApplications(a, evtInfo, true)
	if len(result) != 0 {
		t.Error("Unmatchable params didn't lead to empty result")
	}

	evtInfo = wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "refs/heads/main", BaseRef: ""}
	result, _ = filterApplications(a, evtInfo, true)
	if len(result) != 1 {
		t.Errorf("Push to main should have matched 1 (auto-sync off); got %d", len(result))
	}
}

// A multi-source app whose sources point at the changed repo more than once
// (eg: a chart source and a values source in the same monorepo) must be
// returned once, so it's only diffed once.
func TestFilterApplicationsMultiSourceMatchesOnce(t *testing.T) {
	repoURL := "https://github.com/vince-riv/argo-diff.git"
	a := []Application{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "monorepo-app"},
			Spec: ApplicationSpec{
				Sources: []ApplicationSource{
					{RepoURL: "https://charts.example.com", TargetRevision: "1.2.3", Chart: "somechart"},
					{RepoURL: repoURL, TargetRevision: "main", Path: "charts/mychart"},
					{RepoURL: repoURL, TargetRevision: "main", Path: "values/mychart", Ref: "values"},
				},
			},
		},
	}

	evtInfo := wh.EventInfo{RepoOwner: "vince-riv", RepoName: "argo-diff", RepoDefaultRef: "main", ChangeRef: "my-branch", BaseRef: "main"}
	result, err := filterApplications(a, evtInfo, true)
	if err != nil {
		t.Fatalf("filterApplications() err'd: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("app with 2 matching sources should be returned once; got %d", len(result))
	}
}

// When the context is out of time, matching applications are reported as
// not diffed instead of each one firing an `argocd app diff` that can only fail.
func TestGetApplicationChangesOutOfTime(t *testing.T) {
	appListData, filePath, err := readFileToByteArray(outputListApplications)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", filePath, err)
	}

	tests := []struct {
		name     string
		repoName string
		want     []string
	}{
		{"single source app", "argo-diff-config", []string{"argo-diff"}},
		{"multi source app", "argo-diff-testing", []string{"argo-diff-testing"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARGO_DIFF_MAX_WORKERS", "1")
			diffCalls := 0
			originalExecArgoCdCli := execArgoCdCli
			defer func() { execArgoCdCli = originalExecArgoCdCli }()
			execArgoCdCli = func(ctx context.Context, args []string) ([]byte, error) {
				if len(args) > 1 && args[1] == "diff" {
					diffCalls++
				}
				return appListData, nil
			}

			// already out of time before any diffing starts
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()

			evtInfo := wh.EventInfo{
				RepoOwner:      "vince-riv",
				RepoName:       tt.repoName,
				RepoDefaultRef: "main",
				ChangeRef:      "my-branch",
				BaseRef:        "main",
				Sha:            "abcdef",
			}
			appResList, notDiffed, err := GetApplicationChanges(ctx, evtInfo)
			if err != nil {
				t.Errorf("GetApplicationChanges() err'd: %v", err)
			}
			if len(appResList) != 0 {
				t.Errorf("expected no diff results, got %d", len(appResList))
			}
			if !slices.Equal(notDiffed, tt.want) {
				t.Errorf("notDiffed = %v, want %v", notDiffed, tt.want)
			}
			if diffCalls != 0 {
				t.Errorf("expected no `argocd app diff` calls once out of time, got %d", diffCalls)
			}
		})
	}
}

// Running out of time while enumerating an app-of-apps' nested Applications
// must still be reported: the parent's own diff succeeds, so without an entry
// the run looks complete while every nested application diff is missing.
func TestGetApplicationChangesOutOfTimeEnumeratingNestedApps(t *testing.T) {
	t.Setenv("ARGO_DIFF_MAX_WORKERS", "1")
	appListData, filePath, err := readFileToByteArray(outputListApplications)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", filePath, err)
	}
	// a diff whose changed resource is itself an ArgoCD Application, which sends
	// GetApplicationChanges() into the nested-application path
	nestedAppDiff := []byte(`===== argoproj.io/Application /nested-app ======
--- /tmp/argocd-diff/nested-app-live.yaml	2026-07-31 21:49:00
+++ /tmp/argocd-diff/nested-app	2026-07-31 21:49:00
@@ -1,4 +1,4 @@
   spec:
-    targetRevision: 1.0.0
+    targetRevision: 2.0.0
`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manifestCalls := 0
	originalExecArgoCdCli := execArgoCdCli
	defer func() { execArgoCdCli = originalExecArgoCdCli }()
	execArgoCdCli = func(c context.Context, args []string) ([]byte, error) {
		switch args[1] {
		case "list":
			return appListData, nil
		case "diff":
			// the deadline lands immediately after the parent's diff succeeds
			cancel()
			return nestedAppDiff, makeExitError(t, nil)
		case "manifests":
			manifestCalls++
			return nil, c.Err()
		}
		return nil, fmt.Errorf("unexpected argocd args: %v", args)
	}

	evtInfo := wh.EventInfo{
		RepoOwner:      "vince-riv",
		RepoName:       "argo-diff-config",
		RepoDefaultRef: "main",
		ChangeRef:      "my-branch",
		BaseRef:        "main",
		Sha:            "abcdef",
	}
	appResList, notDiffed, err := GetApplicationChanges(ctx, evtInfo)
	if err != nil {
		t.Errorf("GetApplicationChanges() err'd: %v", err)
	}
	if manifestCalls != 1 {
		t.Errorf("expected 1 `argocd app manifests` call, got %d", manifestCalls)
	}
	// the parent's own diff succeeded and should still be reported
	if len(appResList) != 1 {
		t.Errorf("expected the parent app's diff in the results, got %d results", len(appResList))
	}
	want := []string{"nested apps of argo-diff"}
	if !slices.Equal(notDiffed, want) {
		t.Errorf("notDiffed = %v, want %v", notDiffed, want)
	}
}

// buildTestApps returns n single-source Applications, all matching repoURL,
// suitable for marshaling into a fake `argocd app list -o json` response.
func buildTestApps(n int, repoURL string) []Application {
	apps := make([]Application, n)
	for i := range apps {
		apps[i] = Application{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pool-app-%d", i)},
			Spec: ApplicationSpec{
				Source: &ApplicationSource{RepoURL: repoURL, TargetRevision: "main"},
			},
		}
	}
	return apps
}

// The worker pool must actually bound concurrency to ARGO_DIFF_MAX_WORKERS:
// with more matching apps than the configured limit, no more than that many
// `argocd app diff` calls should ever be in flight at once.
func TestGetApplicationChangesConcurrencyBound(t *testing.T) {
	const workers = 3
	const numApps = 9
	t.Setenv("ARGO_DIFF_MAX_WORKERS", fmt.Sprintf("%d", workers))

	repoURL := "https://github.com/acme/widgets.git"
	appListJSON, err := json.Marshal(buildTestApps(numApps, repoURL))
	if err != nil {
		t.Fatalf("failed to marshal test apps: %v", err)
	}

	var current atomic.Int64
	var maxSeen atomic.Int64

	originalExecArgoCdCli := execArgoCdCli
	defer func() { execArgoCdCli = originalExecArgoCdCli }()
	execArgoCdCli = func(ctx context.Context, args []string) ([]byte, error) {
		if len(args) > 1 && args[1] == "list" {
			return appListJSON, nil
		}
		if len(args) > 1 && args[1] == "diff" {
			n := current.Add(1)
			for {
				m := maxSeen.Load()
				if n <= m || maxSeen.CompareAndSwap(m, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			current.Add(-1)
			return nil, nil // no changes
		}
		return nil, fmt.Errorf("unexpected argocd args: %v", args)
	}

	evtInfo := wh.EventInfo{RepoOwner: "acme", RepoName: "widgets", RepoDefaultRef: "main", ChangeRef: "my-branch", BaseRef: "main", Sha: "abcdef"}
	_, notDiffed, err := GetApplicationChanges(context.Background(), evtInfo)
	if err != nil {
		t.Fatalf("GetApplicationChanges() err'd: %v", err)
	}
	if len(notDiffed) != 0 {
		t.Errorf("expected no notDiffed entries, got %v", notDiffed)
	}
	// Exactly `workers` can under-report on a loaded CI runner; >1 still
	// proves genuine parallelism, and <=workers proves the bound holds.
	if got := maxSeen.Load(); got <= 1 || got > workers {
		t.Errorf("observed max concurrency = %d, want >1 and <=%d", got, workers)
	}
}

// Regardless of how many workers actually run concurrently, appResList must
// come back in the same order (original apps order) as a fully sequential
// (ARGO_DIFF_MAX_WORKERS=1) run, since results are merged from a pre-sized,
// index-addressed slice after each wave's pool drains.
func TestGetApplicationChangesOrderStable(t *testing.T) {
	const numApps = 6
	repoURL := "https://github.com/acme/widgets.git"
	appListJSON, err := json.Marshal(buildTestApps(numApps, repoURL))
	if err != nil {
		t.Fatalf("failed to marshal test apps: %v", err)
	}

	newMock := func(t *testing.T) func(context.Context, []string) ([]byte, error) {
		return func(ctx context.Context, args []string) ([]byte, error) {
			if len(args) > 1 && args[1] == "list" {
				return appListJSON, nil
			}
			if len(args) > 1 && args[1] == "diff" {
				appName := args[2]
				var idx int
				_, _ = fmt.Sscanf(appName, "pool-app-%d", &idx)
				// later-indexed apps finish first, so a truly parallel run
				// completes out of dispatch order
				time.Sleep(time.Duration(numApps-idx) * 5 * time.Millisecond)
				diffStr := fmt.Sprintf("===== apps/Deployment /dummy-%s ======\n--- a\n+++ b\n@@ -1 +1 @@\n-old\n+new\n", appName)
				return []byte(diffStr), makeExitError(t, nil)
			}
			if len(args) > 1 && args[1] == "manifests" {
				return nil, nil
			}
			return nil, fmt.Errorf("unexpected argocd args: %v", args)
		}
	}

	evtInfo := wh.EventInfo{RepoOwner: "acme", RepoName: "widgets", RepoDefaultRef: "main", ChangeRef: "my-branch", BaseRef: "main", Sha: "abcdef"}

	originalExecArgoCdCli := execArgoCdCli
	defer func() { execArgoCdCli = originalExecArgoCdCli }()

	t.Setenv("ARGO_DIFF_MAX_WORKERS", "1")
	execArgoCdCli = newMock(t)
	seqResList, _, err := GetApplicationChanges(context.Background(), evtInfo)
	if err != nil {
		t.Fatalf("GetApplicationChanges() (sequential) err'd: %v", err)
	}

	t.Setenv("ARGO_DIFF_MAX_WORKERS", "4")
	execArgoCdCli = newMock(t)
	parResList, _, err := GetApplicationChanges(context.Background(), evtInfo)
	if err != nil {
		t.Fatalf("GetApplicationChanges() (parallel) err'd: %v", err)
	}

	if len(seqResList) != numApps || len(parResList) != numApps {
		t.Fatalf("expected %d results in both runs, got sequential=%d parallel=%d", numApps, len(seqResList), len(parResList))
	}
	for i := range seqResList {
		seqName := seqResList[i].ArgoApp.Name
		parName := parResList[i].ArgoApp.Name
		if seqName != parName {
			t.Errorf("order mismatch at index %d: sequential=%s parallel=%s", i, seqName, parName)
		}
	}
}

// A parent app-of-apps' diff must be immediately followed by its own nested
// apps' diffs in appResList, even though wave 2 (nested apps) now runs as one
// flattened, pool-bounded batch across all parents rather than per-parent.
// Without the parentIdx-based merge this regresses to "all parents, then all
// nested apps," which scrambles app-of-apps grouping in the PR comment.
func TestGetApplicationChangesNestedAppsGroupedWithParent(t *testing.T) {
	t.Setenv("ARGO_DIFF_MAX_WORKERS", "4")

	const repoURL = "https://github.com/acme/widgets.git"
	const otherRepoURL = "https://github.com/acme/other.git"

	parents := buildTestApps(3, repoURL) // pool-app-0, pool-app-1, pool-app-2
	children := []Application{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-app-0-child"},
			Spec:       ApplicationSpec{Source: &ApplicationSource{RepoURL: otherRepoURL, TargetRevision: "main"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pool-app-2-child"},
			Spec:       ApplicationSpec{Source: &ApplicationSource{RepoURL: otherRepoURL, TargetRevision: "main"}},
		},
	}
	appListJSON, err := json.Marshal(append(append([]Application{}, parents...), children...))
	if err != nil {
		t.Fatalf("failed to marshal test apps: %v", err)
	}

	nestedAppDiff := func(childName string) []byte {
		return []byte(fmt.Sprintf(`===== argoproj.io/Application /%s ======
--- a
+++ b
@@ -1 +1 @@
-old
+new
`, childName))
	}
	manifestWithChild := func(childName string) []byte {
		return []byte(fmt.Sprintf(`apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s
  namespace: argocd
spec:
  source:
    repoURL: %s
    targetRevision: main
`, childName, otherRepoURL))
	}
	plainDiff := []byte(`===== apps/Deployment /dummy ======
--- a
+++ b
@@ -1 +1 @@
-old
+new
`)

	originalExecArgoCdCli := execArgoCdCli
	defer func() { execArgoCdCli = originalExecArgoCdCli }()
	execArgoCdCli = func(ctx context.Context, args []string) ([]byte, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("unexpected argocd args: %v", args)
		}
		appName := ""
		if len(args) > 2 {
			appName = args[2]
		}
		switch {
		case args[1] == "list":
			return appListJSON, nil
		case args[1] == "diff" && (appName == "pool-app-0" || appName == "pool-app-2"):
			time.Sleep(20 * time.Millisecond) // let the sibling parent/children race ahead
			return nestedAppDiff(appName + "-child"), makeExitError(t, nil)
		case args[1] == "diff" && appName == "pool-app-1":
			return plainDiff, makeExitError(t, nil)
		case args[1] == "diff" && (appName == "pool-app-0-child" || appName == "pool-app-2-child"):
			return nestedAppDiff(appName), makeExitError(t, nil)
		case args[1] == "manifests":
			return manifestWithChild(appName + "-child"), nil
		}
		return nil, fmt.Errorf("unexpected argocd args: %v", args)
	}

	evtInfo := wh.EventInfo{RepoOwner: "acme", RepoName: "widgets", RepoDefaultRef: "main", ChangeRef: "my-branch", BaseRef: "main", Sha: "abcdef"}
	appResList, notDiffed, err := GetApplicationChanges(context.Background(), evtInfo)
	if err != nil {
		t.Fatalf("GetApplicationChanges() err'd: %v", err)
	}
	if len(notDiffed) != 0 {
		t.Fatalf("expected no notDiffed entries, got %v", notDiffed)
	}

	var gotNames []string
	for _, r := range appResList {
		gotNames = append(gotNames, r.ArgoApp.Name)
	}
	want := []string{"pool-app-0", "pool-app-0-child", "pool-app-1", "pool-app-2", "pool-app-2-child"}
	if !slices.Equal(gotNames, want) {
		t.Errorf("appResList order = %v, want %v (parent must be immediately followed by its own nested apps)", gotNames, want)
	}
}

func TestVersionCheck(t *testing.T) {
	if !versionCheck("2.12.1") {
		t.Error("v2.12.1 should pass")
	}
	if versionCheck("2.11.100") {
		t.Error("v2.12.100 should not pass")
	}
	if !versionCheck("2.13.0") {
		t.Error("v2.13.0 should pass")
	}
	if versionCheck("1.150.0") {
		t.Error("v1.150.0 should not pass")
	}
}

func TestArgoAppsWithChanges(t *testing.T) {
	ctx := context.Background()
	appResources := []AppResource{
		AppResource{ApiVersion: "v1", Group: "apps", Kind: "Deployment", Namespace: "test", Name: "testdeploy"},
		AppResource{ApiVersion: "v1", Group: "", Kind: "ConfigMap", Namespace: "test", Name: "testcm"},
	}
	result, err := argoAppsWithChanges(ctx, "testapp", appResources, "abcdef")
	if err != nil {
		t.Errorf("argoAppsWithChanges() erroed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected no results, got %d", len(result))
	}
}

func TestManifestIsArgoApplication(t *testing.T) {
	output, filepath, err := readFileToByteArray(appManifestsOutputArgoApps)
	if err != nil {
		t.Errorf("Failed to read %s: %v", filepath, err)
	}
	manifests, err := appManifestHelper(output)
	if err != nil {
		t.Errorf("Decoding yaml in %s failed: %v", filepath, err)
	}
	for i, manifest := range manifests {
		isArgoApp := manifestIsArgoApplication(manifest)
		if i == 0 || i == 1 {
			if isArgoApp {
				t.Errorf("Manifest index %d in %s marked as Argo Application, but isn't", i, filepath)
			}
		} else if i == 2 || i == 3 {
			if !isArgoApp {
				t.Errorf("Manifest index %d in %s not marked as Argo Application, but is", i, filepath)
			}
		} else {
			t.Errorf("Expected 4 manifests in %s", filepath)
		}
	}
}

func TestGitRepoMatch(t *testing.T) {
	const owner = "acme"
	const repo = "widgets"
	tests := []struct {
		name    string
		repoURL string
		chart   string
		want    bool
	}{
		// github.com (existing behavior)
		{"github https .git", "https://github.com/acme/widgets.git", "", true},
		{"github https no .git", "https://github.com/acme/widgets", "", true},
		{"github scp ssh", "git@github.com:acme/widgets.git", "", true},
		// non-github hosts (new fallback)
		{"github enterprise", "https://github.example.com/acme/widgets.git", "", true},
		{"aws codeconnections", "https://codeconnections.us-west-2.amazonaws.com/git-http/111122223333/us-west-2/1a2b3c/acme/widgets.git", "", true},
		{"gitlab mirror no .git", "https://gitlab.example.com/group/acme/widgets", "", true},
		{"generic scp ssh", "git@git.example.com:acme/widgets.git", "", true},
		{"case insensitive", "https://github.example.com/ACME/Widgets.git", "", true},
		// negatives
		{"different repo", "https://github.example.com/acme/gadgets.git", "", false},
		{"owner suffix must not partial match", "https://github.example.com/notacme/widgets.git", "", false},
		{"repo name substring", "https://github.example.com/acme/widgets-internal.git", "", false},
		{"empty url", "", "", false},
		// Chart/OCI sources must never match, even if RepoURL's path happens to
		// collide with owner/repo (RepoURL here is a Helm/OCI registry, not a
		// git remote).
		{"chart source with colliding path is never matched", "oci://registry.example.com/acme/widgets", "widgets", false},
		{"chart source with github.com host is never matched", "https://github.com/acme/widgets.git", "widgets", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := ApplicationSource{RepoURL: tc.repoURL, Chart: tc.chart}
			if got := gitRepoMatch(src, owner, repo); got != tc.want {
				t.Errorf("gitRepoMatch(%+v, %q, %q) = %v; want %v", src, owner, repo, got, tc.want)
			}
		})
	}
}

func TestGitRepoMatch_DisableNonGithubFallback(t *testing.T) {
	const owner = "acme"
	const repo = "widgets"
	tests := []struct {
		name     string
		repoURL  string
		disabled string
		want     bool
	}{
		{"non-github host matches when flag unset", "https://github.example.com/acme/widgets.git", "", true},
		{"non-github host matches when flag false", "https://github.example.com/acme/widgets.git", "false", true},
		{"non-github host blocked when flag true", "https://github.example.com/acme/widgets.git", "true", false},
		{"non-github host blocked when flag TRUE (case-insensitive)", "https://github.example.com/acme/widgets.git", "TRUE", false},
		{"github.com still matches when flag true", "https://github.com/acme/widgets.git", "true", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Empty string behaves identically to the var being unset, since the
			// check only looks for the literal value "true".
			t.Setenv("ARGO_DIFF_DISABLE_NON_GITHUB_REPO_MATCH", tc.disabled)
			src := ApplicationSource{RepoURL: tc.repoURL}
			if got := gitRepoMatch(src, owner, repo); got != tc.want {
				t.Errorf("gitRepoMatch(%+v, %q, %q) with ARGO_DIFF_DISABLE_NON_GITHUB_REPO_MATCH=%q = %v; want %v", src, owner, repo, tc.disabled, got, tc.want)
			}
		})
	}
}
