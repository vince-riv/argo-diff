package webhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v58/github"
	"github.com/rs/zerolog/log"
)

// Data structure for information passed by github webhook events
type EventInfo struct {
	Ignore         bool
	RepoOwner      string
	RepoName       string
	RepoDefaultRef string
	Sha            string
	PrNum          int
	ChangeRef      string
	BaseRef        string
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
	if e.Sha == "" {
		return errors.New("missing SHA in event info object")
	}
	if e.ChangeRef == "" {
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

// Processes a pull_request event received from github
func ProcessPush(payload []byte) (EventInfo, error) {
	pushInfo := NewEventInfo()
	var pushEvent github.PushEvent
	if err := json.Unmarshal(payload, &pushEvent); err != nil {
		log.Error().Err(err).Msg("Error decoding JSON payload")
		return pushInfo, err
	}
	if pushEvent.Ref == nil || pushEvent.Before == nil || pushEvent.After == nil {
		err := errors.New("github.PushEvent missing key fields")
		log.Error().Err(err).Msg("github.PushEvent missing key fields")
		return pushInfo, err
	}
	pushInfo.RepoOwner = *pushEvent.Repo.Owner.Login
	pushInfo.RepoName = *pushEvent.Repo.Name
	pushInfo.ChangeRef = *pushEvent.Ref
	if pushEvent.HeadCommit == nil {
		log.Info().Msgf("Ignoring push event ref %s; before %s, after %s", *pushEvent.Ref, *pushEvent.Before, *pushEvent.After)
		return pushInfo, nil
	}
	if !strings.HasPrefix(pushInfo.ChangeRef, "refs/heads/") {
		log.Info().Msgf("Ignoring non-branch push event ref %s", pushInfo.ChangeRef)
		return pushInfo, nil
	}
	pushInfo.Ignore = false
	pushInfo.Sha = *pushEvent.HeadCommit.ID
	pushInfo.RepoDefaultRef = *pushEvent.Repo.DefaultBranch
	log.Debug().Msgf("Returning EventInfo: %+v", pushInfo)
	return pushInfo, validateEventInfo(pushInfo)
}
