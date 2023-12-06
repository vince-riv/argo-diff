package gendiff

import (
	"fmt"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/rs/zerolog/log"
)

// Produces a unified diff of two strings
func UnifiedDiff(srcFile, destFile, from, to string) string {
	edits := myers.ComputeEdits(span.URIFromPath(srcFile), from, to)
	diff := fmt.Sprint(gotextdiff.ToUnified(srcFile, destFile, from, edits))
	log.Trace().Msgf("UnifiedDiff(%s, %s): %s", srcFile, destFile, diff)
	return diff
}
