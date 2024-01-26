package process_event

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/gendiff"
	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

// Returns first 7 characters of a string (to produce a short commit sha)
func shortSha(str string) string {
	v := []rune(str)
	if len(v) <= 7 {
		return str
	}
	return string(v[:7])
}

// Processes github webhook event data by getting a list of matching argo applications & their manifests and generating diffs
// Sets Github status checks for the relevant commit sha and posts a Github comment it is a pull-request event
// Designed to run within a gorouting to decouple from the webhook response
func ProcessCodeChange(eventInfo webhook.EventInfo, devMode bool, wg *sync.WaitGroup) {
	defer wg.Done()
	// Don't take longer than 3 minutes to execute
	// TODO update internal/argocd to use ctx to gracefully handle timeouts
	// TODO figure out how to call github.Status() with an error status when there's a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	isPr := eventInfo.PrNum > 0

	// set commit status to PENDING
	github.Status(ctx, isPr, github.StatusPending, "", eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)

	// get a list of ArgoCD applications and their manifests whose git URLs match the webhook event
	appResList, err := argocd.GetApplicationChanges(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.RepoDefaultRef, eventInfo.Sha, eventInfo.ChangeRef, eventInfo.BaseRef)
	if err != nil {
		github.Status(ctx, isPr, github.StatusError, err.Error(), eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)
		log.Error().Err(err).Msg("argocd.GetApplicationChanges() failed")
		return // we're done due to a processing error
	}
	log.Trace().Msgf("argocd.GetApplicationChanges() returned %d results", len(appResList))

	errorCount := 0   // keep track of the number of errors
	changeCount := 0  // how many apps have changes
	unknownCount := 0 // how many apps we can't determine if there's changes (usually when we can new manifests but not current ones)
	firstError := ""  // string of the first error we receive - used in commit status message
	markdown := ""    // markdown for pull request comment
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
			markdown += github.AppMarkdownStart(appName, "Error: "+a.WarnStr, appSyncStatus, appHealthStatus, appHealthMsg)
			markdown += github.AppMarkdownEnd()
			_ = cMarkdown.AppMarkdown(appName, "Error: "+a.WarnStr, appSyncStatus, appHealthStatus, appHealthMsg)
			if firstError == "" {
				firstError = a.WarnStr
			}
		} else {
			log.Trace().Msgf("%s has %d Changed Resources", appName, len(a.ChangedResources))
			if len(a.ChangedResources) > 0 {
				changeCount++
				markdown += github.AppMarkdownStart(appName, "", appSyncStatus, appHealthStatus, appHealthMsg)
				appMarkdown := cMarkdown.AppMarkdown(appName, "", appSyncStatus, appHealthStatus, appHealthMsg)
				for _, ar := range a.ChangedResources {
					diffStr := gendiff.UnifiedDiff("live.yaml", fmt.Sprintf("%s.yaml", shortSha(eventInfo.Sha)), ar.YamlCur, ar.YamlNew)
					markdown += github.ResourceDiffMarkdown(ar.ApiVersion, ar.Kind, ar.Name, ar.Namespace, diffStr)
					appMarkdown.AddResourceDiff(ar.ApiVersion, ar.Kind, ar.Name, ar.Namespace, diffStr)
				}
				markdown += github.AppMarkdownEnd()
			}
		}
	}

	newStatus := github.StatusError // commit status is currently pending, newStatus will be the updated status (default to error)
	statusDescription := "Unknown"
	changeCountStr := fmt.Sprintf("%d of %d apps with changes", changeCount, len(appResList))
	if unknownCount > 0 {
		changeCountStr += fmt.Sprintf(" [%d apps unknown]", unknownCount)
	}
	markdownStart := changeCountStr // markdownStart is the pre-amble of the github comment

	if errorCount > 0 {
		// if we had errors, commit status should be a failure
		newStatus = github.StatusFailure
		statusDescription = fmt.Sprintf("%s; %d had an error; first error: %s", changeCountStr, errorCount, firstError)
	} else if firstError != "" {
		// if we had a recoverable error, commit status can be a success (but let's give them the first error)
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s; diff generator failed; first error: %s", changeCountStr, firstError)
	} else {
		// else everything is happy - commit status success
		newStatus = github.StatusSuccess
		statusDescription = fmt.Sprintf("%s - no errors", changeCountStr)
	}
	// send the commit status
	github.Status(ctx, isPr, newStatus, statusDescription, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.Sha, devMode)

	if isPr {
		// if it's a pull-request event, only comment when something has happened
		t := time.Now()
		tStr := t.Format("3:04PM MST, 2 Jan 2006")
		markdownStart += " compared to live state\n"
		markdownStart += "\n" + tStr + "\n"
		cMarkdown.Preamble = markdownStart
		if changeCount == 0 && firstError == "" {
			// if there are no changes or warnings, don't comment (but clear out any existing comments)
			_, _ = github.Comment(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum, eventInfo.Sha, []string{})
		} else {
			_, _ = github.Comment(ctx, eventInfo.RepoOwner, eventInfo.RepoName, eventInfo.PrNum, eventInfo.Sha, cMarkdown.String())
		}
		return
	}
}
