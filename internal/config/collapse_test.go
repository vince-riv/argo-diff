package config

import "testing"

func TestCommentCollapseMode(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", CollapseExpanded},
		{"auto", CollapseAuto},
		{" AUTO ", CollapseAuto},
		{"expanded", CollapseExpanded},
		{"Collapsed", CollapseCollapsed},
		{"nonsense", CollapseExpanded},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE", tt.in)
			if got := CommentCollapseMode(); got != tt.want {
				t.Errorf("CommentCollapseMode() = %q, want %q", got, tt.want)
			}
		})
	}
}
