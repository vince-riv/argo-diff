package github

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	ghinstallation "github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v83/github"
	"github.com/rs/zerolog/log"
)

var (
	statusClient     *github.Client
	statusContextStr = "argo-diff"
	skipCommitStatus = false
)

const statusDescriptionMaxLen = 140

const StatusPending = "pending"
const StatusSuccess = "success"
const StatusFailure = "failure"
const StatusError = "error"

func init() {
	contextStr := strings.TrimSpace(os.Getenv("ARGO_DIFF_CONTEXT_STR"))
	if contextStr != "" {
		statusContextStr = "argo-diff/" + contextStr
	}
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		log.Info().Msg("GITHUB_ACTIONS env var is 'true' - will skip setting commit statuses")
		skipCommitStatus = true
	}
	// Create Github API client
	if githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"); githubPAT != "" {
		statusClient = github.NewClient(nil).WithAuthToken(githubPAT)
		return
	}
	if githubToken := os.Getenv("GITHUB_TOKEN"); githubToken != "" {
		statusClient = github.NewClient(nil).WithAuthToken(githubToken)
		return
	}
	tr := http.DefaultTransport
	appId, err := strconv.ParseInt(os.Getenv("GITHUB_APP_ID"), 10, 64)
	if err != nil {
		log.Error().Err(err).Msgf("Unable to parse %s", os.Getenv("GITHUB_APP_ID"))
		return
	}
	installId, err := strconv.ParseInt(os.Getenv("GITHUB_APP_INSTALLATION_ID"), 10, 64)
	if err != nil {
		log.Error().Err(err).Msgf("Unable to parse %s", os.Getenv("GITHUB_APP_INSTALLATION_ID"))
		return
	}
	privKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	itr, err := ghinstallation.New(tr, appId, installId, []byte(privKey))
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create github client: appId %d, installId %d, privKey %s...", appId, installId, privKey[:15])
		return
	}
	statusClient = github.NewClient(&http.Client{Transport: itr})
}

// Helper that sets commit status for the request commit sha
func Status(ctx context.Context, status, description, repoOwner, repoName, commitSha string, dryRun bool) error {
	if skipCommitStatus {
		log.Debug().Msg("Skipping commit status")
		return nil
	}
	if status != StatusPending && status != StatusSuccess && status != StatusFailure && status != StatusError {
		log.Fatal().Msgf("Cannot create github status with status string '%s'", status)
		return fmt.Errorf("unknown status string '%s'", status)
	}
	contextStr := statusContextStr
	if len(description) > statusDescriptionMaxLen {
		description = description[:137] + "..."
	}
	// TODO add support for AvatarURL ?
	// TODO add support for TargetURL ?
	repoStatus := github.RepoStatus{
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
