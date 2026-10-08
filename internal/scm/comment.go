package scm

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/comment"
)

// outdatedBody replaces an argo-diff comment that a newer run no longer
// needs. Comments are overwritten rather than deleted, so the marker stays and
// a later run can reuse them.
const outdatedBody = "[Outdated argo-diff content]"

// Comment is one comment on a change request, as a provider returns it.
type Comment struct {
	ID     int64
	Body   string
	Author string // login of the comment's author
	URL    string // for log messages only
}

// Commenter is the set of comment primitives a provider implements.
// PostComments runs the provider-neutral algorithm on top of them.
type Commenter interface {
	// GetChangeRequest fetches the change request; PostComments uses it to
	// check that the commit being reported on is still its head.
	GetChangeRequest(ctx context.Context, repo RepoRef, num int) (ChangeRequest, error)
	// ListComments returns every user comment on the change request, oldest
	// first, across all pages. Providers leave out system comments.
	ListComments(ctx context.Context, repo RepoRef, num int) ([]Comment, error)
	CreateComment(ctx context.Context, repo RepoRef, num int, body string) (Comment, error)
	// UpdateComment replaces the body of comment id. num is the change request
	// the comment belongs to (GitLab addresses notes through their MR).
	UpdateComment(ctx context.Context, repo RepoRef, num int, id int64, body string) (Comment, error)
	// CurrentUser is the login comments are posted as. An empty login with a
	// nil error means the identity is not known (eg: under GitHub Actions),
	// and comments are matched by the identifier marker alone.
	CurrentUser(ctx context.Context) (string, error)
}

// isHead reports whether sha is the head of the change request. A lookup
// error assumes it is, so a flaky API doesn't suppress the comment; a missing
// head SHA assumes it is not.
func isHead(ctx context.Context, c Commenter, repo RepoRef, num int, sha string) bool {
	cr, err := c.GetChangeRequest(ctx, repo, num)
	if err != nil {
		log.Warn().Msgf("GetChangeRequest() err'd - assuming %s is HEAD of %s#%d", sha, repo, num)
		return true
	}
	if cr.HeadSHA == "" {
		log.Warn().Msgf("%s#%d has no HEAD SHA - assuming %s is not HEAD", repo, num, sha)
		return false
	}
	return sha == cr.HeadSHA
}

// ExistingComments returns the comments a previous argo-diff run posted on
// the change request, oldest first: those carrying comment.Identifier(), and -
// unless CurrentUser() is empty - written by the current user. Returns an
// empty list if there are none.
func ExistingComments(ctx context.Context, c Commenter, repo RepoRef, num int) ([]Comment, error) {
	login, err := c.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	comments, err := c.ListComments(ctx, repo, num)
	if err != nil {
		log.Error().Err(err).Msgf("Unable to fetch comments for %s#%d", repo, num)
		return nil, err
	}
	log.Debug().Msgf("Checking %d comments in %s#%d", len(comments), repo, num)
	var res []Comment
	for _, cm := range comments {
		if !strings.Contains(cm.Body, comment.Identifier()) {
			continue
		}
		if login == "" || cm.Author == login {
			res = append(res, cm)
		}
	}
	return res, nil
}

// PostComments posts the rendered bodies on the change request, reusing
// argo-diff's existing comments in order: body i edits existing comment i,
// extra bodies are created, and leftover comments are overwritten with
// outdatedBody rather than deleted. An empty bodies list therefore clears out
// every earlier comment. Each body is wrapped with comment.Wrap().
//
// It does nothing when sha is no longer the head of the change request, so a
// slow run can't overwrite the comment of a newer one.
func PostComments(ctx context.Context, c Commenter, repo RepoRef, num int, sha string, bodies []string) ([]Comment, error) {
	var res []Comment
	if !isHead(ctx, c, repo, num, sha) {
		log.Info().Msgf("%s is not HEAD for %s#%d - skipping comment", sha, repo, num)
		return res, nil
	}
	existing, err := ExistingComments(ctx, c, repo, num)
	if err != nil {
		return res, err
	}
	nextExisting := 0
	for i, body := range bodies {
		wrapped := comment.Wrap(body)
		var posted Comment
		if i < len(existing) {
			nextExisting = i + 1
			posted, err = c.UpdateComment(ctx, repo, num, existing[i].ID, wrapped)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to update comment %d for %s#%d", existing[i].ID, repo, num)
				return res, err
			}
		} else {
			posted, err = c.CreateComment(ctx, repo, num, wrapped)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to create comment for %s#%d", repo, num)
				return res, err
			}
		}
		log.Info().Msgf("Created or Updated comment ID %d in %s#%d: %s", posted.ID, repo, num, posted.URL)
		res = append(res, posted)
	}
	for ; nextExisting < len(existing); nextExisting++ {
		id := existing[nextExisting].ID
		posted, err := c.UpdateComment(ctx, repo, num, id, fmt.Sprintf("%s\n\n%s\n", outdatedBody, comment.Identifier()))
		if err != nil {
			log.Error().Err(err).Msgf("Failed to update comment %d for %s#%d", id, repo, num)
			continue
		}
		res = append(res, posted)
	}
	return res, nil
}
