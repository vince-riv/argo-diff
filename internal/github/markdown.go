package github

import (
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
)

// githubCommentHardMax is GitHub's own cap on an issue comment body, per
// https://github.com/orgs/community/discussions/27190#discussioncomment-3254953
// Nothing this package emits may exceed it: a body over the cap is a 422 from
// the API, which means no comment at all.
const githubCommentHardMax = 262144

const (
	// defaultIndexCount renders the summary index once two or more applications
	// have an entry. -1 always renders it, 0 never does.
	defaultIndexCount = 2
	// maxIndexRows caps the summary index so a change matching hundreds of
	// applications can't crowd out the diffs, mirroring timeoutMarkdown()'s
	// name cap in internal/process_event.
	maxIndexRows = 50
	// maxNoticeLen bounds every advisory/error string. They come from
	// err.Error() and from operator input, both unbounded, and they compete
	// with the diffs for the same comment budget.
	maxNoticeLen = 4000
	// maxHealthMsgLen bounds the ArgoCD health message, which is arbitrary
	// text from the cluster.
	maxHealthMsgLen = 500
	// defaultCollapse* are the "auto" collapse thresholds. Below them a comment
	// is small enough to read fully expanded; above them an open wall of diffs
	// hides the shape of the change. They're only consulted in "auto" mode -
	// the default mode is collapseExpanded, which ignores them entirely.
	defaultCollapseAppCount      = 3
	defaultCollapseResourceCount = 5
	// minResourceLen keeps a very low ARGO_DIFF_COMMENT_MAX_CHARS from
	// collapsing every diff into the "too large" marker.
	minResourceLen = 512
)

// ARGO_DIFF_COMMENT_COLLAPSE values.
const (
	collapseAuto      = "auto"
	collapseExpanded  = "expanded"
	collapseCollapsed = "collapsed"
)

const (
	detailsClose    = "</details>\n\n"
	appSeparator    = "\n---\n\n"
	continuedMarker = "\n\n_[Continued in next comment]_\n"
	truncatedMarker = "\n\n`<<< TRUNCATED - comment size limit reached >>>`\n"
	// truncFenceClose and truncTailReserve cover what finalize() appends after
	// its cut: a closing code fence, and up to two </details> - the deepest
	// nesting this renderer produces (application, then resource).
	truncFenceClose   = "\n```\n"
	truncTailReserve  = len(truncFenceClose) + 2*len(detailsClose)
	partHeaderReserve = 64
	// splitReserve is every byte String() appends after a fit check has already
	// passed - the closing tags, the continuation marker, and the part header
	// finalize() prepends. Reserving it up front is what stops those unchecked
	// appends from pushing a body past the budget.
	splitReserve = len(continuedMarker) + 2*len(detailsClose) + partHeaderReserve
)

var argocdUiUrl string

func init() {
	argocdUiUrl = os.Getenv("ARGOCD_UI_BASE_URL")
	if argocdUiUrl == "" {
		log.Warn().Msg("ARGOCD_UI_BASE_URL is not set - links won't be created in comments")
	} else {
		log.Info().Msgf("ARGOCD_UI_BASE_URL is set to %s for comment links", argocdUiUrl)
	}
}

// envInt reads an integer environment variable, warning and falling back to def
// on anything it can't parse. Read on call rather than cached in init() so
// tests can drive it with t.Setenv - the same reason maxWorkers() in
// internal/argocd is a function.
func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Warn().Err(err).Msgf("Failed to decode %s - using %d", key, def)
		return def
	}
	return v
}

// lineMaxChars is the per-line truncation width for diff text.
func lineMaxChars() int {
	n := envInt("COMMENT_LINE_MAX_CHARS", 175)
	if n <= 0 {
		log.Warn().Msgf("COMMENT_LINE_MAX_CHARS must be positive - using 175")
		return 175
	}
	return n
}

