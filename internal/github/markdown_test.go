package github

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// wrappedLen is the size of what Comment() actually posts, so a test can assert
// on the bytes sent to GitHub rather than on String()'s output alone.
func wrappedLen(body string) int {
	return len(wrapComment(body))
}

// setWrapper installs a preamble and identifier for the duration of a test.
// They're package vars set by comment.go's init(), which t.Setenv can't reach.
func setWrapper(t *testing.T, preamble, identifier string) {
	t.Helper()
	origP, origI := commentPreamble, commentIdentifier
	t.Cleanup(func() { commentPreamble, commentIdentifier = origP, origI })
	commentPreamble, commentIdentifier = preamble, identifier
}

func setUiURL(t *testing.T, url string) {
	t.Helper()
	orig := argocdUiUrl
	t.Cleanup(func() { argocdUiUrl = orig })
	argocdUiUrl = url
}

func fakeDiff(lines int) string {
	var b strings.Builder
	b.WriteString("--- live.yaml\n+++ new.yaml\n@@ -1,1 +1,1 @@\n")
	for i := range lines {
		fmt.Fprintf(&b, "-  key%d: old value padding padding padding\n", i)
		fmt.Fprintf(&b, "+  key%d: new value padding padding padding\n", i)
	}
	return b.String()
}

// checkBodyWellFormed asserts the invariants every comment body must hold no
// matter where a split lands: paired <details> tags and paired code fences. An
// unbalanced tag or an odd fence count swallows everything below it.
func checkBodyWellFormed(t *testing.T, i int, body string) {
	t.Helper()
	if open, closed := strings.Count(body, "<details"), strings.Count(body, "</details>"); open != closed {
		t.Errorf("body %d has %d <details> and %d </details>, want them paired:\n%s", i, open, closed, body)
	}
	if n := strings.Count(body, "```"); n%2 != 0 {
		t.Errorf("body %d has %d code fences, want an even number:\n%s", i, n, body)
	}
	// GitHub renders its coloured alert boxes only at the top level of a
	// document. Inside a <details> the extension is skipped and the block
	// degrades to a plain blockquote with a literal "[!NOTE]" first line, so an
	// alert emitted in there is a rendering bug, not a style choice.
	depth := 0
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "<details"):
			depth++
		case strings.HasPrefix(line, "</details>"):
			depth--
		case strings.HasPrefix(line, "> [!") && depth > 0:
			t.Errorf("body %d has an alert %q nested %d <details> deep; GitHub won't render it:\n%s", i, line, depth, body)
		}
	}
}

