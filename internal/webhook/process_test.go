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
const payloadPushBranchDelete = "payload-push-branch-delete.json"
const payloadPushBranchDev = "payload-push-branch-dev.json"
const payloadPushBranchMain = "payload-push-branch-main.json"
const payloadPushTag = "payload-push-tag.json"

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
		}
	}
}

func TestLoadNotAPullRequestEvent(t *testing.T) {
	var result EventInfo
	payload, filePath, err := readFileToByteArray(payloadPushBranchDelete)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadPushBranchDelete, err)
	}
	result, err = ProcessPullRequest(payload)
	if err == nil {
		t.Errorf("ProcessPullRequest() Filepath %s should have resulted in an error", filePath)
	}
	if !result.Ignore {
		t.Errorf("ProcessPullRequest() Bad event %s should have been ignored", filePath)
	}
}

func TestLoadPushEvents(t *testing.T) {
	var result EventInfo

	payloadFiles := []string{payloadPushBranchDelete, payloadPushBranchDev, payloadPushBranchMain, payloadPushTag}
	for _, payloadFile := range payloadFiles {
		payload, filePath, err := readFileToByteArray(payloadFile)
		if err != nil {
			t.Errorf("Failed to read %s: %v", payloadFile, err)
		}
		result, err = ProcessPush(payload)
		if err != nil {
			t.Errorf("Failed to load payload from %s: %v", filePath, err)
		}
		if payloadFile == payloadPushBranchDelete || payloadFile == payloadPushTag {
			if !result.Ignore {
				t.Errorf("ProcessPushRequest() Expected to ignore this event. Payload %s", filePath)
			}
		} else {
			if result.Ignore {
				t.Errorf("ProcessPushRequest() Expected to NOT ignroe this event. Payload %s", filePath)
			}
			if result.PrNum > 0 {
				t.Errorf("ProcessPushRequest() PrNum defined in result [%v]. Payload %s", result, filePath)
			}
			if result.RepoOwner == "" || result.RepoName == "" || result.RepoDefaultRef == "" || result.Sha == "" || result.ChangeRef == "" {
				t.Errorf("ProcessPushRequest() Result has at least one empty value: %+v; Payload %s", result, filePath)
			}
		}
	}
}

func TestLoadNotAPushEvent(t *testing.T) {
	var result EventInfo
	payload, filePath, err := readFileToByteArray(payloadPrOpen)
	if err != nil {
		t.Errorf("Failed to read %s: %v", payloadPrOpen, err)
	}
	result, err = ProcessPush(payload)
	if err == nil {
		t.Errorf("Filepath %s should have resulted in an error", filePath)
	}
	if !result.Ignore {
		t.Errorf("ProcessPushRequest() Bad event %s should have been ignored", filePath)
	}
}
