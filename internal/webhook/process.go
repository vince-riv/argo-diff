package webhook

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/go-github/v90/github"
	"github.com/rs/zerolog/log"

	argoDiffGh "github.com/vince-riv/argo-diff/internal/github"
)

// Data structure for information passed by github webhook events
type EventInfo struct {
	Ignore         bool     `json:"ignore"`
	RepoOwner      string   `json:"owner"`
	RepoName       string   `json:"repo"`
	RepoDefaultRef string   `json:"default_ref"`
	Sha            string   `json:"commit_sha"`
	PrNum          int      `json:"pr"`
	ChangeRef      string   `json:"change_ref"`
	BaseRef        string   `json:"base_ref"`
	Refresh        bool     `json:"refresh"`
	ChangedFiles   []string `json:"changed_files,omitempty"`
}

func NewEventInfo() EventInfo {
	return EventInfo{
		Ignore:         true,
		RepoOwner:      "",
		RepoName:       "",
		RepoDefaultRef: "",
		Sha:            "",
		PrNum:          -1,
		ChangeRef:      "",
		BaseRef:        "",
		Refresh:        false,
	}
}

func validateEventInfo(e EventInfo) error {
	if e.RepoOwner == "" {
		return errors.New("missing repo owner in event info object")
	}
	if e.RepoName == "" {
		return errors.New("missing repo name in event info object")
	}
	if e.RepoDefaultRef == "" {
		return errors.New("missing default ref in event info object")
	}
	if !e.Refresh && e.Sha == "" {
		return errors.New("missing SHA in event info object")
	}
	if !e.Refresh && e.ChangeRef == "" {
		return errors.New("missing change ref in event info object")
	}
	return nil
}

// baseRefChanged reports whether an "edited" pull_request event moved the PR's base branch.
// GitHub sends the "edited" action for title and body edits too, but it only fills in
// changes.base.ref.from when the base itself changed — which is what happens when GitHub
// retargets a stacked PR onto main after its parent branch merges. All the accessors used
// here are nil-safe, so a payload with no base change simply yields "".
func baseRefChanged(prEvent *github.PullRequestEvent) bool {
	return prEvent.GetChanges().GetBase().GetRef().GetFrom() != ""
}

// Processes a pull_request event received from github
func ProcessPullRequest(payload []byte) (EventInfo, error) {
	prInfo := NewEventInfo()
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
	isBaseRetarget := action == "edited" && baseRefChanged(&prEvent)
	if action != "opened" && action != "synchronize" && !isBaseRetarget {
		log.Info().Msg(fmt.Sprintf("Ignoring %s action for PR %s#%d", action, prEvent.Repo.GetFullName(), *prEvent.Number))
		return prInfo, nil
	}
	if isBaseRetarget {
		log.Info().Msgf("PR %s#%d retargeted from %s to %s; treating as actionable",
			prEvent.Repo.GetFullName(), *prEvent.Number, prEvent.GetChanges().GetBase().GetRef().GetFrom(), *prEvent.PullRequest.Base.Ref)
	}
	prInfo.Ignore = false
	prInfo.Sha = *prEvent.PullRequest.Head.SHA
	prInfo.RepoDefaultRef = *prEvent.Repo.DefaultBranch
	prInfo.BaseRef = *prEvent.PullRequest.Base.Ref
	prInfo.ChangeRef = *prEvent.PullRequest.Head.Ref
	log.Debug().Msgf("Returning EventInfo: %+v", prInfo)
	return prInfo, validateEventInfo(prInfo)
}

// Processes a comment created event received from github
func ProcessComment(payload []byte) (EventInfo, error) {
	prInfo := NewEventInfo()
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
	if issueComment.Body == nil || !argoDiffGh.IsRefreshComment(*issueComment.Body) {
		log.Info().Msg("Ignoring pull request comment")
		return prInfo, nil
	}
	prInfo.Ignore = false
	prInfo.Refresh = true
	log.Debug().Msgf("Returning EventInfo: %+v", prInfo)
	return prInfo, validateEventInfo(prInfo)
}