// A fatal error and an advisory notice must be told apart at a glance. The
// error renders as a red [!CAUTION] alert and suppresses the app's diffs; the
// notice renders as a blue [!NOTE] alert *above* diffs that are still worth
// reading.
func TestErrStrAndNoticeStrRenderDistinctly(t *testing.T) {
	t.Run("notice sits above the app block and its diffs survive", func(t *testing.T) {
		c := CommentMarkdown{}
		a := c.AppMarkdown(AppMarkdownOpts{
			Name:       "my-app",
			NoticeStr:  "Unable to discover app-of-apps children of my-app: exit status 1",
			SyncStatus: "Synced", HealthStatus: "Healthy",
		})
		a.AddResourceDiff("apps", "Deployment", "web", "prod", "-old\n+new\n")

		bodies := c.String()
		if len(bodies) != 1 {
			t.Fatalf("got %d comment bodies, want 1", len(bodies))
		}
		body := bodies[0]
		checkBodyWellFormed(t, 0, body)

		if !strings.Contains(body, "> [!NOTE]") {
			t.Errorf("notice did not render as a [!NOTE] alert:\n%s", body)
		}
		if strings.Contains(body, "[!CAUTION]") {
			t.Errorf("an advisory notice must not render as a fatal alert:\n%s", body)
		}
		// the alert is detached from the block, so it has to name the app
		if !strings.Contains(body, "**my-app**") {
			t.Errorf("hoisted notice does not name its application:\n%s", body)
		}
		noticeIdx := strings.Index(body, "> [!NOTE]")
		blockIdx := strings.Index(body, "<details")
		diffIdx := strings.Index(body, "apps/Deployment")
		if diffIdx < 0 {
			t.Fatal("resource diff missing: an advisory notice must not suppress the diffs")
		}
		if noticeIdx > blockIdx {
			t.Error("notice rendered below the application block, want above it")
		}
	})

	t.Run("error renders as a caution and drops the empty block", func(t *testing.T) {
		c := CommentMarkdown{}
		c.AppMarkdown(AppMarkdownOpts{
			Name:       "broken",
			ErrStr:     "Failed to diff application broken: app path does not exist",
			SyncStatus: "Unknown", HealthStatus: "Healthy",
		})
		body := c.String()[0]
		checkBodyWellFormed(t, 0, body)

		if !strings.Contains(body, "> [!CAUTION]") {
			t.Errorf("error did not render as a [!CAUTION] alert:\n%s", body)
		}
		if strings.Contains(body, "[!NOTE]") {
			t.Errorf("a fatal error must not render as an advisory:\n%s", body)
		}
		if strings.Contains(body, "<details") {
			t.Errorf("a failed application has no diffs, so it should have no block to fold:\n%s", body)
		}
		if !strings.Contains(body, "**broken**") {
			t.Errorf("caution does not name its application:\n%s", body)
		}
		if !strings.Contains(body, "app path does not exist") {
			t.Errorf("error text missing:\n%s", body)
		}
		// the fence inside an alert only renders when every line is quoted
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, "app path does not exist") && !strings.HasPrefix(line, "> ") {
				t.Errorf("error line inside the alert is not blockquoted: %q", line)
			}
		}
	})

	// An application whose diffs deserve a second look must not be hidden by
	// auto-collapse, whatever the thresholds say.
	t.Run("an app carrying a notice is never auto-folded", func(t *testing.T) {
		t.Setenv("ARGO_DIFF_COMMENT_INDEX_COUNT", "0")
		c := CommentMarkdown{}
		for i := range 9 { // well past autoCollapseAppCount
			o := AppMarkdownOpts{Name: fmt.Sprintf("app-%d", i), SyncStatus: "Synced", HealthStatus: "Healthy"}
			if i == 4 {
				o.NoticeStr = "children could not be enumerated"
			}
			a := c.AppMarkdown(o)
			a.AddResourceDiff("apps", "Deployment", "web", "prod", "-a\n+b\n")
		}
		body := c.String()[0]
		if strings.Count(body, "<details open>") != 1+9 {
			// one open app block, plus every resource block (all apps have 1 resource)
			t.Errorf("expected exactly the noticed app to stay open:\n%s", body)
		}
		noticed := body[strings.Index(body, "> [!NOTE]"):]
		if !strings.HasPrefix(noticed[strings.Index(noticed, "<details"):], "<details open>") {
			t.Errorf("the app carrying the notice was folded:\n%s", body)
		}
	})
}

// The regression test for the budget leak: Comment() wraps every body with an
// operator preamble and the HTML identifier, neither of which String() used to
// count. A long preamble plus a full body is a 422 from the API and no comment.
func TestBodiesStayWithinBudgetIncludingWrapper(t *testing.T) {
	t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", "9000")
	setWrapper(t, strings.Repeat("P", 3000), "<!-- comment produced by argo-diff[ci] - refs/pull/1/merge -->")

	c := CommentMarkdown{Preamble: "**4 of 4 apps with changes** compared to live state\n"}
	for i := range 4 {
		a := c.AppMarkdown(AppMarkdownOpts{
			Name: fmt.Sprintf("app-%d", i), SyncStatus: "Synced", HealthStatus: "Healthy",
		})
		for j := range 3 {
			a.AddResourceDiff("apps", "Deployment", fmt.Sprintf("web-%d", j), "prod", fakeDiff(10))
		}
	}

	bodies := c.String()
	if len(bodies) < 2 {
		t.Fatalf("got %d bodies, want the content split across several", len(bodies))
	}
	for i, b := range bodies {
		checkBodyWellFormed(t, i, b)
		if got := wrappedLen(b); got > commentMaxLen() {
			t.Errorf("body %d is %d bytes once wrapped, over the %d byte cap", i, got, commentMaxLen())
		}
		if !strings.Contains(b, "part ") {
			t.Errorf("body %d is missing its 'part i of n' header:\n%s", i, b)
		}
	}
	if !strings.Contains(bodies[0], "[Continued in next comment]") {
		t.Errorf("first body does not say it continues:\n%s", bodies[0])
	}
}

