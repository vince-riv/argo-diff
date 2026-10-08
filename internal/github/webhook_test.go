package github

import (
	"net/http"
	"strings"
	"testing"

	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

// Webhook payload fixtures, under github_testdata/webhook/ (readFileToByteArray
// is in comment_test.go).
const payloadPrClose = "webhook/payload-pr-close.json"
const payloadPrOpen = "webhook/payload-pr-open.json"
const payloadPrSync = "webhook/payload-pr-sync.json"
const payloadPrEditedBase = "webhook/payload-pr-edited-base.json"
const payloadPrEditedTitle = "webhook/payload-pr-edited-title.json"
const payloadCommentCreated = "webhook/payload-comment-created.json"
const payloadCommentCreatedArgoDiff = "webhook/payload-comment-argodiff-created.json"

func TestLoadPullRequestEvents(t *testing.T) {
	var result webhook.EventInfo
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
// branch merges. TestLoadPullRequestEvents already checks Ignore/Refresh/non-empty fields for
// this fixture; this test adds the one assertion that is specific to a retarget: BaseRef must
// reflect the new base, not just be non-empty or merely different from the old one.
func TestPullRequestBaseRetarget(t *testing.T) {
	payload, filePath, err := readFileToByteArray(payloadPrEditedBase)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", payloadPrEditedBase, err)
	}
	result, err := ProcessPullRequest(payload)
	if err != nil {
		t.Fatalf("Failed to load payload from %s: %v", filePath, err)
	}
	if result.BaseRef != "main" {
		t.Errorf("ProcessPullRequest() Expected BaseRef to be the new base 'main', got %q. Payload %s", result.BaseRef, filePath)
	}
}

func TestLoadCommentEvent(t *testing.T) {
	var result webhook.EventInfo
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

func TestWebhookHandler(t *testing.T) {
	origSecret := webhookSecret
	t.Cleanup(func() { webhookSecret = origSecret })
	webhookSecret = testSecret
	h := WebhookHandler{}

	headers := http.Header{}
	headers.Set(eventHeader, "ping")
	headers.Set(signatureHeader, "sha256="+pingSig)
	if err := h.Verify(headers, []byte(pingPayload)); err != nil {
		t.Errorf("Verify() rejected a correctly signed ping: %v", err)
	}
	headers.Set(signatureHeader, "sha256="+strings.Repeat("f", 64))
	if err := h.Verify(headers, []byte(pingPayload)); err == nil {
		t.Error("Verify() accepted a bad signature")
	}
	if evt, err := h.Parse(headers, []byte(pingPayload)); err != nil || evt.Kind != scm.WebhookPing || evt.Name != "ping" {
		t.Errorf("Parse(ping) = %+v, %v; want a ping", evt, err)
	}

	headers.Set(eventHeader, "push")
	if evt, _ := h.Parse(headers, []byte("{}")); evt.Kind != scm.WebhookIgnored {
		t.Errorf("Parse(push) kind = %v, want WebhookIgnored", evt.Kind)
	}

	payload, _, err := readFileToByteArray(payloadPrOpen)
	if err != nil {
		t.Fatal(err)
	}
	headers.Set(eventHeader, "pull_request")
	evt, err := h.Parse(headers, payload)
	if err != nil || evt.Kind != scm.WebhookChange || evt.Info.Ignore || evt.Info.Provider != "github" {
		t.Errorf("Parse(pull_request opened) = %+v, %v; want an actionable github change", evt, err)
	}

	payload, _, err = readFileToByteArray(payloadCommentCreatedArgoDiff)
	if err != nil {
		t.Fatal(err)
	}
	headers.Set(eventHeader, "issue_comment")
	evt, err = h.Parse(headers, payload)
	if err != nil || evt.Kind != scm.WebhookChange || !evt.Info.Refresh || evt.Info.Provider != "github" {
		t.Errorf("Parse(issue_comment argo diff) = %+v, %v; want a github refresh", evt, err)
	}
}
