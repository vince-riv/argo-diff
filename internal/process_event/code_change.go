package process_event

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

// How long a single event gets to process before we give up. Configurable via
// ARGO_DIFF_TIMEOUT because the time needed scales with the number of ArgoCD
// applications matching the change: each one costs a round trip to the argocd
// server, so a shared chart in a monorepo can match dozens of apps.
const defaultProcessTimeout = 3 * time.Minute

// processTimeout returns the event processing deadline from ARGO_DIFF_TIMEOUT.
// The value is a Go duration string (eg: "5m", "90s"); a bare integer is
// treated as seconds. Invalid or non-positive values fall back to the default.
func processTimeout() time.Duration {
	envVal := strings.TrimSpace(os.Getenv("ARGO_DIFF_TIMEOUT"))
	if envVal == "" {
		return defaultProcessTimeout
	}
	if d, err := time.ParseDuration(envVal); err == nil && d > 0 {
		return d
	}
	if secs, err := strconv.Atoi(envVal); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	log.Warn().Msgf("Invalid value for ARGO_DIFF_TIMEOUT: %s; must be a positive duration (eg: '5m'); using %s", envVal, defaultProcessTimeout)
	return defaultProcessTimeout
}

// How long the closing GitHub calls (commit status and PR comment) get, so that
// running out of time still produces a partial diff comment naming what was
// missed, rather than no comment at all. Diffing tries to stop this far short of
// the deadline, but since reporting runs on a context of its own, a run can take
// up to this much longer than the timeout.
const defaultReportReserve = 30 * time.Second

// reportReserve returns the portion of timeout to hold back for reporting
// results to GitHub. Budgets too small to absorb the full reserve give up half
// their time, so there's always some left for diffing.
func reportReserve(timeout time.Duration) time.Duration {
	if timeout <= 2*defaultReportReserve {
		return timeout / 2
	}
	return defaultReportReserve
}

// timeoutMarkdown renders the PR comment warning about applications that
// weren't diffed. The list of names is capped so a change matching hundreds of
// applications can't crowd the diffs out of the comment.
func timeoutMarkdown(timeout time.Duration, notDiffed []string) string {
	const maxNames = 20
	names := strings.Join(notDiffed, ", ")
	if len(notDiffed) > maxNames {
		names = fmt.Sprintf("%s and %d more", strings.Join(notDiffed[:maxNames], ", "), len(notDiffed)-maxNames)
	}
	md := "\n> [!WARNING]\n"
	md += fmt.Sprintf("> argo-diff ran out of time, so %d application(s) were **not** diffed: %s\n", len(notDiffed), names)
	md += fmt.Sprintf(">\n> Raise the timeout (currently %s, set via `ARGO_DIFF_TIMEOUT` or the `timeout` input in GitHub Actions) to diff them.\n", timeout)
	return md
}

// Returns first 7 characters of a string (to produce a short commit sha)
/*
func shortSha(str string) string {
	v := []rune(str)
	if len(v) <= 7 {
		return str
	}
	return string(v[:7])
}
*/

