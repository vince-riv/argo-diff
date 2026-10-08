package github

import (
	"context"

	"github.com/google/go-github/v92/github"

	"github.com/vince-riv/argo-diff/internal/scm"
)

// Comment() and getExistingComments() used to live in this package. The
// algorithm now runs in internal/scm on top of Provider's primitives; these
// adapters keep comment_test.go's calls and assertions as they were, so the
// tests still pin the behavior the move had to preserve.

func toIssueComments(cs []scm.Comment) []*github.IssueComment {
	var res []*github.IssueComment
	for _, c := range cs {
		res = append(res, &github.IssueComment{ID: new(c.ID), Body: new(c.Body)})
	}
	return res
}

func getExistingComments(ctx context.Context, owner, repo string, prNum int) ([]*github.IssueComment, error) {
	cs, err := scm.ExistingComments(ctx, Provider{}, scm.RepoRef{Owner: owner, Name: repo}, prNum)
	return toIssueComments(cs), err
}

func Comment(ctx context.Context, owner, repo string, prNum int, sha string, commentBodies []string) ([]*github.IssueComment, error) {
	cs, err := scm.PostComments(ctx, Provider{}, scm.RepoRef{Owner: owner, Name: repo}, prNum, sha, commentBodies)
	return toIssueComments(cs), err
}
