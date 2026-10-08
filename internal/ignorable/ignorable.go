// Package ignorable decides which resource diffs are "ignorable": diffs whose
// every changed line matches a configured regex, such as a Helm chart bump that
// only touches version labels. internal/github renders them folded.
package ignorable

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/config"
)

const (
	EnvRegexes      = "ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE_REGEXES"
	EnvExcludeKinds = "ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE_EXCLUDE_KINDS"

	AnnotationPrefix       = "argo-diff.vince-riv.io"
	AnnotationFlag         = AnnotationPrefix + "/collapse-ignorable"
	AnnotationRegexes      = AnnotationPrefix + "/collapse-ignorable-regexes"
	AnnotationIncludeKinds = AnnotationPrefix + "/collapse-ignorable-include-kinds"

	// emptyListSentinel is the only way to ask for an empty global list: an
	// unset or empty variable means "use the defaults".
	emptyListSentinel = "[]"

	maxListEntries = 32
	maxEntryLen    = 1024
)

// DefaultRegexes are used when EnvRegexes is unset or empty. Keep the README in
// step with this list.
var DefaultRegexes = []string{
	// app version label; the value can be anything (v1.2, a SHA, latest), so match the key only
	`^\s*app\.kubernetes\.io/version:\s`,
	// helm.sh/chart and the legacy chart label; the <name>-<semver> value shape keeps a child
	// Application's spec.source.chart (a real change) out. -v? covers "chart: keel-v1.2.3"
	`^\s*(helm\.sh/)?chart:\s+["']?[A-Za-z0-9][-A-Za-z0-9_.]*-v?\d+\.\d+\.\d+[-+A-Za-z0-9_.]*["']?\s*$`,
	// CRD annotation recording the code generator version
	`^\s*controller-gen\.kubebuilder\.io/version:\s+["']?v?\d+\.\d+\.\d+[-+A-Za-z0-9_.]*["']?\s*$`,
}

// DefaultExcludeKinds are used when EnvExcludeKinds is unset or empty.
var DefaultExcludeKinds = []string{"argoproj.io/*"}

// GroupKind is one parsed group/kind entry, lowercased. Kind "*" means every
// kind in Group.
type GroupKind struct{ Group, Kind string }

func (g GroupKind) matches(group, kind string) bool {
	return g.Group == strings.ToLower(group) && (g.Kind == "*" || g.Kind == strings.ToLower(kind))
}

func anyMatches(list []GroupKind, group, kind string) bool {
	for _, g := range list {
		if g.matches(group, kind) {
			return true
		}
	}
	return false
}

// Global is the operator's configuration, parsed once per event.
type Global struct {
	Active   bool // config.CollapseIgnorableActive()
	Regexes  []*regexp.Regexp
	Exclude  []GroupKind
	Warnings []string // one per invalid or over-limit entry
}

// Policy is the resolved configuration for one application.
type Policy struct {
	Active  bool
	regexes []*regexp.Regexp
	exclude []GroupKind
	include []GroupKind
}

// entries splits raw into trimmed, non-blank, non-comment entries. sep "\n"
// splits only on newlines (regexes contain commas); sep "," also splits each
// line on commas. Entries over the limits are skipped with a warning.
func entries(raw, where string, commaToo bool) (out []string, warnings []string) {
	var all []string
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !commaToo {
			all = append(all, line)
			continue
		}
		for tok := range strings.SplitSeq(line, ",") {
			tok = strings.TrimSpace(tok)
			if tok != "" && !strings.HasPrefix(tok, "#") {
				all = append(all, tok)
			}
		}
	}
	for i, e := range all {
		switch {
		case i >= maxListEntries:
			warnings = append(warnings, fmt.Sprintf("argo-diff skipped entry %q in %s: more than %d entries", truncate(e), where, maxListEntries))
		case len(e) > maxEntryLen:
			warnings = append(warnings, fmt.Sprintf("argo-diff skipped entry %q in %s: longer than %d bytes", truncate(e), where, maxEntryLen))
		default:
			out = append(out, e)
		}
	}
	return out, warnings
}

func truncate(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

// parseRegexes parses a newline-separated regex list. empty reports the
// explicit "[]" sentinel.
func parseRegexes(raw, where string) (res []*regexp.Regexp, warnings []string, empty bool) {
	if strings.TrimSpace(raw) == emptyListSentinel {
		return nil, nil, true
	}
	list, warnings := entries(raw, where, false)
	for _, e := range list {
		re, err := regexp.Compile(e)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("argo-diff skipped invalid regex %q in %s", e, where))
			continue
		}
		res = append(res, re)
	}
	return res, warnings, false
}

// parseGroupKinds parses a comma/newline-separated group/kind list.
func parseGroupKinds(raw, where string) (res []GroupKind, warnings []string, empty bool) {
	if strings.TrimSpace(raw) == emptyListSentinel {
		return nil, nil, true
	}
	list, warnings := entries(raw, where, true)
	for _, e := range list {
		gk, problem := parseGroupKind(e)
		if problem != "" {
			warnings = append(warnings, fmt.Sprintf("argo-diff skipped invalid group/kind %q in %s: %s", e, where, problem))
			continue
		}
		res = append(res, gk)
	}
	return res, warnings, false
}