// Processes github webhook event data by getting a list of matching argo applications & their manifests and generating diffs
// Sets Github status checks for the relevant commit sha and posts a Github comment it is a pull-request event
// Designed to run within a gorouting to decouple from the webhook response
func ProcessCodeChange(eventInfo webhook.EventInfo, devMode bool, wg *sync.WaitGroup, callerErr *error) {
	defer wg.Done()
	// TODO figure out how to call github.Status() with an error status when there's a timeout
	timeout := processTimeout()
	log.Debug().Msgf("Processing event with a %s timeout", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Validate this is a PR event (required for PR-only support)
	if eventInfo.PrNum <= 0 {
		log.Error().Msg("ProcessCodeChange called with non-PR event - this is not supported")
		*callerErr = fmt.Errorf("only pull request events are supported")
		return
	}

	// Get PR details if this is a refresh event
	if eventInfo.Refresh {
		pull, err := github.GetPullRequest(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum)
		if err != nil {
			log.Error().Err(err).Msgf("github.GetPullRequest(%s, %s, %d) failed", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum)
			*callerErr = err
			return
		}
		base := pull.GetBase()
		head := pull.GetHead()
		if base == nil || head == nil {
			log.Error().Msgf("Empty branch information when refreshing %s/%s#%d", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum)
			*callerErr = fmt.Errorf("empty branch information when refreshing %s/%s#%d", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum)
			return
		}
		eventInfo.Sha = *head.SHA
		eventInfo.ChangeRef = *head.Ref
		eventInfo.BaseRef = *base.Ref
	}

	// Get list of changed files in the PR
	changedFiles, err := github.ListPullRequestFiles(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum)
	if err != nil {
		*callerErr = err
		log.Error().Err(err).Msgf("Failed to list pull request files for %s/%s#%d", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum)
	} else {
		eventInfo.ChangedFiles = changedFiles
	}

	// set commit status to PENDING
	err = github.Status(ctx, github.StatusPending, "", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)
	if err != nil {
		log.Warn().Err(err).Msgf("Failed to set commit status %s for %s/%s@%s", github.StatusPending, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha)
	}

	// get a list of ArgoCD applications and their manifests whose git URLs match the webhook event
	// diffing aims to stop a reserve short of the deadline, so that reporting fits within the
	// budget. The parent context still caps it though: if the GitHub calls above have already
	// eaten the reserve, diffing gets whatever is left of the budget instead. Reporting survives
	// either way, because the context it uses below doesn't derive from this one.
	reserve := reportReserve(timeout)
	diffCtx, diffCancel := context.WithTimeout(ctx, timeout-reserve)
	defer diffCancel()
	appResList, notDiffed, err := argocd.GetApplicationChanges(diffCtx, eventInfo)

	// report on a context of its own: the one above may be at or past its
	// deadline, and a partial comment is far more useful than no comment
	reportCtx, reportCancel := context.WithTimeout(context.Background(), reserve)
	defer reportCancel()

	if err != nil {
		log.Error().Err(err).Msg("argocd.GetApplicationChanges() failed")
		_ = github.Status(reportCtx, github.StatusError, err.Error(), eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)
		*callerErr = err
		return // we're done due to a processing error
	}
	log.Debug().Msgf("argocd.GetApplicationChanges() returned %d results", len(appResList))
	log.Trace().Msgf("argocd.GetApplicationChanges() returned: %+v", appResList)

	errorCount := 0   // keep track of the number of errors
	changeCount := 0  // how many apps have changes
	unknownCount := 0 // how many apps we can't determine if there's changes (usually when we can new manifests but not current ones)
	firstError := ""  // string of the first error we receive - used in commit status message
	cMarkdown := github.CommentMarkdown{}
	for _, a := range appResList {
		appName := a.ArgoApp.ObjectMeta.Name
		appSyncStatus := a.ArgoApp.Status.Sync.Status
		appHealthStatus := a.ArgoApp.Status.Health.Status
		appHealthMsg := a.ArgoApp.Status.Health.Message
		// appNs := a.ArgoApp.ObjectMeta.Namespance
		if a.WarnStr != "" {
			log.Trace().Msgf("%s has WarnStr %s", appName, a.WarnStr)
			errorCount++
			_ = cMarkdown.AppMarkdown(appName, "Error: "+a.WarnStr, appSyncStatus, appHealthStatus, appHealthMsg)
			if firstError == "" {
				firstError = a.WarnStr
			}
		} else {
			log.Trace().Msgf("%s has %d Changed Resources", appName, len(a.ChangedResources))
			if len(a.ChangedResources) > 0 {
				changeCount++
				appMarkdown := cMarkdown.AppMarkdown(appName, "", appSyncStatus, appHealthStatus, appHealthMsg)
				for _, ar := range a.ChangedResources {
					appMarkdown.AddResourceDiff(ar.Group, ar.Kind, ar.Name, ar.Namespace, ar.DiffStr)
				}
			}
		}
	}

	// commit status is currently pending, newStatus will be the updated status (default to error)
	newStatus := github.StatusError //nolint:ineffassign
	statusDescription := "Unknown"  //nolint:ineffassign
	changeCountStr := fmt.Sprintf("%d of %d apps with changes", changeCount, len(appResList))
	if unknownCount > 0 {
		changeCountStr += fmt.Sprintf(" [%d apps unknown]", unknownCount)
	}
	if len(notDiffed) > 0 {
		changeCountStr += fmt.Sprintf(" [%d apps not diffed]", len(notDiffed))
	}
	markdownStart := changeCountStr // markdownStart is the pre-amble of the github comment

	if errorCount > 0 {
		// if we had errors, commit status should be a failure
		newStatus = github.StatusFailure
		statusDescription = fmt.Sprintf("%s; %d had an error; first error: %s", changeCountStr, errorCount, firstError)
		*callerErr = fmt.Errorf("%d application(s) failed to generate a diff; first error: %s", errorCount, firstError)
	} else if firstError != "" {
		// if we had a recoverable error, commit status can be a success (but let's give them the first error)
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s; diff generator failed; first error: %s", changeCountStr, firstError)
	} else {
		// else everything is happy - commit status success
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s - no errors", changeCountStr)
	}
	if len(notDiffed) > 0 {
		// results are incomplete - fail rather than report success on a partial diff
		newStatus = github.StatusFailure
		statusDescription = fmt.Sprintf("%d app(s) not diffed (timed out); %s", len(notDiffed), statusDescription)
		if *callerErr == nil {
			*callerErr = fmt.Errorf("timed out (ARGO_DIFF_TIMEOUT is %s); %d application(s) were not diffed", timeout, len(notDiffed))
		}
	}
	// send the commit status
	_ = github.Status(reportCtx, newStatus, statusDescription, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)

	// Post PR comment when something has happened
	t := time.Now()
	tStr := t.Format("3:04PM MST, 2 Jan 2006")
	markdownStart += " compared to live state\n"
	markdownStart += "\n" + tStr + "\n"
	if len(notDiffed) > 0 {
		markdownStart += timeoutMarkdown(timeout, notDiffed)
	}
	cMarkdown.Preamble = markdownStart
	if changeCount == 0 && firstError == "" && len(notDiffed) == 0 {
		// if there are no changes or warnings, don't comment (but clear out any existing comments)
		_, _ = github.Comment(reportCtx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum, eventInfo.Sha, []string{})
	} else {
		_, _ = github.Comment(reportCtx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum, eventInfo.Sha, cMarkdown.String())
	}
}