// A resource whose diff cannot fit in any body must degrade to the too-large
// marker rather than produce an over-cap body.
func TestOversizedResourceStaysInBudget(t *testing.T) {
	t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", "4000")
	setWrapper(t, "", "<!-- argo-diff -->")

	c := CommentMarkdown{}
	a := c.AppMarkdown(AppMarkdownOpts{Name: "big", SyncStatus: "Synced", HealthStatus: "Healthy"})
	a.AddResourceDiff("apps", "Deployment", "web", "prod", fakeDiff(2000))

	for i, b := range c.String() {
		checkBodyWellFormed(t, i, b)
		if got := wrappedLen(b); got > commentMaxLen() {
			t.Errorf("body %d is %d bytes once wrapped, over the %d byte cap", i, got, commentMaxLen())
		}
	}
}

// ErrStr is err.Error() and unbounded. It must not be able to crowd out the
// budget or the diffs.
func TestOversizedErrStrIsTruncated(t *testing.T) {
	c := CommentMarkdown{}
	c.AppMarkdown(AppMarkdownOpts{
		Name: "loud", ErrStr: strings.Repeat("x", 100000),
		SyncStatus: "Unknown", HealthStatus: "Unknown",
	})
	body := c.String()[0]
	checkBodyWellFormed(t, 0, body)
	if len(body) > maxNoticeLen*2 {
		t.Errorf("body is %d bytes, want the error bounded near maxNoticeLen (%d)", len(body), maxNoticeLen)
	}
	if !strings.Contains(body, "[truncated]") {
		t.Error("truncation is not visible to the reader")
	}
}

func TestIndexCount(t *testing.T) {
	setUiURL(t, "https://argocd.example")
	tests := []struct {
		env     string
		apps    int
		wantIdx bool
	}{
		{"", 1, false},        // default 2: one app is not worth an index
		{"", 2, true},         // default 2
		{"-1", 1, true},       // always
		{"0", 9, false},       // never
		{"5", 4, false},       // threshold not met
		{"5", 5, true},        // threshold met
		{"nonsense", 2, true}, // unparseable falls back to the default
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("env=%q/apps=%d", tt.env, tt.apps), func(t *testing.T) {
			t.Setenv("ARGO_DIFF_COMMENT_INDEX_COUNT", tt.env)
			c := CommentMarkdown{}
			for i := range tt.apps {
				a := c.AppMarkdown(AppMarkdownOpts{
					Name: fmt.Sprintf("app-%d", i), SyncStatus: "Synced", HealthStatus: "Healthy",
				})
				a.AddResourceDiff("apps", "Deployment", "web", "prod", "-a\n+b\n")
			}
			body := c.String()[0]
			if got := strings.Contains(body, "| Application | Changed | Sync | Health |"); got != tt.wantIdx {
				t.Errorf("index rendered = %v, want %v:\n%s", got, tt.wantIdx, body)
			}
		})
	}
}

