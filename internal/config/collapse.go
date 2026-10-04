package config

import (
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

// ARGO_DIFF_COMMENT_COLLAPSE values.
const (
	CollapseAuto      = "auto"
	CollapseExpanded  = "expanded"
	CollapseCollapsed = "collapsed"
)

// CommentCollapseMode returns the resolved ARGO_DIFF_COMMENT_COLLAPSE value:
// one of CollapseAuto, CollapseExpanded or CollapseCollapsed. Empty or
// unknown values fall back to CollapseExpanded (unknown also warns). Read on
// call, not init()-cached, so tests can t.Setenv it.
func CommentCollapseMode() string {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("ARGO_DIFF_COMMENT_COLLAPSE")))
	switch raw {
	case "":
		return CollapseExpanded
	case CollapseAuto, CollapseExpanded, CollapseCollapsed:
		return raw
	}
	log.Warn().Msgf("Unknown ARGO_DIFF_COMMENT_COLLAPSE value %q - using %s", raw, CollapseExpanded)
	return CollapseExpanded
}

// CollapseIgnorableEnvVar toggles the "fold ignorable resources" rule of auto
// collapse mode.
const CollapseIgnorableEnvVar = "ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE"

// CollapseIgnorableActive reports whether auto mode folds ignorable resources
// instead of applying the app/resource count thresholds: the mode is auto and
// ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE is true (the default). Both
// internal/github and internal/ignorable ask this one function so they can
// never disagree. Outside auto mode the flag has no effect.
func CollapseIgnorableActive() bool {
	if CommentCollapseMode() != CollapseAuto {
		return false
	}
	switch raw := strings.ToLower(strings.TrimSpace(os.Getenv(CollapseIgnorableEnvVar))); raw {
	case "", "true":
		return true
	case "false":
		return false
	default:
		log.Warn().Msgf("Unknown %s value %q - using true", CollapseIgnorableEnvVar, raw)
		return true
	}
}