// commentMaxLen is the cap on one rendered comment, including the preamble and
// marker Comment() wraps around it. It can be lowered (a GitHub Enterprise
// instance with a smaller cap, or forcing a split in testing) but never raised
// past what the API accepts.
func commentMaxLen() int {
	n := envInt("ARGO_DIFF_COMMENT_MAX_CHARS", githubCommentHardMax)
	if n <= 0 || n > githubCommentHardMax {
		if n != githubCommentHardMax {
			log.Warn().Msgf("ARGO_DIFF_COMMENT_MAX_CHARS %d is out of range - using %d", n, githubCommentHardMax)
		}
		return githubCommentHardMax
	}
	return n
}

// indexCount decides when the summary index renders: -1 always, 0 never, and
// any positive n once n or more applications have an entry.
func indexCount() int {
	return envInt("ARGO_DIFF_COMMENT_INDEX_COUNT", defaultIndexCount)
}

// collapseAppCount is the "auto" mode threshold on application count. Zero or
// negative is invalid - collapseCollapsed is the only way to fold everything -
// so it warns and falls back to the default, the same guard lineMaxChars()
// uses.
func collapseAppCount() int {
	n := envInt("ARGO_DIFF_COMMENT_COLLAPSE_APP_COUNT", defaultCollapseAppCount)
	if n <= 0 {
		log.Warn().Msgf("ARGO_DIFF_COMMENT_COLLAPSE_APP_COUNT must be positive - using %d", defaultCollapseAppCount)
		return defaultCollapseAppCount
	}
	return n
}

// collapseResourceCount is the "auto" mode threshold on a single application's
// resource count. Same non-positive guard as collapseAppCount.
func collapseResourceCount() int {
	n := envInt("ARGO_DIFF_COMMENT_COLLAPSE_RESOURCE_COUNT", defaultCollapseResourceCount)
	if n <= 0 {
		log.Warn().Msgf("ARGO_DIFF_COMMENT_COLLAPSE_RESOURCE_COUNT must be positive - using %d", defaultCollapseResourceCount)
		return defaultCollapseResourceCount
	}
	return n
}

func collapseMode() string {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("ARGO_DIFF_COMMENT_COLLAPSE")))
	switch raw {
	case "":
		return collapseExpanded
	case collapseAuto, collapseExpanded, collapseCollapsed:
		return raw
	}
	log.Warn().Msgf("Unknown ARGO_DIFF_COMMENT_COLLAPSE value %q - using %s", raw, collapseExpanded)
	return collapseExpanded
}

// commentBudget is how much rendered markdown one comment body may hold, after
// subtracting what Comment() adds around it.
//
// The floor keeps finalize()'s invariant unconditional: below roughly
// len(truncatedMarker)+truncTailReserve there is no room to truncate into, so
// truncateBytes() returns "" and the marker alone lands over budget. Reaching
// that needs ARGO_DIFF_COMMENT_MAX_CHARS set below the operator's own
// ARGO_DIFF_COMMENT_PREAMBLE, so the only cap breached is one they set
// themselves - but the warning is the part they need, since nothing else says
// their preamble has eaten the comment.
func commentBudget() int {
	n := commentMaxLen() - commentWrapperLen()
	if n >= minResourceLen {
		return n
	}
	log.Warn().Msgf("ARGO_DIFF_COMMENT_MAX_CHARS %d leaves only %d bytes after the preamble and marker",
		commentMaxLen(), n)
	// The floor may never exceed what GitHub itself accepts: raising the budget
	// past the real headroom turns a useless-but-postable body into a 422, and
	// posting nothing is worse than posting something tiny. boundPreamble()
	// keeps this out of reach for the preamble, but commentIdentifier carries
	// ARGO_DIFF_CONTEXT_STR, which stays unbounded because comment matching
	// depends on it.
	if room := githubCommentHardMax - commentWrapperLen(); room < minResourceLen {
		return max(room, 0)
	}
	return minResourceLen
}

