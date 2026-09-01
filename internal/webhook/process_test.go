package webhook

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	// Ensure to get the latest version
)

const testDataDir = "webhook_testdata"

const payloadPrClose = "payload-pr-close.json"
const payloadPrOpen = "payload-pr-open.json"
const payloadPrSync = "payload-pr-sync.json"
const payloadPrEditedBase = "payload-pr-edited-base.json"
const payloadPrEditedTitle = "payload-pr-edited-title.json"
const payloadCommentCreated = "payload-comment-created.json"
const payloadCommentCreatedArgoDiff = "payload-comment-argodiff-created.json"

func readFileToByteArray(fileName string) ([]byte, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("Error getting current working directory: %w", err)
	}

	filePath := filepath.Join(workingDir, testDataDir, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, filePath, fmt.Errorf("error reading file '%s': %w", filePath, err)
	}

	return data, filePath, nil
}

func TestLoadPullRequestEvents(t *testing.T) {
	var result EventInfo
	payloadFiles := []string{payloadPrClose, payloadPrOpen, payloadPrSync, payloadPrEditedBase, payloadPrEditedTitle}
	for _, payloadFile := range payloadFiles {
		payload, filePath, err := readFileToByteArray(payloadFile)
		if err != nil {
			t.Errorf("Failed to read %s: %v", payloadFile, err)
		}
		result, err = ProcessPullRequest(payload)
		if err != nil {
			t.Errorf("Failed to load payload from %s: %v", filePath, err)
		}
		if payloadFile == payloadPrClose || payloadFile == payloadPrEditedTitle {
			if !result.Ignore {
				t.Errorf("ProcessPullRequest() Expected to ignore this event. Payload %s", filePath)
			}
		} else {
			if result.Ignore {
				t.Errorf("ProcessPullRequest() Expected to NOT ignroe this event. Payload %s", filePath)
			}
			if result.RepoOwner == "" || result.RepoName == "" || result.RepoDefaultRef == "" || result.Sha == "" || result.PrNum < 1 || result.ChangeRef == "" || result.BaseRef == "" {
				t.Errorf("ProcessPullRequest() Result has at least one empty value: %+v; Payload %s", result, filePath)
			}
			if result.RepoDefaultRef == result.ChangeRef {
				t.Errorf("ProcessPullRequest() ChangeRef is the same as DefaultRef")
			}
			if result.Refresh {
				t.Errorf("ProcessPullRequest() Expected to NOT set refresh flag. Payload %s", filePath)
			}
		}
	}
}

// TestPullRequestBaseRetarget covers the case where GitHub retargets a stacked PR onto a new
// base branch (an "edited" action with changes.base.ref.from set) after the PR's original base
// branch merges. This must be treated as actionable, distinct from ordinary title/body edits.
func TestPullRequestBaseRetarget(t *testing.T) {
	payload, filePath, err := readFileToByteArray(payloadPrEditedBase)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", payloadPrEditedBase, err)
	}
	result, err := ProcessPullRequest(payload)
	if err != nil {
		t.Fatalf("Failed to load payload from %s: %v", filePath, err)
	}
	if result.Ignore {
		t.Errorf("ProcessPullRequest() Expected to NOT ignore a base-retarget edited event. Payload %s", filePath)
	}
	if result.BaseRef == "" {
		t.Errorf("ProcessPullRequest() Expected BaseRef to be set for a base-retarget event. Payload %s", filePath)
	}
	if result.BaseRef == "feature/parent-branch" {
		t.Errorf("ProcessPullRequest() Expected BaseRef to reflect the new base, not the old one. Payload %s", filePath)
	}
	if result.Refresh {
		t.Errorf("ProcessPullRequest() Expected to NOT set refresh flag for a base-retarget event. Payload %s", filePath)
	}
}

func TestLoadCommentEvent(t *testing.T) {
	var result EventInfo
	// const payloadCommentCreated = "payload-comment-created.json"
	// const payloadCommentCreatedArgoDiff = "payload-comment-argodiff-created.json"
	payloadFiles := []string{payloadCommentCreated, payloadCommentCreatedArgoDiff}
	for _, payloadFile := range payloadFiles {
		payload, filePath, err := readFileToByteArray(payloadFile)
		if err != nil {
			t.Errorf("Failed to read %s: %v", payloadFile, err)
		}
		if err != nil {
			t.Errorf("Failed to read %s: %v", payloadCommentCreated, err)
		}
		result, err = ProcessComment(payload)
		if err != nil {
			t.Errorf("Failed to load payload from %s: %v", filePath, err)
		}
		if result.RepoOwner == "" || result.RepoName == "" || result.RepoDefaultRef == "" || result.PrNum < 1 {
			t.Errorf("ProcessComment() Result has at least one empty value: %+v; Payload %s", result, filePath)
		}
		if payloadFile == payloadCommentCreated {
			if !result.Ignore {
				t.Errorf("ProcessComment() Expected to ignore this event. Payload %s", filePath)
			}
			if result.Refresh {
				t.Errorf("ProcessComment() Expected to NOT set refresh flag. Payload %s", filePath)
			}
		}
		if payloadFile == payloadCommentCreatedArgoDiff {
			if result.Ignore {
				t.Errorf("ProcessComment() Expected to NOT ignore this event. Payload %s", filePath)
			}
			if !result.Refresh {
				t.Errorf("ProcessComment() Expected to set refresh flag. Payload %s", filePath)
			}
		}
	}
}
