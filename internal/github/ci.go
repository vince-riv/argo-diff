package github

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vince-riv/argo-diff/internal/webhook"
)

// DetectCI reports whether argo-diff is running as a GitHub Action step.
func (Provider) DetectCI() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// EventFromCIEnv builds the event from the variables GitHub Actions sets for a
// pull_request workflow. Refresh is set, so the head SHA and refs are re-read
// from the API rather than trusted from the environment.
func (Provider) EventFromCIEnv() (webhook.EventInfo, error) {
	if ghEvent := os.Getenv("GITHUB_EVENT_NAME"); ghEvent != "pull_request" {
		return webhook.EventInfo{}, fmt.Errorf("unexpected value for GITHUB_EVENT_NAME: %s (expecting pull_request)", ghEvent)
	}
	prRef := os.Getenv("GITHUB_REF")
	prRefParts := strings.SplitN(prRef, "/", 4)
	if len(prRefParts) < 3 {
		return webhook.EventInfo{}, fmt.Errorf("failed extract pull request number from GITHUB_REF %s", prRef)
	}
	prNum, err := strconv.Atoi(prRefParts[2])
	if err != nil {
		return webhook.EventInfo{}, fmt.Errorf("failed extract pull request number from GITHUB_REF %s: %s", prRef, err.Error())
	}
	repoParts := strings.SplitN(os.Getenv("GITHUB_REPOSITORY"), "/", 2)
	if len(repoParts) != 2 {
		return webhook.EventInfo{}, fmt.Errorf("failed to split GITHUB_REPOSITORY %q into owner/repo", os.Getenv("GITHUB_REPOSITORY"))
	}
	return webhook.EventInfo{
		Provider:       providerName,
		RepoOwner:      repoParts[0],
		RepoName:       repoParts[1],
		RepoDefaultRef: os.Getenv("REPO_DEFAULT_REF"),
		PrNum:          prNum,
		ChangeRef:      os.Getenv("GITHUB_HEAD_REF"),
		BaseRef:        os.Getenv("GITHUB_BASE_REF"),
		Refresh:        true, // have argo-diff refresh sha, change-ref, and base-ref
	}, nil
}
