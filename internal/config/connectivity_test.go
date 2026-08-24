package config

import (
	"reflect"
	"sort"
	"testing"
)

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestParseBypassList(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantBypass  []string
		wantUnknown []string
	}{
		{"empty", "", nil, nil},
		{"github", "github", []string{"github"}, nil},
		{"argocd", "argocd", []string{"argocd"}, nil},
		{"true", "true", []string{"argocd", "github"}, nil},
		{"all", "all", []string{"argocd", "github"}, nil},
		{"false", "false", nil, nil},
		{"none", "none", nil, nil},
		{"mixed case with spaces", "GitHub, ARGOCD", []string{"argocd", "github"}, nil},
		{"empty tokens", " ,, github ,", []string{"github"}, nil},
		{"one unknown mixed with known", "github,gitlab", []string{"github"}, []string{"gitlab"}},
		{"unknown only", "gitlab", nil, []string{"gitlab"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bypassed, unknown := parseBypassList(tc.raw)
			if got := sortedKeys(bypassed); !reflect.DeepEqual(got, tc.wantBypass) {
				t.Errorf("parseBypassList(%q) bypassed = %v, want %v", tc.raw, got, tc.wantBypass)
			}
			if !reflect.DeepEqual(unknown, tc.wantUnknown) {
				t.Errorf("parseBypassList(%q) unknown = %v, want %v", tc.raw, unknown, tc.wantUnknown)
			}
		})
	}
}

func TestBypassConnectivityCheck(t *testing.T) {
	tests := []struct {
		name          string
		envVal        string
		component     string
		wantBypassed  bool
		otherWantsToo bool
	}{
		{"unset", "", ComponentGithub, false, false},
		{"github only bypasses github", "github", ComponentGithub, true, false},
		{"github only does not bypass argocd", "github", ComponentArgoCD, false, false},
		{"true bypasses both", "true", ComponentGithub, true, true},
		{"all bypasses both", "all", ComponentArgoCD, true, true},
		{"false bypasses nothing", "false", ComponentGithub, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(BypassEnvVar, tc.envVal)
			if got := BypassConnectivityCheck(tc.component); got != tc.wantBypassed {
				t.Errorf("BypassConnectivityCheck(%q) with env %q = %v, want %v", tc.component, tc.envVal, got, tc.wantBypassed)
			}
		})
	}
}

// LogBypassConfig just needs to not panic across the interesting inputs; its
// output is exercised indirectly via parseBypassList's table above.
func TestLogBypassConfigDoesNotPanic(t *testing.T) {
	for _, v := range []string{"", "github", "true", "all", "false", "gitlab", "github,gitlab"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv(BypassEnvVar, v)
			LogBypassConfig()
		})
	}
}
