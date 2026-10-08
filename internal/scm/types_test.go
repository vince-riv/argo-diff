package scm

import "testing"

func TestRepoRefPaths(t *testing.T) {
	cases := []struct {
		ref      RepoRef
		fullPath string
		str      string
	}{
		{RepoRef{Owner: "vince-riv", Name: "argo-diff"}, "vince-riv/argo-diff", "vince-riv/argo-diff"},
		{RepoRef{Host: "github.com", Owner: "vince-riv", Name: "argo-diff"}, "vince-riv/argo-diff", "github.com/vince-riv/argo-diff"},
		// GitLab nested groups: the owner carries the whole namespace path
		{RepoRef{Host: "gitlab.com", Owner: "group/subgroup", Name: "project"}, "group/subgroup/project", "gitlab.com/group/subgroup/project"},
	}
	for _, tc := range cases {
		if got := tc.ref.FullPath(); got != tc.fullPath {
			t.Errorf("%+v.FullPath() = %q, want %q", tc.ref, got, tc.fullPath)
		}
		if got := tc.ref.String(); got != tc.str {
			t.Errorf("%+v.String() = %q, want %q", tc.ref, got, tc.str)
		}
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusPending, StatusSuccess, StatusFailure, StatusError} {
		if !s.Valid() {
			t.Errorf("Status(%q).Valid() = false, want true", s)
		}
	}
	// "failed" is GitLab's spelling; providers map onto their own states, so
	// it is not a neutral state
	for _, s := range []Status{"", "failed", "running", "PENDING"} {
		if s.Valid() {
			t.Errorf("Status(%q).Valid() = true, want false", s)
		}
	}
}
