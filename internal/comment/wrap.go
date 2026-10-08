package comment

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

// What Wrap() puts around every rendered body: the operator preamble, and the
// identifier marker argo-diff finds its own comments by. Set in init(), so tests
// assign them directly.
var (
	contextStr        string
	commentPreamble   string
	commentIdentifier string
)

func init() {
	contextStr = strings.TrimSpace(os.Getenv("ARGO_DIFF_CONTEXT_STR"))
	commentPreamble = boundPreamble("ARGO_DIFF_COMMENT_PREAMBLE", strings.TrimSpace(os.Getenv("ARGO_DIFF_COMMENT_PREAMBLE")))
	if commentPreamble == "" {
		commentPreamble = boundPreamble("ARGO_DIFF_CONTEXT_STR", contextStr)
	}
	commentIdentifier = fmt.Sprintf("<!-- comment produced by argo-diff[%s] -->", contextStr)
}

// SetIdentifierRef embeds ref in the identifier marker, so runs for different
// change requests don't match each other's comments. A provider calls it from
// its init() when it runs in CI (eg: GITHUB_REF under GitHub Actions), before
// any comment is rendered or posted.
func SetIdentifierRef(ref string) {
	commentIdentifier = fmt.Sprintf("<!-- comment produced by argo-diff[%s] - %s -->", contextStr, ref)
}

// Identifier is the HTML marker every argo-diff comment ends with.
func Identifier() string {
	return commentIdentifier
}

// maxPreambleLen bounds ARGO_DIFF_COMMENT_PREAMBLE. README documents "150
// chars or less", so this is generous - it exists to keep the comment budget's
// inputs all bounded, not to police the guideline.
const maxPreambleLen = 4000

// boundPreamble caps the operator preamble. Every other input to the size
// budget is bounded - ErrStr, NoticeStr, each notice, HealthMsg, every resource
// body - and this was the last one that was not. An unbounded preamble is
// subtracted from the budget by commentWrapperLen(), so a large enough one
// leaves no room for content and, past the dialect's HardMax, no room for the
// preamble itself.
//
// name is the environment variable the value actually came from: the preamble
// falls back to ARGO_DIFF_CONTEXT_STR, and telling that operator to shorten an
// ARGO_DIFF_COMMENT_PREAMBLE they never set sends them looking in the wrong
// place.
func boundPreamble(name, s string) string {
	if len(s) <= maxPreambleLen {
		return s
	}
	log.Warn().Msgf("%s is %d bytes - truncating the comment preamble to %d", name, len(s), maxPreambleLen)
	return truncateBytes(s, maxPreambleLen)
}

// Wrap builds the body actually posted: the operator preamble, the rendered
// markdown, and the identifier argo-diff finds its own comments by.
func Wrap(body string) string {
	var b strings.Builder
	if commentPreamble != "" {
		b.WriteString(commentPreamble)
		b.WriteString("\n\n")
	}
	b.WriteString(body)
	b.WriteString("\n\n")
	b.WriteString(commentIdentifier)
	b.WriteString("\n")
	return b.String()
}

// commentWrapperLen is how many bytes Wrap() adds around a body. None of it is
// visible to markdown.go's budget, so String() subtracts it - otherwise a long
// ARGO_DIFF_COMMENT_PREAMBLE plus a full-size body is a 422 from the API and no
// comment at all. Derived from Wrap() rather than recomputed, so the two cannot
// drift apart.
func commentWrapperLen() int {
	return len(Wrap(""))
}
