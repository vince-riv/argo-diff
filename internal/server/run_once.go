package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/vince-riv/argo-diff/internal/process_event"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

func eventInfoFromEnv() (*webhook.EventInfo, error) {
	if ghEvent := os.Getenv("GITHUB_EVENT_NAME"); ghEvent != "pull_request" {
		return nil, fmt.Errorf("unexpected value for GITHUB_EVENT_NAME: %s (expecting pull_request)", ghEvent)
	}
	prRef := os.Getenv("GITHUB_REF")
	prRefParts := strings.SplitN(prRef, "/", 4)
	prNum, err := strconv.Atoi(prRefParts[2])
	if err != nil {
		return nil, fmt.Errorf("failed extract pull request number from GITHUB_REF %s: %s", prRef, err.Error())
	}
	repoParts := strings.SplitN(os.Getenv("GITHUB_REPOSITORY"), "/", 2)
	evt := webhook.EventInfo{
		RepoOwner:      repoParts[0],
		RepoName:       repoParts[1],
		RepoDefaultRef: os.Getenv("REPO_DEFAULT_REF"),
		PrNum:          prNum,
		ChangeRef:      os.Getenv("GITHUB_HEAD_REF"),
		BaseRef:        os.Getenv("GITHUB_BASE_REF"),
		Refresh:        true, // have argo-diff refresh sha, change-ref, and base-ref
	}

	return &evt, nil
}

func eventInfoFromFile(filePath string) (*webhook.EventInfo, error) {
	var reader io.Reader

	if filePath == "-" {
		// read from stdin
		reader = os.Stdin
	} else {
		// read from file
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()
		reader = file
	}

	var evt webhook.EventInfo
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&evt); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}
	return &evt, nil
}

func ProcessFileEvent(filePath string, devMode bool) error {
	log.Debug().Msgf("processFileEvent('%s')", filePath)
	evtp, err := eventInfoFromFile(filePath)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to read event from %s", filePath)
		return err
	}
	log.Info().Msgf("Processing event data from %s: %+v", filePath, *evtp)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go process_event.ProcessCodeChange(*evtp, devMode, &wg)
	wg.Wait()
	return nil
}

func ProcessGithubAction() error {
	log.Debug().Msg("ProcessGithubAction()")
	evtp, err := eventInfoFromEnv()
	if err != nil {
		return err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go process_event.ProcessCodeChange(*evtp, true, &wg)
	wg.Wait()
	return nil
}
