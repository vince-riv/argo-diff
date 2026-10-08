package comment

// githubCommentHardMax is GitHub's own cap on an issue comment body, per
// https://github.com/orgs/community/discussions/27190#discussioncomment-3254953
// Nothing this package emits may exceed it: a body over the cap is a 422 from
// the API, which means no comment at all.
const githubCommentHardMax = 262144

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

// GitHub is the github.com dialect.
var GitHub = Dialect{Name: "github", HardMax: githubCommentHardMax}

// Dialects lists every known dialect. Tests render through each of them.
var Dialects = []Dialect{GitHub}
