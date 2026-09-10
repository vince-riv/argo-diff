package config

import (
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// NoticeEnvVar is the environment variable an operator sets to put an advisory
// at the top of every argo-diff PR comment — a deprecation warning, or a note
// that a capability is unavailable in this environment. Multiple notices are
// separated by "|".
const NoticeEnvVar = "ARGO_DIFF_COMMENT_NOTICE"

// noticeSep splits NoticeEnvVar into several notices. It is not a comma or a
// newline because notice text is prose and both appear in ordinary sentences.
const noticeSep = "|"

// maxNotices bounds what AddNotice() can accumulate. Notices share the comment
// budget with the diffs, and a notice raised from a per-application code path
// could otherwise be raised once per application.
const maxNotices = 10

var (
	noticeMu    sync.Mutex
	addedNotice []string
)

// AddNotice raises a comment-level advisory from argo-diff itself, for the case
// where the run succeeded but something about the environment is worth telling
// the reader — an argocd CLI too old for a feature, say. Duplicates are
// dropped, so a per-application code path can call it unconditionally.
//
// Safe to call from any goroutine: internal/argocd diffs applications
// concurrently.
func AddNotice(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	noticeMu.Lock()
	defer noticeMu.Unlock()
	if slices.Contains(addedNotice, s) {
		return
	}
	if len(addedNotice) >= maxNotices {
		log.Warn().Msgf("Dropping comment notice %q: already holding %d", s, maxNotices)
		return
	}
	addedNotice = append(addedNotice, s)
}

// Notices returns every advisory to render at the top of the PR comment: the
// operator's NoticeEnvVar entries first, then anything AddNotice() raised.
//
// The env var is read on call rather than cached in an init(), so tests can
// drive it with t.Setenv — the same reason BypassConnectivityCheck() is a
// function.
func Notices() []string {
	var res []string
	for n := range strings.SplitSeq(os.Getenv(NoticeEnvVar), noticeSep) {
		if n = strings.TrimSpace(n); n != "" {
			res = append(res, n)
		}
	}
	noticeMu.Lock()
	defer noticeMu.Unlock()
	return append(res, addedNotice...)
}

// resetNotices clears what AddNotice() has accumulated. Test-only: the process
// handles one event per run in one-shot mode, and the webhook server's notices
// describe the environment, not the event, so nothing in production resets it.
func resetNotices() {
	noticeMu.Lock()
	defer noticeMu.Unlock()
	addedNotice = nil
}
