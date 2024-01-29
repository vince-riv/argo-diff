package github

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v58/github"
	"github.com/rs/zerolog/log"
)

var (
	statusClient     *github.Client
	statusContextStr = "argo-diff"
)

const statusDescriptionMaxLen = 140

const StatusPending = "pending"
const StatusSuccess = "success"
const StatusFailure = "failure"
const StatusError = "error"

func init() {
	statusContextEnv := os.Getenv("GITHUB_STATUS_CONTEXT_STR")
	if statusContextEnv != "" {
		statusContextStr = statusContextEnv
	}
	// Create Github API client
	if githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"); githubPAT != "" {
		statusClient = github.NewClient(nil).WithAuthToken(githubPAT)
		return
	}
	tr := http.DefaultTransport
	appId, err := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
	if err != nil {
		log.Error().Err(err).Msgf("Unable to parse %s", os.Getenv("GITHUB_APP_ID"))
		return
	}
	installId, err := strconv.ParseInt(os.Getenv("GITHUB_INSTALLATION_ID"), 10, 64)
	if err != nil {
		log.Error().Err(err).Msgf("Unable to parse %s", os.Getenv("GITHUB_INSTALLATION_ID"))
		return
	}
	privKeyFile := os.Getenv("GITHUB_PRIVATE_KEY_FILE")
	itr, err := ghinstallation.NewKeyFromFile(tr, appId, installId, privKeyFile)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create github client: appId %d, installId %d, privKeyFile %s", appId, installId, privKeyFile)
		return
	}
	statusClient = github.NewClient(&http.Client{Transport: itr})
}

// Helper that sets commit status for the request commit sha
func Status(ctx context.Context, isPr bool, status, description, repoOwner, repoName, commitSha string, dryRun bool) error {
	if status != StatusPending && status != StatusSuccess && status != StatusFailure && status != StatusError {
		log.Fatal().Msgf("Cannot create github status with status string '%s'", status)
		return fmt.Errorf("unknown status string '%s'", status)
	}
	contextStr := statusContextStr + " (push)"
	if isPr {
		contextStr = statusContextStr + " (pull_request)"
	}
	if len(description) > statusDescriptionMaxLen {
		description = description[:137] + "..."
	}
	// TODO add support for AvatarURL ?
	// TODO add support for TargetURL ?
	repoStatus := &github.RepoStatus{
		State:       &status,
		Description: &description,
		Context:     github.String(contextStr),
	}

	if dryRun {
		log.Info().Msgf("DRY RUN: statusClient.Repositories.CreateStatus(_, %s, %s, %s, %v)", repoOwner, repoName, commitSha, repoStatus)
		return nil
	}
	if statusClient == nil {
		log.Error().Msg("Cannot call github API - I don't have a client set")
		return fmt.Errorf("no github status client")
	}
	_, resp, err := statusClient.Repositories.CreateStatus(ctx, repoOwner, repoName, commitSha, repoStatus)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create repo status %s/%s@%s: %s %s '%s'", repoOwner, repoName, commitSha, contextStr, status, description)
		return err
	}
	log.Info().Msgf("%s - repo status %s/%s@%s: %s %s '%s'", resp.Status, repoOwner, repoName, commitSha, contextStr, status, description)
	return nil
}
