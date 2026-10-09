package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/vince-riv/argo-diff/internal/scm"
)

// stubProvider is a provider with fixed startup answers. The embedded nil
// scm.Provider panics on any method selectProviders must not call.
type stubProvider struct {
	scm.Provider
	name        string
	enabled     bool
	validateErr error
	validated   *bool
}

func (s stubProvider) Name() string            { return s.name }
func (s stubProvider) Enabled() bool           { return s.enabled }
func (s stubProvider) CredentialsHint() string { return s.name + "_TOKEN" }
func (s stubProvider) ValidateConfig() error {
	if s.validated != nil {
		*s.validated = true
	}
	return s.validateErr
}

func names(ps []scm.Provider) string {
	var n []string
	for _, p := range ps {
		n = append(n, p.Name())
	}
	return strings.Join(n, ",")
}

func TestSelectProviders(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		github     bool // credentials present
		gitlab     bool
		want       string
		wantErrHas string
	}{
		{"default is github", "", true, false, "github", ""},
		{"default ignores a stray gitlab token", "", true, true, "github", ""},
		{"default without github credentials", "", false, true, "", "or set ARGO_DIFF_SCM_PROVIDERS"},
		{"gitlab only", "gitlab", false, true, "gitlab", ""},
		{"gitlab only ignores github credentials", "gitlab", true, true, "gitlab", ""},
		{"both", "github,gitlab", true, true, "github,gitlab", ""},
		{"listed without credentials", "github,gitlab", true, false, "", "gitlab is listed in ARGO_DIFF_SCM_PROVIDERS but has no credentials; set one of: gitlab_TOKEN"},
		{"unknown name", "gitlb", true, true, "", `unknown source control provider "gitlb"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ARGO_DIFF_SCM_PROVIDERS", tc.env)
			known := []scm.Provider{
				stubProvider{name: "github", enabled: tc.github},
				stubProvider{name: "gitlab", enabled: tc.gitlab},
			}
			got, err := selectProviders(known)
			if tc.wantErrHas != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrHas) {
					t.Fatalf("selectProviders() error = %v, want one containing %q", err, tc.wantErrHas)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectProviders() error = %v", err)
			}
			if names(got) != tc.want {
				t.Errorf("selectProviders() = %s, want %s", names(got), tc.want)
			}
		})
	}
}

// ValidateConfig runs only for listed providers, so an unlisted provider's
// bad configuration (eg: GitLab's CA file) never stops startup.
func TestSelectProvidersValidatesListedOnly(t *testing.T) {
	t.Setenv("ARGO_DIFF_SCM_PROVIDERS", "github")
	var githubValidated, gitlabValidated bool
	known := []scm.Provider{
		stubProvider{name: "github", enabled: true, validated: &githubValidated},
		stubProvider{name: "gitlab", enabled: true, validateErr: errors.New("bad CA file"), validated: &gitlabValidated},
	}
	if _, err := selectProviders(known); err != nil {
		t.Fatalf("selectProviders() = %v", err)
	}
	if !githubValidated || gitlabValidated {
		t.Errorf("validated github=%v gitlab=%v, want github only", githubValidated, gitlabValidated)
	}

	t.Setenv("ARGO_DIFF_SCM_PROVIDERS", "gitlab")
	if _, err := selectProviders(known); err == nil || !strings.Contains(err.Error(), "invalid gitlab configuration: bad CA file") {
		t.Errorf("selectProviders() = %v, want the gitlab configuration error", err)
	}
}
