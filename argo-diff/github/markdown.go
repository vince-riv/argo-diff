package github

import "fmt"

func AppMarkdown(appName, warnStr string) string {
	md := fmt.Sprintf("### %s\n\n", appName)
	if warnStr != "" {
		md += warnStr + "\n\n"
	}
	return md
}

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
