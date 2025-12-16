package gendiff

import (
	"github.com/akedrou/textdiff"
	"github.com/rs/zerolog/log"
)

// Produces a unified diff of two strings
func UnifiedDiff(srcFile, destFile, from, to string) string {
	diff := textdiff.Unified(srcFile, destFile, from, to)
	log.Trace().Msgf("UnifiedDiff(%s, %s): %s", srcFile, destFile, diff)
	return diff
}
