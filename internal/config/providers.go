package config

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// ScmProvidersEnvVar lists the source control providers argo-diff runs for.
const ScmProvidersEnvVar = "ARGO_DIFF_SCM_PROVIDERS"

// DefaultScmProviders is what an unset or empty ARGO_DIFF_SCM_PROVIDERS means:
// GitHub only, as before multi-provider support.
const DefaultScmProviders = "github"

// ScmProviders parses ARGO_DIFF_SCM_PROVIDERS: a comma-separated,
// case-insensitive, whitespace-trimmed list of provider names, each one of
// known. Unset or empty means DefaultScmProviders. An unknown name is an
// error, not a warning: this list decides what runs, so a typo must not turn a
// provider off without a word. Duplicates and empty entries are dropped.
//
// It also reports whether the value came from the default, so an error about
// a listed provider can say the operator never listed it.
func ScmProviders(known []string) (names []string, isDefault bool, err error) {
	raw := strings.TrimSpace(os.Getenv(ScmProvidersEnvVar))
	if raw == "" {
		raw, isDefault = DefaultScmProviders, true
	}
	for tok := range strings.SplitSeq(raw, ",") {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" || slices.Contains(names, tok) {
			continue
		}
		if !slices.Contains(known, tok) {
			return nil, isDefault, fmt.Errorf("%s: unknown source control provider %q; known providers are %s", ScmProvidersEnvVar, tok, strings.Join(known, ", "))
		}
		names = append(names, tok)
	}
	if len(names) == 0 {
		return nil, isDefault, fmt.Errorf("%s=%q names no source control provider; known providers are %s", ScmProvidersEnvVar, raw, strings.Join(known, ", "))
	}
	return names, isDefault, nil
}
