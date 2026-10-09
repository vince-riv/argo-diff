package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/rs/zerolog/log"
	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/vince-riv/argo-diff/internal/scm"
)

// statusDescriptionMaxLen is GitLab's limit on a commit status description,
// in characters (a longer one is a 400).
const statusDescriptionMaxLen = 255

// statusName is the commit status name: argo-diff, or
// argo-diff/<ARGO_DIFF_CONTEXT_STR>. GitLab updates a status of this name on
// the SHA in place only while it is pending or running; a finished one gets a
// new record.
var statusName = "argo-diff"

func init() {
	if contextStr := strings.TrimSpace(os.Getenv("ARGO_DIFF_CONTEXT_STR")); contextStr != "" {
		statusName = "argo-diff/" + contextStr
	}
}

// statusState maps a neutral state onto GitLab's. GitLab has no "failure" or
// "error" (it rejects "failure" with a 400); both become "failed".
func statusState(s scm.Status) (gitlab.BuildStateValue, error) {
	switch s {
	case scm.StatusPending:
		return gitlab.Pending, nil
	case scm.StatusSuccess:
		return gitlab.Success, nil
	case scm.StatusFailure, scm.StatusError:
		return gitlab.Failed, nil
	}
	return "", fmt.Errorf("unknown status string '%s'", s)
}

// alreadyPending reports whether err is GitLab refusing to set a pending
// status on a SHA whose status of the same name is already pending (a 400:
// "Cannot transition status via :enqueue from :pending"). That happens when a
// run died before its final status, or two runs overlap on one SHA; the
// status the caller wanted is already there.
func alreadyPending(err error) bool {
	var er *gitlab.ErrorResponse
	return errors.As(err, &er) && er.HasStatusCode(http.StatusBadRequest) &&
		strings.Contains(er.Message, "Cannot transition status via :enqueue from :pending")
}

// truncateDescription cuts description to GitLab's limit, counted in
// characters, ending it with "..." when it was cut.
func truncateDescription(description string) string {
	if utf8.RuneCountInString(description) <= statusDescriptionMaxLen {
		return description
	}
	r := []rune(description)
	return string(r[:statusDescriptionMaxLen-3]) + "..."
}

// SetStatus sets the argo-diff commit status on sha. dryRun (dev mode) logs
// instead of calling the API.
func (Provider) SetStatus(ctx context.Context, repo scm.RepoRef, sha string, state scm.Status, description string, dryRun bool) error {
	glState, err := statusState(state)
	if err != nil {
		log.Error().Err(err).Msg("Cannot create gitlab commit status")
		return err
	}
	description = truncateDescription(description)
	opts := &gitlab.SetCommitStatusOptions{
		State:       glState,
		Name:        &statusName,
		Description: &description,
	}
	if dryRun {
		log.Info().Msgf("DRY RUN: Commits.SetCommitStatus(%s, %s, %s %s '%s')", repo, sha, statusName, glState, description)
		return nil
	}
	if client == nil {
		return errors.New("no gitlab client")
	}
	// No Ref or PipelineID is sent, so GitLab attaches the status to the first
	// branch of the project containing sha; see context.md for what that means
	// for fork MRs.
	_, resp, err := client.Commits.SetCommitStatus(repo.FullPath(), sha, opts, gitlab.WithContext(ctx))
	if err != nil && glState == gitlab.Pending && alreadyPending(err) {
		log.Info().Msgf("Commit status %s@%s: %s is already pending", repo, sha, statusName)
		return nil
	}
	if err != nil {
		logAPIError(err, "Failed to set commit status %s@%s: %s %s '%s'", repo, sha, statusName, glState, description)
		return err
	}
	log.Info().Msgf("%s - commit status %s@%s: %s %s '%s'", resp.Status, repo, sha, statusName, glState, description)
	return nil
}