func TestCollapseMode(t *testing.T) {
	tests := []struct {
		mode      string
		apps      int
		resources int
		wantApp   string
		wantRes   string
	}{
		{"", 1, 1, "<details open>", "<details open>"},         // auto, small
		{"", 9, 1, "<details>", "<details open>"},              // auto, many apps
		{"", 1, 9, "<details open>", "<details>"},              // auto, many resources
		{"expanded", 9, 9, "<details open>", "<details open>"}, // forced open
		{"collapsed", 1, 1, "<details>", "<details>"},          // forced closed
		{"nonsense", 1, 1, "<details open>", "<details open>"}, // falls back to auto
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("mode=%q/%dapps/%dres", tt.mode, tt.apps, tt.resources), func(t *testing.T) {
			t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE", tt.mode)
			t.Setenv("ARGO_DIFF_COMMENT_INDEX_COUNT", "0")
			c := CommentMarkdown{}
			for i := range tt.apps {
				a := c.AppMarkdown(AppMarkdownOpts{
					Name: fmt.Sprintf("app-%d", i), SyncStatus: "Synced", HealthStatus: "Healthy",
				})
				for j := range tt.resources {
					a.AddResourceDiff("apps", "Deployment", fmt.Sprintf("web-%d", j), "prod", "-a\n+b\n")
				}
			}
			body := c.String()[0]
			// the app block is the first <details> in the body
			appTag := body[strings.Index(body, "<details"):]
			appTag = appTag[:strings.Index(appTag, "\n")]
			if appTag != tt.wantApp {
				t.Errorf("app block = %q, want %q", appTag, tt.wantApp)
			}
			resIdx := strings.Index(body, "<summary><code>")
			resTag := body[:resIdx]
			resTag = resTag[strings.LastIndex(resTag, "<details"):]
			resTag = resTag[:strings.Index(resTag, "\n")]
			if resTag != tt.wantRes {
				t.Errorf("resource block = %q, want %q", resTag, tt.wantRes)
			}
		})
	}
}

func TestTopLevelNotices(t *testing.T) {
	c := CommentMarkdown{
		Preamble: "**1 of 1 apps with changes**\n",
		Notices:  []string{"feature X is deprecated", "  ", "argocd CLI 2.11 is too old for Y"},
	}
	a := c.AppMarkdown(AppMarkdownOpts{Name: "app", SyncStatus: "Synced", HealthStatus: "Healthy"})
	a.AddResourceDiff("apps", "Deployment", "web", "prod", "-a\n+b\n")

	body := c.String()[0]
	checkBodyWellFormed(t, 0, body)
	if n := strings.Count(body, "> [!NOTE]"); n != 2 {
		t.Errorf("got %d [!NOTE] alerts, want 2 (the blank notice must be dropped):\n%s", n, body)
	}
	if strings.Index(body, "deprecated") > strings.Index(body, "<details") {
		t.Error("top-level notices must render above the application blocks")
	}
}

func TestDiffStats(t *testing.T) {
	tests := []struct {
		name           string
		in             string
		added, removed int
	}{
		{"empty", "", 0, 0},
		{"headers only", "--- live.yaml\n+++ new.yaml\n@@ -1 +1 @@\n", 0, 0},
		{"added only", "+a\n+b\n c\n", 2, 0},
		{"removed only", "-a\n-b\n c\n", 0, 2},
		{"mixed with headers", "--- live.yaml\n+++ new.yaml\n-a\n+b\n+c\n", 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			added, removed := diffStats(tt.in)
			if added != tt.added || removed != tt.removed {
				t.Errorf("diffStats() = (+%d, -%d), want (+%d, -%d)", added, removed, tt.added, tt.removed)
			}
		})
	}
}

