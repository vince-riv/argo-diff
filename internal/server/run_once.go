package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/vince-riv/argo-diff/internal/process_event"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

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
