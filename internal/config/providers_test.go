package config

import (
	"reflect"
	"testing"
)

func TestScmProviders(t *testing.T) {
	known := []string{"github", "gitlab"}
	tests := []struct {
		name, env   string
		want        []string
		wantDefault bool
		wantErr     bool
	}{
		{"unset", "", []string{"github"}, true, false},
		{"blank", "  ", []string{"github"}, true, false},
		{"gitlab only", "gitlab", []string{"gitlab"}, false, false},
		{"both, mixed case and spaces", " GitHub , GITLAB ", []string{"github", "gitlab"}, false, false},
		{"duplicates and empty entries", "gitlab,,gitlab, github", []string{"gitlab", "github"}, false, false},
		{"typo is fatal", "github,gitlb", nil, false, true},
		{"all is not a provider", "all", nil, false, true},
		{"only commas", ",,", nil, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ScmProvidersEnvVar, tc.env)
			got, isDefault, err := ScmProviders(known)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ScmProviders() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, tc.want) || isDefault != tc.wantDefault {
				t.Errorf("ScmProviders() = %v, default %v; want %v, default %v", got, isDefault, tc.want, tc.wantDefault)
			}
		})
	}
}
