package comment

// githubCommentHardMax is GitHub's own cap on an issue comment body, per
// https://github.com/orgs/community/discussions/27190#discussioncomment-3254953
// Nothing this package emits may exceed it: a body over the cap is a 422 from
// the API, which means no comment at all.
const githubCommentHardMax = 262144

// gitlabNoteHardMax is GitLab's cap on a note body: 1 MiB, counted in bytes
// (verified against gitlab.com in issue #160, Phase 0: 1,048,577 bytes is a
// 400). len() counts bytes, so multi-byte text is sized correctly.
const gitlabNoteHardMax = 1048576

// Dialect is one provider's comment rendering rules.
//
// Every supported provider renders the GitHub alert syntax (> [!NOTE]) and
// only at the top level of a document, so String() always hoists alerts out of
// <details>. Add a field here for a provider that differs, rather than
// branching on Name.
type Dialect struct {
	// Name is the provider name, eg: "github". For logs.
	Name string
	// HardMax is the largest comment body the provider's API accepts, in
	// bytes. ARGO_DIFF_COMMENT_MAX_CHARS can lower it but never raise it.
	HardMax int
}

// orDefault is d, or GitHub for a dialect with no HardMax - the zero value
// included, since GitHub is the default provider. Everything that reads a
// dialect's rules goes through it, so no caller can size a body against 0.
func (d Dialect) orDefault() Dialect {
	if d.HardMax <= 0 {
		return GitHub
	}
	return d
}

// GitHub is the github.com dialect.
var GitHub = Dialect{Name: "github", HardMax: githubCommentHardMax}

// GitLab is the GitLab dialect (gitlab.com and self-managed 17.10+, the first
// release with alerts). Alerts nested in <details> render on gitlab.com but
// were not checked on 17.10, so they are hoisted as for GitHub.
var GitLab = Dialect{Name: "gitlab", HardMax: gitlabNoteHardMax}

// Dialects lists every known dialect. Tests render through each of them.
var Dialects = []Dialect{GitHub, GitLab}
