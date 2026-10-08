package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/google/go-github/v92/github"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/comment"
	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

const (
	eventHeader     = "X-GitHub-Event"
	signatureHeader = "X-Hub-Signature-256"
)

// webhookSecret is GITHUB_WEBHOOK_SECRET, the HMAC key GitHub signs webhook
// payloads with.
var webhookSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")

// WebhookHandler verifies and parses GitHub webhook requests.
type WebhookHandler struct{}

var _ scm.WebhookHandler = WebhookHandler{}

func (WebhookHandler) EventName(h http.Header) string {
	return h.Get(eventHeader)
}

// Verify checks the X-Hub-Signature-256 HMAC against GITHUB_WEBHOOK_SECRET.
func (WebhookHandler) Verify(h http.Header, body []byte) error {
	if !VerifySignature(body, h.Get(signatureHeader), webhookSecret) {
		return errors.New("invalid signature")
	}
	return nil
}

// Parse handles ping, pull_request and issue_comment events; every other
// event type is reported as scm.WebhookIgnored.
func (WebhookHandler) Parse(h http.Header, body []byte) (scm.WebhookEvent, error) {
	evt := scm.WebhookEvent{Name: h.Get(eventHeader), Info: webhook.NewEventInfo()}
	var err error
	switch evt.Name {
	case "ping":
		evt.Kind = scm.WebhookPing
		return evt, nil
	case "pull_request":
		evt.Kind = scm.WebhookChange
		evt.Info, err = ProcessPullRequest(body)
	case "issue_comment":
		evt.Kind = scm.WebhookChange
		evt.Info, err = ProcessComment(body)
	default:
		evt.Kind = scm.WebhookIgnored
		return evt, nil
	}
	evt.Info.Provider = providerName
	return evt, err
}

// Processes a pull_request event received from github
func ProcessPullRequest(payload []byte) (webhook.EventInfo, error) {
	prInfo := webhook.NewEventInfo()
	var prEvent github.PullRequestEvent
	if err := json.Unmarshal(payload, &prEvent); err != nil {
		log.Error().Err(err).Msg("Error decoding JSON payload")
		return prInfo, err
	}
	if prEvent.Action == nil {
		err := errors.New("github.PullRequestEvent missing key field")
		log.Error().Err(err).Msg("github.PushEvent missing key field")
		return prInfo, err
	}
	prInfo.RepoOwner = *prEvent.Repo.Owner.Login
	prInfo.RepoName = *prEvent.Repo.Name
	prInfo.PrNum = *prEvent.Number
	action := *prEvent.Action
	// GitHub sends the "edited" action for title and body edits too, but it only fills in
	// changes.base.ref.from when the base itself changed — which is what happens when GitHub
	// retargets a stacked PR onto main after its parent branch merges. The accessor chain is
	// nil-safe, so a payload with no base change simply yields "".
	oldBaseRef := prEvent.GetChanges().GetBase().GetRef().GetFrom()
	isBaseRetarget := action == "edited" && oldBaseRef != ""
	if action != "opened" && action != "synchronize" && !isBaseRetarget {
		log.Info().Msg(fmt.Sprintf("Ignoring %s action for PR %s#%d", action, prEvent.Repo.GetFullName(), *prEvent.Number))
		return prInfo, nil
	}
	if isBaseRetarget {
		log.Info().Msgf("PR %s#%d retargeted from %s to %s; treating as actionable",
			prEvent.Repo.GetFullName(), *prEvent.Number, oldBaseRef, *prEvent.PullRequest.Base.Ref)
	}
	prInfo.Ignore = false
	prInfo.Sha = *prEvent.PullRequest.Head.SHA
	prInfo.RepoDefaultRef = *prEvent.Repo.DefaultBranch
	prInfo.BaseRef = *prEvent.PullRequest.Base.Ref
	prInfo.ChangeRef = *prEvent.PullRequest.Head.Ref
	log.Debug().Msgf("Returning EventInfo: %+v", prInfo)
	return prInfo, prInfo.Validate()
}

// Processes a comment created event received from github
func ProcessComment(payload []byte) (webhook.EventInfo, error) {
	prInfo := webhook.NewEventInfo()
	var commentEvent github.IssueCommentEvent
	if err := json.Unmarshal(payload, &commentEvent); err != nil {
		log.Error().Err(err).Msg("Error decoding JSON payload")
		return prInfo, err
	}
	if action := commentEvent.GetAction(); action != "created" {
		log.Info().Msgf("Ignoring issue comment event with action %s", action)
		return prInfo, nil
	}
	issue := commentEvent.GetIssue()
	issueComment := commentEvent.GetComment()
	repo := commentEvent.GetRepo()
	if issue == nil || issueComment == nil || repo == nil {
		log.Warn().Msg("Ignoring issue comment event with missing field(s)")
		return prInfo, nil
	}
	if issue.PullRequestLinks == nil {
		log.Info().Msg("Ignoring non-pull issue comment")
		return prInfo, nil
	}
	prInfo.PrNum = *issue.Number
	prInfo.RepoOwner = *repo.Owner.Login
	prInfo.RepoName = *repo.Name
	prInfo.RepoDefaultRef = *repo.DefaultBranch
	if issueComment.Body == nil || !comment.IsRefreshComment(*issueComment.Body) {
		log.Info().Msg("Ignoring pull request comment")
		return prInfo, nil
	}
	prInfo.Ignore = false
	prInfo.Refresh = true
	log.Debug().Msgf("Returning EventInfo: %+v", prInfo)
	return prInfo, prInfo.Validate()
}
