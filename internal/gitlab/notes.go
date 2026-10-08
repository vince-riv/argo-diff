package gitlab

import (
	"context"
	"errors"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/vince-riv/argo-diff/internal/comment"
	"github.com/vince-riv/argo-diff/internal/scm"
)

// notesPerPage is the page size for listing an MR's notes. 100 is the API's
// maximum.
const notesPerPage = 100

func (Provider) Dialect() comment.Dialect { return comment.GitLab }

// CurrentUser returns the username notes are posted as. With the gitlab
// connectivity check bypassed it returns "", so notes are matched by marker
// alone, as the GitHub provider does with its bypass.
func (Provider) CurrentUser(ctx context.Context) (string, error) {
	if bypassGitlabCheck() {
		return "", nil
	}
	return getCurrentUser(ctx)
}

// ListComments returns every user note on merge request num, oldest first,
// across all pages. System notes ("added 1 commit", ...) are left out.
func (Provider) ListComments(ctx context.Context, repo scm.RepoRef, num int) ([]scm.Comment, error) {
	if client == nil {
		return nil, errors.New("no gitlab client")
	}
	opts := &gitlab.ListMergeRequestNotesOptions{
		ListOptions: gitlab.ListOptions{PerPage: notesPerPage},
		OrderBy:     new("created_at"),
		Sort:        new("asc"),
	}
	var res []scm.Comment
	for {
		notes, resp, err := client.Notes.ListMergeRequestNotes(repo.FullPath(), int64(num), opts, gitlab.WithContext(ctx))
		logResponse(resp, "Notes.ListMergeRequestNotes()")
		if err != nil {
			logAPIError(err, "Unable to list the notes of merge request %s!%d", repo, num)
			return nil, err
		}
		for _, n := range notes {
			if n == nil || n.System {
				continue
			}
			res = append(res, toComment(n, repo, num))
		}
		if resp.NextPage == 0 {
			return res, nil
		}
		opts.Page = resp.NextPage
	}
}

func (Provider) CreateComment(ctx context.Context, repo scm.RepoRef, num int, body string) (scm.Comment, error) {
	if client == nil {
		return scm.Comment{}, errors.New("no gitlab client")
	}
	n, resp, err := client.Notes.CreateMergeRequestNote(repo.FullPath(), int64(num), &gitlab.CreateMergeRequestNoteOptions{Body: &body}, gitlab.WithContext(ctx))
	logResponse(resp, "Notes.CreateMergeRequestNote()")
	return postedNote(n, err, repo, num)
}

// UpdateComment replaces the body of note id on merge request num. GitLab
// addresses notes through their MR.
func (Provider) UpdateComment(ctx context.Context, repo scm.RepoRef, num int, id int64, body string) (scm.Comment, error) {
	if client == nil {
		return scm.Comment{}, errors.New("no gitlab client")
	}
	n, resp, err := client.Notes.UpdateMergeRequestNote(repo.FullPath(), int64(num), id, &gitlab.UpdateMergeRequestNoteOptions{Body: &body}, gitlab.WithContext(ctx))
	logResponse(resp, "Notes.UpdateMergeRequestNote()")
	return postedNote(n, err, repo, num)
}

// postedNote converts the note a create/update call returned.
func postedNote(n *gitlab.Note, err error, repo scm.RepoRef, num int) (scm.Comment, error) {
	if err != nil {
		logAPIError(err, "Unable to post a note on merge request %s!%d", repo, num)
		return scm.Comment{}, err
	}
	if n == nil {
		return scm.Comment{}, errors.New("gitlab returned no note")
	}
	return toComment(n, repo, num), nil
}

// toComment converts a note. The API gives notes no URL, so the link to the
// note in the MR is built from the instance's web URL; it is for logs only.
func toComment(n *gitlab.Note, repo scm.RepoRef, num int) scm.Comment {
	return scm.Comment{
		ID:     n.ID,
		Body:   n.Body,
		Author: n.Author.Username,
		URL:    fmt.Sprintf("%s/%s/-/merge_requests/%d#note_%d", webBaseURL, repo.FullPath(), num, n.ID),
	}
}