// truncateLines must always end in exactly one newline, so a caller can close a
// code fence on its own line without inspecting the original string.
func TestTruncateLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"no trailing newline", "abc", 10, "abc\n"},
		{"trailing newline", "abc\n", 10, "abc\n"},
		{"several trailing newlines", "abc\n\n\n", 10, "abc\n"},
		{"truncated", "abcdef", 3, "abc...[TRUNCATED]\n"},
		{"multibyte not split", "ααα", 3, "α...[TRUNCATED]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncateLines(tt.in, tt.max); got != tt.want {
				t.Errorf("truncateLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A resource diff's summary carries the change size so a reader can judge it
// before expanding a folded block.
func TestResourceSummaryStats(t *testing.T) {
	c := CommentMarkdown{}
	a := c.AppMarkdown(AppMarkdownOpts{Name: "app", SyncStatus: "Synced", HealthStatus: "Healthy"})
	a.AddResourceDiff("apps", "Deployment", "web", "prod", "--- live\n+++ new\n-a\n+b\n+c\n")
	a.AddResourceDiff("", "ConfigMap", "cfg", "prod", "-a\n+b\n")

	body := c.String()[0]
	if !strings.Contains(body, "<code>apps/Deployment</code> prod/web · <b>+2 −1</b>") {
		t.Errorf("resource summary missing name or stats:\n%s", body)
	}
	// a core-group resource must not render with a leading slash
	if !strings.Contains(body, "<code>ConfigMap</code> prod/cfg") {
		t.Errorf("core-group resource summary is wrong:\n%s", body)
	}
}

// The budget only works if commentWrapperLen() stays in step with what
// Comment() actually posts. They are derived from one function so they cannot
// drift, and this pins that down.
func TestCommentWrapperLenMatchesWrapComment(t *testing.T) {
	for _, preamble := range []string{"", "Argo-Diff for **prod**"} {
		setWrapper(t, preamble, "<!-- comment produced by argo-diff[prod] -->")
		body := "some rendered markdown"
		want := len(wrapComment(body))
		if got := len(body) + commentWrapperLen(); got != want {
			t.Errorf("preamble %q: body+wrapper = %d, want %d", preamble, got, want)
		}
	}
}

// An application carrying an alert *and* a near-cap resource is the case the
// budget used to miss: maxResourceBodyLen() reserved only the <details> header,
// not the ~8KB the hoisted alerts can add ahead of it, so the body overshot and
// fell through to finalize() - which then cut through an open ```diff fence and
// two open <details>.
func TestAlertPlusLargeResourceStaysWellFormed(t *testing.T) {
	t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", "20000")
	setWrapper(t, "", "<!-- argo-diff -->")

	for _, lines := range []int{150, 180, 400} {
		t.Run(fmt.Sprintf("%d changed lines", lines), func(t *testing.T) {
			c := CommentMarkdown{Preamble: "**1 of 1 apps with changes**\n"}
			a := c.AppMarkdown(AppMarkdownOpts{
				Name:       "noisy",
				NoticeStr:  strings.Repeat("n", maxNoticeLen),
				SyncStatus: "Synced", HealthStatus: "Healthy",
			})
			a.AddResourceDiff("apps", "Deployment", "web", "prod", fakeDiff(lines))

			bodies := c.String()
			for i, b := range bodies {
				checkBodyWellFormed(t, i, b)
				if got := wrappedLen(b); got > commentMaxLen() {
					t.Errorf("body %d is %d bytes once wrapped, over the %d byte cap", i, got, commentMaxLen())
				}
				if strings.TrimSpace(b) == "" {
					t.Errorf("body %d is empty", i)
				}
			}
		})
	}
}

// finalize() is the declared last guard, so it has to be safe on its own even
// when a body reaches it mid-markup.
func TestFinalizeClosesWhatItCuts(t *testing.T) {
	body := "text\n\n<details open>\n<summary>x</summary>\n\n<details open>\n<summary>y</summary>\n\n```diff\n" +
		strings.Repeat("-  some removed line\n", 500) +
		"```\n\n</details>\n\n</details>\n\n"
	const budget = 2000
	got := finalize([]string{body}, budget)
	if len(got) != 1 {
		t.Fatalf("got %d bodies, want 1", len(got))
	}
	checkBodyWellFormed(t, 0, got[0])
	if len(got[0]) > budget {
		t.Errorf("truncated body is %d bytes, over the %d byte budget", len(got[0]), budget)
	}
	if !strings.Contains(got[0], "TRUNCATED") {
		t.Error("truncation is not visible to the reader")
	}
	// the marker must sit outside the fence, or it renders as diff text
	fenceIdx := strings.LastIndex(got[0], "```")
	if fenceIdx > strings.Index(got[0], "TRUNCATED") {
		t.Errorf("truncation marker is inside the code fence:\n%s", got[0])
	}
}

// A cap lower than the operator's own preamble used to leave no room to
// truncate into: truncateBytes() returned "" and the marker alone landed over
// budget. commentBudget()'s floor makes finalize()'s invariant unconditional.
func TestAbsurdlyLowCapStillProducesUsableBodies(t *testing.T) {
	setWrapper(t, strings.Repeat("P", 400), "<!-- argo-diff -->")
	for _, cap := range []string{"20", "77", "300", "500"} {
		t.Run("cap="+cap, func(t *testing.T) {
			t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", cap)
			c := CommentMarkdown{Preamble: "**1 of 1 apps with changes**\n"}
			a := c.AppMarkdown(AppMarkdownOpts{Name: "app", SyncStatus: "Synced", HealthStatus: "Healthy"})
			a.AddResourceDiff("apps", "Deployment", "web", "prod", fakeDiff(50))

			budget := commentBudget()
			for i, b := range c.String() {
				checkBodyWellFormed(t, i, b)
				if len(b) > budget {
					t.Errorf("body %d is %d bytes, over the %d byte budget:\n%s", i, len(b), budget, b)
				}
				if strings.TrimSpace(b) == "" {
					t.Errorf("body %d is empty", i)
				}
			}
		})
	}
}

// commentBudget()'s floor may never raise the budget above the real headroom.
// Doing so turns a useless-but-postable body into a 422, and posting nothing is
// worse than posting something tiny. Reachable only through a wrapper large
// enough to eat the whole comment.
func TestBudgetFloorNeverExceedsTheHardMax(t *testing.T) {
	tests := []struct {
		name             string
		preamble         string
		cap              string
		wantFloorEngages bool
	}{
		{"huge wrapper, default cap", strings.Repeat("P", 262000), "", false},
		{"huge wrapper, low cap", strings.Repeat("P", 262000), "5000", false},
		{"small wrapper, absurdly low cap", "ctx", "40", true},
		{"small wrapper, default cap", "ctx", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setWrapper(t, tt.preamble, "<!-- argo-diff -->")
			t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", tt.cap)

			budget := commentBudget()
			if room := githubCommentHardMax - commentWrapperLen(); budget > room {
				t.Errorf("budget %d exceeds the %d bytes GitHub actually leaves - every body is a 422", budget, room)
			}
			if got := budget == minResourceLen; got != tt.wantFloorEngages {
				t.Errorf("floor engaged = %v, want %v (budget %d)", got, tt.wantFloorEngages, budget)
			}

			c := CommentMarkdown{Preamble: "**1 of 1 apps with changes**\n"}
			a := c.AppMarkdown(AppMarkdownOpts{Name: "app", SyncStatus: "Synced", HealthStatus: "Healthy"})
			a.AddResourceDiff("apps", "Deployment", "web", "prod", fakeDiff(50))
			for i, b := range c.String() {
				checkBodyWellFormed(t, i, b)
				if got := wrappedLen(b); got > githubCommentHardMax {
					t.Errorf("body %d is %d bytes once wrapped, over GitHub cap %d - this is a 422", i, got, githubCommentHardMax)
				}
			}
		})
	}
}

// The preamble was the last input to the size budget that nothing bounded.
func TestBoundPreamble(t *testing.T) {
	if got := boundPreamble("short"); got != "short" {
		t.Errorf("boundPreamble(short) = %q, want it untouched", got)
	}
	if got := boundPreamble(strings.Repeat("x", maxPreambleLen)); len(got) != maxPreambleLen {
		t.Errorf("boundPreamble at the limit = %d bytes, want %d", len(got), maxPreambleLen)
	}
	if got := boundPreamble(strings.Repeat("x", 262000)); len(got) != maxPreambleLen {
		t.Errorf("boundPreamble(262000) = %d bytes, want %d", len(got), maxPreambleLen)
	}
	// a cut must not split a rune
	if got := boundPreamble(strings.Repeat("α", maxPreambleLen)); !utf8.ValidString(got) {
		t.Error("boundPreamble split a multi-byte rune")
	}
}
