package comment

import (
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

// defaultRefreshCommentKeywords is used when ARGO_DIFF_REFRESH_COMMENT_KEYWORDS is unset, blank, or
// parses to zero usable keywords (eg: a value of just ",").
const defaultRefreshCommentKeywords = "argo diff,argo-diff"

// Set in init(), so tests assign them directly.
var (
	lowerContextStr        string
	refreshCommentKeywords []string
)

func init() {
	lowerContextStr = strings.ToLower(strings.TrimSpace(os.Getenv("ARGO_DIFF_CONTEXT_STR")))
	refreshCommentKeywords = parseRefreshCommentKeywords(os.Getenv("ARGO_DIFF_REFRESH_COMMENT_KEYWORDS"))
}

// parseRefreshCommentKeywords splits a comma-separated ARGO_DIFF_REFRESH_COMMENT_KEYWORDS value into
// lower-cased, trimmed keywords, falling back to defaultRefreshCommentKeywords when raw is blank or
// parses to no usable keywords.
func parseRefreshCommentKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultRefreshCommentKeywords
	}
	var keywords []string
	for _, keyword := range strings.Split(raw, ",") {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	if len(keywords) == 0 {
		log.Warn().Msg("ARGO_DIFF_REFRESH_COMMENT_KEYWORDS parsed to zero keywords; using defaults")
		return strings.Split(defaultRefreshCommentKeywords, ",")
	}
	return keywords
}

// IsRefreshComment reports whether a comment on a change request should trigger a refresh. It
// matches any keyword in refreshCommentKeywords (parsed once in init() from
// ARGO_DIFF_REFRESH_COMMENT_KEYWORDS, default "argo diff,argo-diff"), each optionally followed by a
// trailing ARGO_DIFF_CONTEXT_STR suffix (eg: "argo diff prod") so one comment can target a single
// instance in a multi-instance setup. Every provider's webhook parsing uses it.
func IsRefreshComment(comment string) bool {
	input := strings.ToLower(strings.TrimSpace(comment))
	for _, keyword := range refreshCommentKeywords {
		if input == keyword || input == keyword+" "+lowerContextStr {
			return true
		}
	}
	return false
}
