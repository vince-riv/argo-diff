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
	payloadFiles := []string{payloadPrClose, payloadPrOpen, payloadPrSync}
	for _, payloadFile := range payloadFiles {
		payload, filePath, err := readFileToByteArray(payloadFile)
		if err != nil {
			t.Errorf("Failed to read %s: %v", payloadFile, err)
		}
		result, err = ProcessPullRequest(payload)
		if err != nil {
			t.Errorf("Failed to load payload from %s: %v", filePath, err)
		}
		if payloadFile == payloadPrClose {
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
