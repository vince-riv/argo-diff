package github

func AppendDiffComment(appName, diffStr, warnStr string) string {
	md := ""
	if warnStr != "" {
		md += "<details open>\n"
	} else {
		md += "<details>\n"
	}
	// TODO link to argo app?
	md += "  <summary>" + appName + "</summary>\n\n"
	if warnStr != "" {
		md += warnStr + "\n\n"
	}
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
