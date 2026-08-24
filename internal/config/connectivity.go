// Package config holds cross-package operator configuration that more than one internal
// package needs to read — starting with the connectivity-check bypass.
package config

import (
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

// BypassEnvVar is the environment variable operators set to skip startup/runtime
// connectivity checks. Value is a comma-separated list of component names
// ("github", "argocd"), or "true"/"all" to skip every check.
const BypassEnvVar = "ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS"

// Component names accepted in ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS.
const (
	ComponentGithub = "github"
	ComponentArgoCD = "argocd"
)

// allComponents is what "true"/"all" expands to.
var allComponents = []string{ComponentGithub, ComponentArgoCD}

// parseBypassList parses raw (the env var value) into the set of bypassed
// components and any tokens it didn't recognize. It is pure and does no
// logging, so it's safe to call on every event without spamming warnings, and
// is fully table-testable on its own.
func parseBypassList(raw string) (map[string]bool, []string) {
	bypassed := map[string]bool{}
	var unknown []string
	for tok := range strings.SplitSeq(raw, ",") {
		tok = strings.ToLower(strings.TrimSpace(tok))
		if tok == "" {
			continue
		}
		switch tok {
		case "true", "all":
			for _, c := range allComponents {
				bypassed[c] = true
			}
		case "false", "none":
			// recognized no-op
		case ComponentGithub, ComponentArgoCD:
			bypassed[tok] = true
		default:
			unknown = append(unknown, tok)
		}
	}
	return bypassed, unknown
}

// BypassConnectivityCheck reports whether component's connectivity check should
// be skipped, per ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS. A plain function rather
// than an init()-cached value so tests can use t.Setenv — same rationale as
// maxWorkers() in internal/argocd/concurrency.go.
func BypassConnectivityCheck(component string) bool {
	bypassed, _ := parseBypassList(os.Getenv(BypassEnvVar))
	return bypassed[component]
}

// LogBypassConfig logs the resolved bypass configuration once at startup:
// a warning for any unrecognized token, and an info line naming what's bypassed
// (if anything). Call this once from cmd/main.go, not from per-event code paths.
func LogBypassConfig() {
	raw := os.Getenv(BypassEnvVar)
	if raw == "" {
		return
	}
	bypassed, unknown := parseBypassList(raw)
	for _, tok := range unknown {
		log.Warn().Msgf("%s: unrecognized value %q ignored; allowed values are \"github\", \"argocd\", \"true\", \"all\"", BypassEnvVar, tok)
	}
	if len(bypassed) == 0 {
		return
	}
	var comps []string
	for _, c := range allComponents {
		if bypassed[c] {
			comps = append(comps, c)
		}
	}
	log.Warn().Msgf("%s: connectivity checks bypassed for: %s", BypassEnvVar, strings.Join(comps, ", "))
}
