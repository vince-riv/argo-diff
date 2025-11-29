package webhook

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/go-github/v79/github"
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
	if *prEvent.Action != "opened" && *prEvent.Action != "synchronize" {
		log.Info().Msg(fmt.Sprintf("Ignoring %s action for PR %s#%d", *prEvent.Action, *prEvent.Repo, *prEvent.Number))
		return prInfo, nil
	}
	prInfo.Ignore = false
	prInfo.Sha = *prEvent.PullRequest.Head.SHA
	prInfo.RepoDefaultRef = *prEvent.Repo.DefaultBranch
	prInfo.BaseRef = *prEvent.PullRequest.Base.Ref // FUTURE USE
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
