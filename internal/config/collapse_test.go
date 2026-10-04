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

func TestCollapseIgnorableActive(t *testing.T) {
	modes := []string{"", "expanded", "collapsed", "auto", "nonsense"}
	flags := map[string]bool{"": true, "true": true, "TRUE": true, " true ": true, "false": false, "yes": true}
	for _, mode := range modes {
		for flag, flagOn := range flags {
			t.Run(mode+"/"+flag, func(t *testing.T) {
				t.Setenv("ARGO_DIFF_COMMENT_COLLAPSE", mode)
				t.Setenv(CollapseIgnorableEnvVar, flag)
				want := mode == "auto" && flagOn
				if got := CollapseIgnorableActive(); got != want {
					t.Errorf("CollapseIgnorableActive() = %v, want %v", got, want)
				}
			})
		}
	}
}
