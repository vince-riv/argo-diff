package comment

import "testing"

func TestParseRefreshCommentKeywords(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty uses defaults", "", []string{"argo diff", "argo-diff"}},
		{"whitespace-only uses defaults", "   ", []string{"argo diff", "argo-diff"}},
		{"only commas uses defaults", ",,,", []string{"argo diff", "argo-diff"}},
		{"commas and whitespace uses defaults", " , , ", []string{"argo diff", "argo-diff"}},
		{"single custom keyword", "refresh me", []string{"refresh me"}},
		{"multiple keywords trimmed and lowercased", "  Argo Diff , DO-IT  ", []string{"argo diff", "do-it"}},
		{"empty entries between commas dropped", "foo,,bar", []string{"foo", "bar"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRefreshCommentKeywords(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("parseRefreshCommentKeywords(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseRefreshCommentKeywords(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestIsRefreshComment(t *testing.T) {
	cases := []struct {
		name     string
		keywords []string
		lowerCtx string
		comment  string
		want     bool
	}{
		{"default keyword matches", []string{"argo diff", "argo-diff"}, "", "argo diff", true},
		{"default keyword with context suffix", []string{"argo diff", "argo-diff"}, "prod", "argo diff prod", true},
		{"unrelated comment", []string{"argo diff", "argo-diff"}, "", "hello world", false},
		{"custom keyword matches", []string{"refresh please"}, "", "refresh please", true},
		{"custom keyword with context suffix", []string{"refresh please"}, "staging", "refresh please staging", true},
		{"defaults no longer match after replacement", []string{"refresh please"}, "", "argo diff", false},
		{"case insensitive input", []string{"argo diff", "argo-diff"}, "", "ARGO DIFF", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			origKeywords := refreshCommentKeywords
			origLowerCtx := lowerContextStr
			defer func() {
				refreshCommentKeywords = origKeywords
				lowerContextStr = origLowerCtx
			}()
			refreshCommentKeywords = tc.keywords
			lowerContextStr = tc.lowerCtx
			if got := IsRefreshComment(tc.comment); got != tc.want {
				t.Errorf("IsRefreshComment(%q) = %v, want %v", tc.comment, got, tc.want)
			}
		})
	}
}
