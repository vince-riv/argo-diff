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

// redactEnvValue returns a redacted version of an environment variable value.
// For sensitive values, it shows only the first 3 characters followed by "***".
// Returns "<not set>" if the variable is not set.
func redactEnvValue(key string, isSensitive bool) string {
	value := os.Getenv(key)
	if value == "" {
		return "<not set>"
	}
	if !isSensitive {
		return value
	}
	if len(value) <= 3 {
		return "***"
	}
	return value[:3] + "***"
}

// logEnvironmentVariables logs all relevant environment variables for debugging,
// redacting sensitive values to show only the first 3 characters.
func logEnvironmentVariables() {
	log.Debug().Msg("=== Environment Variables ===")

	// Sensitive variables - show only first 3 chars
	sensitiveVars := []string{
		"ARGOCD_AUTH_TOKEN",
		"GITHUB_WEBHOOK_SECRET",
		"GITHUB_PERSONAL_ACCESS_TOKEN",
		"GITHUB_TOKEN",
		"GITHUB_APP_PRIVATE_KEY",
	}
	for _, key := range sensitiveVars {
		log.Debug().Str(key, redactEnvValue(key, true)).Msg("")
	}

	// Non-sensitive variables - show full value
	nonSensitiveVars := []string{
		"ARGOCD_SERVER_ADDR",
		"ARGOCD_UI_BASE_URL",
		"ARGOCD_SERVER_INSECURE",
		"ARGOCD_SERVER_PLAINTEXT",
		"ARGOCD_GRPC_WEB",
		"ARGOCD_GRPC_WEB_ROOT_PATH",
		"ARGOCD_CLI_CMD_NAME",
		"GITHUB_APP_ID",
		"GITHUB_APP_INSTALLATION_ID",
		"APP_ENV",
		"LOG_LEVEL",
		"GITHUB_ACTIONS",
		"GITHUB_EVENT_NAME",
		"GITHUB_REF",
		"GITHUB_REPOSITORY",
		"REPO_DEFAULT_REF",
		"GITHUB_HEAD_REF",
		"GITHUB_BASE_REF",
		"ARGO_DIFF_CONTEXT_STR",
		"ARGO_DIFF_CI",
		"ARGO_DIFF_COMMENT_PREAMBLE",
		"COMMENT_LINE_MAX_CHARS",
	}
	for _, key := range nonSensitiveVars {
		log.Debug().Str(key, redactEnvValue(key, false)).Msg("")
	}

	log.Debug().Msg("=== End Environment Variables ===")
}

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
	logEnvironmentVariables()
	evtp, err := eventInfoFromFile(filePath)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to read event from %s", filePath)
		return err
	}
	log.Info().Msgf("Processing event data from %s: %+v", filePath, *evtp)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go process_event.ProcessCodeChange(*evtp, devMode, &wg, &err)
	wg.Wait()
	return err
}

func ProcessGithubAction() error {
	log.Debug().Msg("ProcessGithubAction()")
	logEnvironmentVariables()
	evtp, err := eventInfoFromEnv()
	if err != nil {
		return err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go process_event.ProcessCodeChange(*evtp, true, &wg, &err)
	wg.Wait()
	return err
}
