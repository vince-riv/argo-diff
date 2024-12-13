package github

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/gitops-engine/pkg/health"
	"github.com/rs/zerolog/log"
)

const maxCommentLen = 261500 // 262,144 per https://github.com/orgs/community/discussions/27190#discussioncomment-3254953
const maxResourceDiffLen = 260000

var argocdUiUrl string
var commentLineMaxChar int

func init() {
	argocdUiUrl = os.Getenv("ARGOCD_UI_BASE_URL")
	if argocdUiUrl == "" {
		log.Warn().Msg("ARGOCD_UI_BASE_URL is not set - links won't be created im comments")
	} else {
		log.Info().Msgf("ARGOCD_UI_BASE_URL is set to %s for comment links", argocdUiUrl)
	}
	commentLineMaxChar = 175
	lineMaxCharStr := os.Getenv("COMMENT_LINE_MAX_CHARS")
	if lineMaxCharStr != "" {
		v, err := strconv.Atoi(lineMaxCharStr)
		if err == nil {
			commentLineMaxChar = v
		} else {
			log.Warn().Err(err).Msg("Failed to decode COMMENT_LINE_MAX_CHARS")
		}
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

func truncateLines(s string, maxLen int) string {
	var result string
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if len(line) > maxLen {
			result += line[:maxLen] + "...[TRUNCATED]"
		} else {
			result += line
		}
		result += "\n"
	}
	return result
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

type ArgoAppMarkdown struct {
	AppName      string
	WarnStr      string
	SyncStatus   v1alpha1.SyncStatusCode
	HealthStatus health.HealthStatusCode
	HealthMsg    string
	Preamble     string
	Resources    []string
	Closing      string
}

type CommentMarkdown struct {
	Preamble string
	ArgoApps []ArgoAppMarkdown
	Closing  string
}

func (c *CommentMarkdown) AppMarkdown(appName, warnStr string, syncStatus v1alpha1.SyncStatusCode, healthStatus health.HealthStatusCode, healthMsg string) *ArgoAppMarkdown {
	a := ArgoAppMarkdown{
		AppName:      appName,
		WarnStr:      warnStr,
		SyncStatus:   syncStatus,
		HealthStatus: healthStatus,
		HealthMsg:    healthMsg,
	}
	c.ArgoApps = append(c.ArgoApps, a)
	return &c.ArgoApps[len(c.ArgoApps)-1]
}

func (c CommentMarkdown) String() []string {
	var res []string
	md := c.Preamble

	for _, a := range c.ArgoApps {
		newMd := a.OverviewStr(false)
		if len(a.Resources) == 0 {
			if len(md+newMd) <= maxCommentLen {
				md += newMd
			} else {
				res = append(res, md)
				md = newMd
			}
		} else {
			// look ahead to first resource when calculating comment length
			if len(md+newMd+a.Resources[0]) <= maxCommentLen {
				md += newMd
			} else {
				md += "</details>\n\n"
				res = append(res, md)
				md = newMd
			}
			for _, r := range a.Resources {
				if len(md+r) <= maxCommentLen {
					md += r
				} else {
					md += "\n\n[Continued in next comment]\n"
					res = append(res, md)
					md = a.OverviewStr(true)
					md += r
				}
			}
		}
		md += "</details>\n\n"
	}
	res = append(res, md)
	return res
}

func (a *ArgoAppMarkdown) AddResourceDiff(group, kind, name, ns, diffStr string) {
	md := "\n<details open>\n"
	md += fmt.Sprintf("  <summary>===== %s/%s %s/%s =====</summary>\n\n", group, kind, ns, name)
	diffMd := ""
	if diffStr != "" {
		diffMd += "```diff\n"
		diffMd += truncateLines(diffStr, commentLineMaxChar)
		if diffStr[len(diffStr)-1] != '\n' {
			diffMd += "\n"
		}
		diffMd += "```\n\n"
	}
	if len(md+diffMd) > maxResourceDiffLen {
		md += "`<<< DIFF TOO LARGE TO DISPLAY >>>`"
	} else {
		md += diffMd
	}
	md += "</details>\n\n"
	a.Resources = append(a.Resources, md)
}

func (a ArgoAppMarkdown) OverviewStr(continued bool) string {
	md := "\n"
	if !continued {
		md += "---\n"
	}
	md += "<details open>\n"
	if continued {
		md += fmt.Sprintf("<summary>=== %s (cont.) ===</summary>\n\n", capitalizeWords(a.AppName))
	} else {
		md += fmt.Sprintf("<summary>=== %s ===</summary>\n\n", capitalizeWords(a.AppName))
	}
	if argocdUiUrl != "" {
		md += fmt.Sprintf("[ArgoCD UI](%s/applications/argocd/%s)\n", argocdUiUrl, a.AppName)
	}
	md += syncString(a.SyncStatus) + "\n"
	md += healthString(a.HealthStatus, a.HealthMsg) + "\n\n"
	if a.WarnStr != "" {
		md += a.WarnStr + "\n\n"
	}
	return md
}