// parseGroupKind parses one entry: "<group>/<Kind>", "<group>/*", "<Kind>" or
// "/<Kind>" (core group). problem is non-empty for an invalid entry.
func parseGroupKind(e string) (gk GroupKind, problem string) {
	if e == "*" {
		return gk, "a bare * is not supported"
	}
	group, kind, hasSlash := strings.Cut(e, "/")
	if strings.Contains(kind, "/") {
		return gk, "more than one /"
	}
	if !hasSlash {
		if strings.Contains(e, ".") {
			return gk, fmt.Sprintf("did you mean `%s/*`?", e)
		}
		group, kind = "", e
	}
	switch {
	case kind == "":
		return gk, "missing kind"
	case group == "*":
		return gk, "* is not supported as a group"
	case group == "" && kind == "*":
		return gk, "* needs a group"
	}
	return GroupKind{Group: strings.ToLower(group), Kind: strings.ToLower(kind)}, ""
}

func compileDefaults() []*regexp.Regexp {
	res := make([]*regexp.Regexp, 0, len(DefaultRegexes))
	for _, r := range DefaultRegexes {
		res = append(res, regexp.MustCompile(r))
	}
	return res
}

func defaultExclude() []GroupKind {
	res := make([]GroupKind, 0, len(DefaultExcludeKinds))
	for _, e := range DefaultExcludeKinds {
		gk, problem := parseGroupKind(e)
		if problem != "" {
			panic("invalid default exclude kind " + e + ": " + problem)
		}
		res = append(res, gk)
	}
	return res
}

// LoadGlobal reads the operator configuration from the environment on call, so
// tests can t.Setenv it. It parses the lists only when the feature is active.
func LoadGlobal() Global {
	g := Global{Active: config.CollapseIgnorableActive()}
	if !g.Active {
		return g
	}
	if raw := strings.TrimSpace(os.Getenv(EnvRegexes)); raw == "" {
		g.Regexes = compileDefaults()
	} else {
		g.Regexes, g.Warnings, _ = parseRegexes(raw, EnvRegexes)
	}
	if raw := strings.TrimSpace(os.Getenv(EnvExcludeKinds)); raw == "" {
		g.Exclude = defaultExclude()
	} else {
		var w []string
		g.Exclude, w, _ = parseGroupKinds(raw, EnvExcludeKinds)
		g.Warnings = append(g.Warnings, w...)
	}
	for _, w := range g.Warnings {
		log.Warn().Msg(w)
	}
	return g
}

// ForApp resolves the policy for one application from its annotations. The
// warnings are per-application advisories. Annotations cannot turn the feature
// on: with an inactive global, they are ignored.
func (g Global) ForApp(annotations map[string]string) (p Policy, warnings []string) {
	if !g.Active {
		return Policy{}, nil
	}
	switch v := strings.ToLower(strings.TrimSpace(annotations[AnnotationFlag])); v {
	case "false":
		return Policy{}, nil
	case "", "true":
	default:
		warnings = append(warnings, fmt.Sprintf("argo-diff ignored value %q of annotation %s (expected true or false)", v, AnnotationFlag))
	}
	p = Policy{Active: true, exclude: g.Exclude}
	p.regexes = append(p.regexes, g.Regexes...)
	if raw, ok := annotations[AnnotationRegexes]; ok {
		res, w, _ := parseRegexes(raw, "annotation "+AnnotationRegexes)
		p.regexes = append(p.regexes, res...)
		warnings = append(warnings, w...)
	}
	if raw, ok := annotations[AnnotationIncludeKinds]; ok {
		res, w, _ := parseGroupKinds(raw, "annotation "+AnnotationIncludeKinds)
		p.include = res
		warnings = append(warnings, w...)
	}
	for _, w := range warnings {
		log.Warn().Msg(w)
	}
	return p, warnings
}

// Ignorable reports whether a resource diff is ignorable. See the package
// context.md for the decision order.
func (p Policy) Ignorable(group, kind, diffStr string) bool {
	if !p.Active {
		return false
	}
	if anyMatches(p.exclude, group, kind) && !anyMatches(p.include, group, kind) {
		return false
	}
	if len(p.regexes) == 0 {
		return false
	}
	changed := 0
	for line := range strings.SplitSeq(diffStr, "\n") {
		// the same header rule as diffStats() in internal/comment/markdown.go: keep
		// the two in step. The trailing space matters, a removed YAML document
		// separator renders as "----" and is a real change
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			continue
		}
		if !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") {
			continue
		}
		changed++
		if !p.matchesAny(line[1:]) {
			return false
		}
	}
	return changed > 0
}

func (p Policy) matchesAny(s string) bool {
	for _, re := range p.regexes {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// LogConfig logs the effective configuration once at startup.
func LogConfig() {
	g := LoadGlobal()
	if !g.Active {
		log.Debug().Msg("Ignorable diff folding is inactive (needs ARGO_DIFF_COMMENT_COLLAPSE=auto and ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE=true)")
		if os.Getenv(EnvRegexes) != "" || os.Getenv(EnvExcludeKinds) != "" {
			log.Info().Msgf("%s and %s have no effect until ARGO_DIFF_COMMENT_COLLAPSE=auto", EnvRegexes, EnvExcludeKinds)
		}
		return
	}
	res := make([]string, 0, len(g.Regexes))
	for _, re := range g.Regexes {
		res = append(res, re.String())
	}
	ex := make([]string, 0, len(g.Exclude))
	for _, e := range g.Exclude {
		ex = append(ex, e.Group+"/"+e.Kind)
	}
	log.Info().Msgf("Ignorable diff folding is active: %d regexes %q, excluded kinds %q", len(res), res, ex)
}
