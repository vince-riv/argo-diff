package gitlab

import (
	"net/http"
	"testing"

	"github.com/vince-riv/argo-diff/internal/scm"
)

// Until Phases 3 and 4 of issue #160, GitLab runs only from event files: no
// CI detection, and webhooks are rejected without failing server startup.
func TestCIAndWebhookPlaceholders(t *testing.T) {
	p := Provider{}
	if p.DetectCI() {
		t.Error("DetectCI() = true")
	}
	if _, err := p.EventFromCIEnv(); err == nil {
		t.Error("EventFromCIEnv() = nil error")
	}
	h := p.WebhookHandler()
	if err := h.CheckConfig(); err != nil {
		t.Errorf("CheckConfig() = %v, want nil so the server still starts", err)
	}
	hdr := http.Header{}
	hdr.Set("X-Gitlab-Event", "Merge Request Hook")
	if got := h.EventName(hdr); got != "Merge Request Hook" {
		t.Errorf("EventName() = %q", got)
	}
	if err := h.Verify(hdr, []byte("{}")); err == nil {
		t.Error("Verify() = nil, want every request rejected")
	}
	if evt, err := h.Parse(hdr, []byte("{}")); err == nil || evt.Kind != scm.WebhookIgnored {
		t.Errorf("Parse() = %+v, %v; want ignored with an error", evt, err)
	}
}

func TestName(t *testing.T) {
	if got := (Provider{}).Name(); got != "gitlab" {
		t.Errorf("Name() = %q", got)
	}
}