// truncateBytes cuts s to at most max bytes without splitting a UTF-8 rune.
func truncateBytes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

// truncateNotice bounds an advisory or error string. Both are built from
// unbounded input and share the comment budget with the diffs.
func truncateNotice(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxNoticeLen {
		return s
	}
	return truncateBytes(s, maxNoticeLen) + "\n…[truncated]"
}

// blockquote prefixes every line of s with "> " so it can sit inside a GitHub
// alert. A fenced block inside an alert only renders when every one of its
// lines carries the prefix, blank lines included.
func blockquote(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// alert renders a GitHub alert block. kind is NOTE, WARNING or CAUTION, which
// GitHub styles with a distinct icon and colour each - that difference is what
// keeps an advisory notice from reading like a fatal error.
func alert(kind, body string) string {
	return fmt.Sprintf("> [!%s]\n%s\n", kind, blockquote(body))
}

func detailsTag(open bool) string {
	if open {
		return "<details open>"
	}
	return "<details>"
}

// syncString and healthString use literal Unicode emoji rather than GitHub
// :shortcodes: because these strings also render inside raw HTML <summary>
// elements, where shortcode substitution is not reliable.
func syncString(s string) string {
	switch s {
	case "Synced":
		// desired and live states match
		return "✅ Synced"
	case "OutOfSync":
		// there is a drift between desired and live states
		return "⚠️ OutOfSync"
	case "Unknown":
		// the sync state could not be reliably determined
		return "❓ Unknown"
	default:
		return "⁉️ " + s
	}
}

func healthString(s string) string {
	emoji := "⁉️"
	switch s {
	case "Unknown":
		// health assessment failed and the actual health status is unknown
		emoji = "❓"
	case "Progressing":
		// not healthy, but still has a chance to reach a healthy state
		emoji = "⏳"
	case "Healthy":
		emoji = "💚"
	case "Suspended":
		// suspended or paused, eg: a suspended CronJob
		emoji = "🚫"
	case "Degraded":
		// status indicates failure, or it could not reach a healthy state in time
		emoji = "❌"
	case "Missing":
		// the resource is missing from the cluster
		emoji = "👻"
	}
	return emoji + " " + s
}

// truncateLines cuts individual lines to maxLen. The result always ends in
// exactly one newline, which is what lets a caller close a code fence on its
// own line without inspecting the original string.
func truncateLines(s string, maxLen int) string {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		if len(line) > maxLen {
			b.WriteString(truncateBytes(line, maxLen))
			b.WriteString("...[TRUNCATED]")
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// diffStats counts added and removed lines in a unified diff, skipping the
// ---/+++ file headers so they don't read as one added and one removed line.
func diffStats(diffStr string) (added, removed int) {
	for line := range strings.SplitSeq(diffStr, "\n") {
		switch {
		// match the file headers exactly, trailing space included: a removed
		// YAML document separator renders as "----" and must still count
		case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	return added, removed
}

// resourceMarkdown holds a resource's rendered pieces rather than a finished
// block, because whether it renders folded depends on totals that aren't known
// until String() runs.
type resourceMarkdown struct {
	Summary string // inner HTML of <summary>
	Body    string // the ```diff fence, or the too-large marker
}

func (r resourceMarkdown) render(open bool) string {
	// no leading newline: whatever precedes a resource block - the app overview
	// or the previous resource's close - already ends in a blank line
	md := detailsTag(open) + "\n"
	md += "<summary>" + r.Summary + "</summary>\n\n"
	md += r.Body
	md += detailsClose
	return md
}

type ArgoAppMarkdown struct {
	AppName string
	// ErrStr is fatal: this application's diff failed, so there are no
	// trustworthy resources to show. Renders as a red [!CAUTION] alert.
	ErrStr string
	// NoticeStr is advisory: the diff is good, but something alongside it
	// degraded. Renders as a blue [!NOTE] alert above the diffs.
	NoticeStr    string
	SyncStatus   string
	HealthStatus string
	HealthMsg    string
	Resources    []resourceMarkdown
}

type CommentMarkdown struct {
	Preamble string
	// Notices are comment-level advisories - a deprecation, or a capability
	// disabled by the environment. Rendered as [!NOTE] alerts on the first body.
	Notices  []string
	ArgoApps []*ArgoAppMarkdown
}

// AppMarkdownOpts carries one application's identity and status. It's a struct
// rather than positional arguments because six adjacent strings are too easy to
// transpose - and confusing ErrStr with NoticeStr inverts the severity a reader
// sees.
type AppMarkdownOpts struct {
	Name         string
	ErrStr       string
	NoticeStr    string
	SyncStatus   string
	HealthStatus string
	HealthMsg    string
}

func (c *CommentMarkdown) AppMarkdown(o AppMarkdownOpts) *ArgoAppMarkdown {
	a := &ArgoAppMarkdown{
		AppName:      o.Name,
		ErrStr:       truncateNotice(o.ErrStr),
		NoticeStr:    truncateNotice(o.NoticeStr),
		SyncStatus:   o.SyncStatus,
		HealthStatus: o.HealthStatus,
		HealthMsg:    truncateBytes(strings.TrimSpace(o.HealthMsg), maxHealthMsgLen),
	}
	c.ArgoApps = append(c.ArgoApps, a)
	return a
}

func (a ArgoAppMarkdown) url() string {
	if argocdUiUrl == "" {
		return ""
	}
	return fmt.Sprintf("%s/applications/argocd/%s", argocdUiUrl, a.AppName)
}

// summaryLine is the one-line <summary> for an application: its name, what
// happened to it, and both statuses. It replaces the three stacked lines the
// comment used to carry, so a reader can scan a folded comment without
// expanding anything.
func (a ArgoAppMarkdown) summaryLine(continued bool) string {
	parts := []string{"<b>" + html.EscapeString(a.AppName) + "</b>"}
	switch {
	case continued:
		parts = append(parts, "continued")
	case a.ErrStr != "":
		parts = append(parts, "❗ diff failed")
	default:
		parts = append(parts, fmt.Sprintf("%d changed", len(a.Resources)))
	}
	parts = append(parts, syncString(a.SyncStatus), healthString(a.HealthStatus))
	return strings.Join(parts, " · ")
}

// alerts renders this application's advisory and error blocks. String() emits
// them OUTSIDE the app's <details>, and they name the application because of
// it.
//
// GitHub renders its coloured alert boxes only at the top level of a document.
// Inside any HTML element - <details> included - the extension is skipped and
// the block degrades to a plain blockquote with a literal "[!NOTE]" as its
// first line. No amount of blank lines or wrapper markup changes that, so the
// alerts have to live outside the block. Keeping them there is also better
// behaviour: a folded application can no longer hide the reason its diffs are
// missing or suspect.
func (a ArgoAppMarkdown) alerts() string {
	md := ""
	if a.ErrStr != "" {
		body := fmt.Sprintf("**%s** - diff failed, so no manifests are shown. %s · %s\n\n```\n%s\n```",
			a.AppName, syncString(a.SyncStatus), healthString(a.HealthStatus), a.ErrStr)
		if u := a.url(); u != "" {
			body += fmt.Sprintf("\n\n[Open in ArgoCD ↗](%s)", u)
		}
		md += alert("CAUTION", body)
	}
	if a.NoticeStr != "" {
		md += alert("NOTE", fmt.Sprintf("**%s** - %s", a.AppName, a.NoticeStr))
	}
	return md
}

// OverviewStr is the application's <details> header. It carries no alerts - see
// alerts() for why those sit outside the block.
func (a ArgoAppMarkdown) OverviewStr(continued, open bool) string {
	md := detailsTag(open) + "\n"
	md += "<summary>" + a.summaryLine(continued) + "</summary>\n\n"
	if u := a.url(); u != "" {
		md += fmt.Sprintf("[Open in ArgoCD ↗](%s)\n\n", u)
	}
	if a.HealthMsg != "" {
		md += "<sub>" + html.EscapeString(a.HealthMsg) + "</sub>\n\n"
	}
	return md
}

// maxResourceBodyLen is the largest a single resource's diff may render: what
// is left of one body once this application's continuation header and the
// resource's own markup are accounted for. Deriving it from the budget rather
// than hardcoding it is what stops a lowered ARGO_DIFF_COMMENT_MAX_CHARS from
// leaving a resource no body can hold.
func (a ArgoAppMarkdown) maxResourceBodyLen(summary string) int {
	const markup = 64 // the <details>/<summary> scaffolding around the body
	// Reserve everything String() emits ahead of the first resource: the rule,
	// this app's alerts - up to maxNoticeLen each, so ~8KB - and the header.
	// OverviewStr(false, true) is the first-body form and the larger of the
	// two. ErrStr/NoticeStr are set by AppMarkdown() before any
	// AddResourceDiff() call, so alerts() is complete here.
	head := len(appSeparator) + len(a.alerts()) + len(a.OverviewStr(false, true))
	n := commentBudget() - splitReserve - head - len(summary) - markup
	if n < minResourceLen {
		return minResourceLen
	}
	return n
}

func (a *ArgoAppMarkdown) AddResourceDiff(group, kind, name, ns, diffStr string) {
	gk := kind
	if group != "" {
		gk = group + "/" + kind
	}
	summary := fmt.Sprintf("<code>%s</code> %s", html.EscapeString(gk), html.EscapeString(ns+"/"+name))
	if added, removed := diffStats(diffStr); added > 0 || removed > 0 {
		summary += fmt.Sprintf(" · <b>+%d −%d</b>", added, removed)
	}
	body := ""
	if strings.TrimSpace(diffStr) != "" {
		body = "```diff\n" + truncateLines(diffStr, lineMaxChars()) + "```\n\n"
	}
	if len(body) > a.maxResourceBodyLen(summary) {
		body = "`<<< DIFF TOO LARGE TO DISPLAY >>>`\n\n"
	}
	a.Resources = append(a.Resources, resourceMarkdown{Summary: summary, Body: body})
}

// appOpen and resourceOpen decide whether a block renders folded.
func (c CommentMarkdown) appOpen(a *ArgoAppMarkdown) bool {
	switch collapseMode() {
	case collapseExpanded:
		return true
	case collapseCollapsed:
		return false
	}
	// never fold an application carrying an advisory or an error: its alert
	// says these diffs deserve a second look, so hiding them behind a fold
	// works against the reader
	if a.NoticeStr != "" || a.ErrStr != "" {
		return true
	}
	return len(c.ArgoApps) <= collapseAppCount()
}

func (c CommentMarkdown) resourceOpen(a *ArgoAppMarkdown) bool {
	switch collapseMode() {
	case collapseExpanded:
		return true
	case collapseCollapsed:
		return false
	}
	return len(a.Resources) <= collapseResourceCount()
}

// indexTable renders the summary index: one row per application, so a reader
// sees every app, its change count and its statuses without expanding anything.
// Gated by ARGO_DIFF_COMMENT_INDEX_COUNT.
func (c CommentMarkdown) indexTable() string {
	n := indexCount()
	if n == 0 || len(c.ArgoApps) == 0 || (n > 0 && len(c.ArgoApps) < n) {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n| Application | Changed | Sync | Health |\n| --- | --: | --- | --- |\n")
	for i, a := range c.ArgoApps {
		if i >= maxIndexRows {
			fmt.Fprintf(&b, "| _…and %d more_ | | | |\n", len(c.ArgoApps)-maxIndexRows)
			break
		}
		name := html.EscapeString(a.AppName)
		if u := a.url(); u != "" {
			name = fmt.Sprintf("[%s](%s)", name, u)
		}
		changed := strconv.Itoa(len(a.Resources))
		if a.ErrStr != "" {
			changed = "❗"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", name, changed, syncString(a.SyncStatus), healthString(a.HealthStatus))
	}
	return b.String()
}

// String renders the comment, splitting it across as many bodies as it takes.
// Every append is checked against a budget that already accounts for what
// Comment() wraps around each body and for the closing tags and markers added
// after a check has passed - see splitReserve.
func (c CommentMarkdown) String() []string {
	budget := commentBudget()
	limit := budget - splitReserve
	if limit < minResourceLen {
		limit = minResourceLen
	}
	var res []string
	md := c.Preamble
	for _, n := range c.Notices {
		if n = truncateNotice(n); n != "" {
			md += alert("NOTE", n)
		}
	}
	md += c.indexTable()

	flush := func() {
		if md != "" {
			res = append(res, md)
			md = ""
		}
	}

	for _, a := range c.ArgoApps {
		appOpen := c.appOpen(a)
		// the rule and the alerts precede the block, so the alerts stay at the
		// top level of the document where GitHub will render them
		head := appSeparator + a.alerts()
		if len(a.Resources) == 0 {
			// a failed application is its alert - there are no diffs to fold
			if a.ErrStr == "" {
				head += a.OverviewStr(false, appOpen) + detailsClose
			}
			if len(md)+len(head) > limit {
				flush()
			}
			md += head
			continue
		}
		overview := head + a.OverviewStr(false, appOpen)
		resOpen := c.resourceOpen(a)
		// look ahead to the first resource: opening an application block at the
		// tail of a body that can't hold any of its diffs just wastes a header.
		// The second term is the same progress guard the resource loop uses - if
		// it doesn't fit in an empty body either, splitting only emits the
		// preamble on its own and still doesn't fit.
		first := a.Resources[0].render(resOpen)
		if len(md)+len(overview)+len(first) > limit && len(overview)+len(first) <= limit {
			flush()
		}
		md += overview
		placed := 0
		for _, r := range a.Resources {
			rendered := r.render(resOpen)
			// placed > 0 guarantees progress: a body holding nothing but this
			// app's header has no more room than a fresh one would, so
			// splitting there would emit an empty body and still not fit
			if placed > 0 && len(md)+len(rendered) > limit {
				md += detailsClose + continuedMarker
				res = append(res, md)
				md = a.OverviewStr(true, appOpen)
				placed = 0
			}
			md += rendered
			placed++
		}
		md += detailsClose
	}
	res = append(res, md)
	return finalize(res, budget)
}

// finalize labels continuation bodies and enforces the hard cap. Nothing above
// this point may emit a body GitHub would reject: a 422 means no comment at
// all, which is worse than a truncated one.
func finalize(bodies []string, budget int) []string {
	if len(bodies) > 1 {
		for i := range bodies {
			bodies[i] = fmt.Sprintf("**Argo-Diff** - part %d of %d\n", i+1, len(bodies)) + bodies[i]
		}
	}
	for i, b := range bodies {
		if len(b) <= budget {
			continue
		}
		log.Warn().Msgf("comment body %d is %d bytes, over the %d byte budget - truncating", i+1, len(b), budget)
		t := truncateBytes(b, budget-len(truncatedMarker)-truncTailReserve)
		// The cut lands at an arbitrary byte, almost always inside a resource.
		// Close what it left open, innermost first, or the fence swallows the
		// truncation marker and the unclosed <details> swallow everything after.
		if strings.Count(t, "```")%2 != 0 {
			t += truncFenceClose
		}
		t += truncatedMarker
		for range strings.Count(t, "<details") - strings.Count(t, "</details>") {
			t += detailsClose
		}
		bodies[i] = t
	}
	return bodies
}
