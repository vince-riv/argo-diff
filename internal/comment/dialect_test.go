package comment

import (
	"fmt"
	"strings"
	"testing"
)

// renderMixed builds a comment that exercises every block the renderer emits:
// comment-level notices, a failed app, an app with a notice, and enough
// resources - one of them oversized - to force splitting under a low cap.
func renderMixed(d Dialect) []string {
	c := CommentMarkdown{Dialect: d, Preamble: "**3 of 3 apps with changes** compared to live state\n"}
	c.Notices = []string{"a comment-level notice"}
	c.AppMarkdown(AppMarkdownOpts{Name: "broken", ErrStr: "rpc error: boom", SyncStatus: "Unknown", HealthStatus: "Missing"})
	n := c.AppMarkdown(AppMarkdownOpts{Name: "noticed", NoticeStr: "children not enumerated", SyncStatus: "OutOfSync", HealthStatus: "Healthy"})
	n.AddResourceDiff("apps", "Deployment", "web", "prod", fakeDiff(20))
	big := c.AppMarkdown(AppMarkdownOpts{Name: "big", SyncStatus: "OutOfSync", HealthStatus: "Healthy"})
	for j := range 6 {
		big.AddResourceDiff("", "ConfigMap", fmt.Sprintf("cm-%d", j), "prod", fakeDiff(30))
	}
	big.AddResourceDiff("apps", "Deployment", "huge", "prod", fakeDiff(5000))
	return c.String()
}

// checkBodyWellFormed has to hold for every dialect, not just the default one:
// each dialect brings its own HardMax, and so its own split points.
func TestEveryDialectWellFormed(t *testing.T) {
	setWrapper(t, "operator preamble", "<!-- comment produced by argo-diff[ci] -->")
	for _, d := range Dialects {
		for _, maxChars := range []string{"", "9000"} {
			t.Run(fmt.Sprintf("%s/max=%q", d.Name, maxChars), func(t *testing.T) {
				t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", maxChars)
				bodies := renderMixed(d)
				for i, b := range bodies {
					checkBodyWellFormed(t, i, b)
					if got := wrappedLen(b); got > commentMaxLen(d) {
						t.Errorf("body %d is %d bytes once wrapped, over the %d byte cap", i, got, commentMaxLen(d))
					}
				}
				if maxChars != "" && len(bodies) < 2 {
					t.Errorf("got %d bodies under a %s byte cap, want a split", len(bodies), maxChars)
				}
			})
		}
	}
}

// A dialect's HardMax is the budget when ARGO_DIFF_COMMENT_MAX_CHARS is unset,
// and the cap the variable cannot raise.
func TestDialectHardMaxIsUsed(t *testing.T) {
	setWrapper(t, "", "<!-- argo-diff -->")
	small := Dialect{Name: "small", HardMax: 6000}
	for _, maxChars := range []string{"", "50000"} {
		t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", maxChars)
		if got := commentMaxLen(small); got != small.HardMax {
			t.Errorf("ARGO_DIFF_COMMENT_MAX_CHARS=%q: commentMaxLen() = %d, want the dialect's %d", maxChars, got, small.HardMax)
		}
		bodies := renderMixed(small)
		if len(bodies) < 2 {
			t.Errorf("ARGO_DIFF_COMMENT_MAX_CHARS=%q: got %d bodies, want a split at the dialect's HardMax", maxChars, len(bodies))
		}
		for i, b := range bodies {
			checkBodyWellFormed(t, i, b)
			if got := wrappedLen(b); got > small.HardMax {
				t.Errorf("body %d is %d bytes once wrapped, over the dialect's %d", i, got, small.HardMax)
			}
		}
	}
}

// The zero-value Dialect renders exactly like GitHub's, since GitHub is the
// default provider.
func TestZeroDialectIsGitHub(t *testing.T) {
	setWrapper(t, "", "<!-- argo-diff -->")
	t.Setenv("ARGO_DIFF_COMMENT_MAX_CHARS", "9000")
	got := strings.Join(renderMixed(Dialect{}), "\x00")
	want := strings.Join(renderMixed(GitHub), "\x00")
	if got != want {
		t.Error("zero-value Dialect renders differently from GitHub")
	}
}
