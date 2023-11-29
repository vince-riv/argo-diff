package github

import (
	"fmt"
	"os"

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

// Helper to generate markdown for pre-amble of argo-diff's PR comment
func AppMarkdownStart(appName, warnStr string) string {
	md := "\n---\n"
	md += "<details open>\n"
	md += fmt.Sprintf("<summary>%s</summary>\n\n", appName)
	if argocdUiUrl != "" {
		md += fmt.Sprintf("[ArgoCD UI](%s/applications/argocd/%s)", argocdUiUrl, appName)
	}
	if warnStr != "" {
		md += warnStr + "\n\n"
	}
	return md
}

// Helper to generate markdown a diff in argo-diff's PR comment
func ResourceDiffMarkdown(apiVersion, kind, name, ns, diffStr string) string {
	md := "<details open>\n"
	// TODO link to argo app?
	md += fmt.Sprintf("  <summary>%s %s %s.%s</summary>\n\n", apiVersion, kind, ns, name)
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
