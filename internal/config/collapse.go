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
