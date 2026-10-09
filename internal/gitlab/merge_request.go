package gitlab

import (
	"context"
	"errors"

	"github.com/rs/zerolog/log"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/vince-riv/argo-diff/internal/scm"
)

// diffsPerPage is the page size for listing an MR's changed files. 100 is
// the API's maximum.
const diffsPerPage = 100

// GetChangeRequest fetches merge request num (its IID) of repo. A field the
// API did not return is left empty; see scm.ChangeRequest.
func (Provider) GetChangeRequest(ctx context.Context, repo scm.RepoRef, num int) (scm.ChangeRequest, error) {
	if client == nil {
		return scm.ChangeRequest{}, errors.New("no gitlab client")
	}
	mr, resp, err := client.MergeRequests.GetMergeRequest(repo.FullPath(), int64(num), nil, gitlab.WithContext(ctx))
	logResponse(resp, "MergeRequests.GetMergeRequest()")
	if err != nil {
		logAPIError(err, "Unable to fetch merge request %s!%d", repo, num)
		return scm.ChangeRequest{}, err
	}
	return scm.ChangeRequest{
		Number:  num,
		HeadSHA: mr.SHA,
		HeadRef: mr.SourceBranch,
		BaseRef: mr.TargetBranch,
	}, nil
}

// ListChangedFiles returns every path merge request num touches, across all
// pages of its diffs. A renamed file contributes both its old and its new
// path, so a manifest-generate-paths filter sees either.
func (Provider) ListChangedFiles(ctx context.Context, repo scm.RepoRef, num int) ([]string, error) {
	if client == nil {
		return nil, errors.New("no gitlab client")
	}
	opts := &gitlab.ListMergeRequestDiffsOptions{ListOptions: gitlab.ListOptions{PerPage: diffsPerPage}}
	var files []string
	seen := map[string]bool{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	for {
		diffs, resp, err := client.MergeRequests.ListMergeRequestDiffs(repo.FullPath(), int64(num), opts, gitlab.WithContext(ctx))
		logResponse(resp, "MergeRequests.ListMergeRequestDiffs()")
		if err != nil {
			logAPIError(err, "Unable to list the diffs of merge request %s!%d", repo, num)
			return nil, err
		}
		for _, d := range diffs {
			if d == nil {
				continue
			}
			add(d.OldPath)
			add(d.NewPath)
		}
		if resp.NextPage == 0 {
			log.Debug().Msgf("%s!%d changes %d files", repo, num, len(files))
			return files, nil
		}
		opts.Page = resp.NextPage
	}
}
