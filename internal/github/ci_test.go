package github

import "testing"

func TestEventFromCIEnv(t *testing.T) {
	setEnv := func(t *testing.T, event, ref, repo string) {
		t.Helper()
		t.Setenv("GITHUB_EVENT_NAME", event)
		t.Setenv("GITHUB_REF", ref)
		t.Setenv("GITHUB_REPOSITORY", repo)
		t.Setenv("REPO_DEFAULT_REF", "main")
		t.Setenv("GITHUB_HEAD_REF", "feature")
		t.Setenv("GITHUB_BASE_REF", "main")
	}

	setEnv(t, "pull_request", "refs/pull/42/merge", "vince-riv/argo-diff")
	evt, err := Provider{}.EventFromCIEnv()
	if err != nil {
		t.Fatalf("EventFromCIEnv() err'd: %v", err)
	}
	if evt.Provider != "github" || evt.RepoOwner != "vince-riv" || evt.RepoName != "argo-diff" || evt.PrNum != 42 ||
		evt.RepoDefaultRef != "main" || evt.ChangeRef != "feature" || evt.BaseRef != "main" || !evt.Refresh {
		t.Errorf("EventFromCIEnv() = %+v", evt)
	}

	for name, env := range map[string][3]string{
		"push event":          {"push", "refs/pull/42/merge", "vince-riv/argo-diff"},
		"branch ref":          {"pull_request", "refs/heads/main", "vince-riv/argo-diff"},
		"short ref":           {"pull_request", "refs/pull", "vince-riv/argo-diff"},
		"repository no owner": {"pull_request", "refs/pull/42/merge", "argo-diff"},
	} {
		t.Run(name, func(t *testing.T) {
			setEnv(t, env[0], env[1], env[2])
			if evt, err := (Provider{}).EventFromCIEnv(); err == nil {
				t.Errorf("EventFromCIEnv() = %+v, want an error", evt)
			}
		})
	}
}

func TestDetectCI(t *testing.T) {
	for val, want := range map[string]bool{"true": true, "": false, "false": false} {
		t.Setenv("GITHUB_ACTIONS", val)
		if got := (Provider{}).DetectCI(); got != want {
			t.Errorf("GITHUB_ACTIONS=%q: DetectCI() = %v, want %v", val, got, want)
		}
	}
}

func TestEnabledAndValidateConfig(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		wantEnabled bool
		wantErr     bool
	}{
		{"nothing set", nil, false, false},
		{"personal access token", map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "x"}, true, false},
		{"token", map[string]string{"GITHUB_TOKEN": "x"}, true, false},
		{"complete app", map[string]string{"GITHUB_APP_ID": "1", "GITHUB_APP_INSTALLATION_ID": "2", "GITHUB_APP_PRIVATE_KEY": "k"}, true, false},
		{"incomplete app", map[string]string{"GITHUB_APP_ID": "1"}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, e := range append([]string{"GITHUB_PERSONAL_ACCESS_TOKEN", "GITHUB_TOKEN"}, appEnvVars...) {
				t.Setenv(e, tc.env[e])
			}
			p := Provider{}
			if got := p.Enabled(); got != tc.wantEnabled {
				t.Errorf("Enabled() = %v, want %v", got, tc.wantEnabled)
			}
			if !tc.wantEnabled {
				return
			}
			if err := p.ValidateConfig(); (err != nil) != tc.wantErr {
				t.Errorf("ValidateConfig() = %v, want error: %v", err, tc.wantErr)
			}
		})
	}
}
