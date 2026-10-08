// Package scm holds the source-control-provider abstraction: the neutral types
// every provider (GitHub today) speaks in, so the rest of argo-diff does not
// depend on any one provider's API types.
package scm

// RepoRef identifies a repository on a provider.
//
// Owner is everything before the repository's own name, and may contain "/":
// GitLab projects sit in nested groups (eg: "group/subgroup"). Host is the
// provider's hostname (eg: "github.com"); it may be empty, in which case the
// provider addresses the repository on its configured API host.
type RepoRef struct {
	Host  string
	Owner string
	Name  string
}

// FullPath is the repository's path on its host, eg: "vince-riv/argo-diff".
func (r RepoRef) FullPath() string {
	return r.Owner + "/" + r.Name
}

// String is FullPath, prefixed with Host when one is set. For log messages.
func (r RepoRef) String() string {
	if r.Host == "" {
		return r.FullPath()
	}
	return r.Host + "/" + r.FullPath()
}

// ChangeRequest is the provider-neutral view of a pull request (GitHub) or
// merge request (GitLab): just the fields argo-diff needs to diff it.
//
// A field the provider did not return is left empty rather than erroring, so
// each caller can decide what a missing field means to it.
type ChangeRequest struct {
	Number  int
	HeadSHA string // commit at the tip of the change
	HeadRef string // source branch
	BaseRef string // target branch
}

// Status is a commit status state. Providers map these onto their own states
// (eg: GitLab has no "failure" and calls it "failed").
type Status string

const (
	StatusPending Status = "pending"
	StatusSuccess Status = "success"
	StatusFailure Status = "failure"
	StatusError   Status = "error"
)

// Valid reports whether s is one of the four states above.
func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusSuccess, StatusFailure, StatusError:
		return true
	}
	return false
}
