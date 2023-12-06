package github

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/rs/zerolog/log"
)

var argocdUiUrl string

func init() {
	argocdUiUrl = os.Getenv("ARGOCD_UI_BASE_URL")
	if argocdUiUrl == "" {
		log.Warn().Msg("ARGOCD_UI_BASE_URL is not set - links won't be created im comments")
	} else {
		log.Info().Msgf("ARGOCD_UI_BASE_URL is set to %s for comment links", argocdUiUrl)
	}
}

func capitalizeWords(s string) string {
	inWord := false
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' || r == '.' {
			inWord = false
			return r
		}
		if !inWord {
			inWord = true
			return unicode.ToTitle(r)
		}
		return r
	}, s)
}

func syncString(s v1alpha1.SyncStatusCode) string {
	switch s {
	case v1alpha1.SyncStatusCodeUnknown:
		// status of a sync could not be reliably determined
		return string(s) + " :question:"
	case v1alpha1.SyncStatusCodeSynced:
		// that desired and live states match
		return string(s) + " :white_check_mark:"
	case v1alpha1.SyncStatusCodeOutOfSync:
		// there is a drift between desired and live states
		return string(s) + " :warning:"
	default:
		return string(s) + " :interrobang:"
	}
}

func healthString(s health.HealthStatusCode, msg string) string {
	emoji := ":interrobang:" // default to !?
	switch s {
	case health.HealthStatusUnknown:
		// "Unknown": Indicates that health assessment failed and actual health status is unknown
		emoji = " :question:"
	case health.HealthStatusProgressing:
		// "Progressing": Progressing health status means that resource is not healthy but still have a chance to reach healthy state
		emoji = " :hourglass_flowing_sand:"
	case health.HealthStatusHealthy:
		// "Healthy": Resource is 100% healthy
		emoji = " :green_heart:"
	case health.HealthStatusSuspended:
		// "Suspended": Assigned to resources that are suspended or paused. The typical example is a [suspended](https://kubernetes.io/docs/tasks/job/automated-tasks-with-cron-jobs/#suspend) CronJob.
		emoji = " :no_entry_sign:"
	case health.HealthStatusDegraded:
		// "Degraded": Degraded status is used if resource status indicates failure or resource could not reach healthy state within some timeout.
		emoji = " :x:"
	case health.HealthStatusMissing:
		// Indicates that resource is missing in the cluster.
		emoji = " :ghost:"
	}
	if msg == "" {
		return string(s) + emoji
	} else {
		return string(s) + emoji + " - " + msg
	}
}

// Helper to generate markdown for pre-amble of argo-diff's PR comment
func AppMarkdownStart(appName, warnStr string, syncStatus v1alpha1.SyncStatusCode, healthStatus health.HealthStatusCode, healthMsg string) string {
	md := "\n---\n"
	md += "<details open>\n"
	md += fmt.Sprintf("<summary>=== %s ===</summary>\n\n", capitalizeWords(appName))
	if argocdUiUrl != "" {
		md += fmt.Sprintf("[ArgoCD UI](%s/applications/argocd/%s)\n", argocdUiUrl, appName)
	}
	md += syncString(syncStatus) + "\n"
	md += healthString(healthStatus, healthMsg) + "\n\n"
	if warnStr != "" {
		md += warnStr + "\n\n"
	}
	return md
}

// Helper to generate markdown a diff in argo-diff's PR comment
func ResourceDiffMarkdown(apiVersion, kind, name, ns, diffStr string) string {
	md := "<details open>\n"
	md += fmt.Sprintf("  <summary>%s/%s %s/%s</summary>\n\n", apiVersion, kind, ns, name)
	if diffStr != "" {
		md += "```diff\n"
		md += diffStr
		if diffStr[len(diffStr)-1] != '\n' {
			md += "\n"
		}
		md += "```\n\n"
	}
	md += "</details>\n\n"
	return md
}

// Helper to generate markdown for the end of argo-diff's PR comment
func AppMarkdownEnd() string {
	return "</details>\n\n"
}
