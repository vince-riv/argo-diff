package github

import (
	"strings"
	"testing"
)

// An advisory notice (argocd.ApplicationResourcesWithChanges.NoticeStr) is
// rendered through the same ArgoAppMarkdown.WarnStr slot as a fatal warning,
// but unlike the fatal case it sits above diffs that must still be readable.
// The fenced block therefore has to close on its own line: error text doesn't
// reliably end in a newline, and an unclosed fence swallows every diff below.
func TestAppMarkdownWarnStrRendersAboveDiffs(t *testing.T) {
	tests := []struct {
		name    string
		warnStr string
	}{
		{"no trailing newline", "Unable to discover app-of-apps children of my-app: exit status 1"},
		{"trailing newline", "Unable to discover app-of-apps children of my-app: exit status 1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CommentMarkdown{}
			a := c.AppMarkdown("my-app", tt.warnStr, "Synced", "Healthy", "")
			a.AddResourceDiff("apps", "Deployment", "web", "prod", "-old\n+new\n")

			bodies := c.String()
			if len(bodies) != 1 {
				t.Fatalf("got %d comment bodies, want 1", len(bodies))
			}
			body := bodies[0]

			warnIdx := strings.Index(body, "exit status 1")
			diffIdx := strings.Index(body, "apps/Deployment")
			if warnIdx < 0 {
				t.Fatal("warning text missing from comment body")
			}
			if diffIdx < 0 {
				t.Fatal("resource diff missing from comment body: a warning must not suppress the diffs")
			}
			if warnIdx > diffIdx {
				t.Error("warning rendered below the diffs, want above them")
			}
			if !strings.Contains(body, "exit status 1\n```") {
				t.Errorf("code fence does not start its own line, so it never closes and hides the diffs below; body:\n%s", body)
			}
			// every fence in the block must be paired, or the rest of the
			// comment renders as code
			if n := strings.Count(body, "```"); n%2 != 0 {
				t.Errorf("got %d code fences, want an even number; body:\n%s", n, body)
			}
		})
	}
}
