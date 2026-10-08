package server

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/vince-riv/argo-diff/internal/process_event"
	"github.com/vince-riv/argo-diff/internal/scm"
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
		"ARGO_DIFF_COMMENT_NOTICE",
		"ARGO_DIFF_COMMENT_COLLAPSE",
		"ARGO_DIFF_COMMENT_COLLAPSE_APP_COUNT",
		"ARGO_DIFF_COMMENT_COLLAPSE_RESOURCE_COUNT",
		"ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE",
		"ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE_REGEXES",
		"ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE_EXCLUDE_KINDS",
		"ARGO_DIFF_COMMENT_INDEX_COUNT",
		"ARGO_DIFF_COMMENT_MAX_CHARS",
		"ARGO_DIFF_MAX_WORKERS",
		"ARGO_DIFF_TIMEOUT",
		"ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS",
		"COMMENT_LINE_MAX_CHARS",
	}
	for _, key := range nonSensitiveVars {
		log.Debug().Str(key, redactEnvValue(key, false)).Msg("")
	}

	log.Debug().Msg("=== End Environment Variables ===")
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
	p, err := scm.Lookup(evtp.Provider)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go process_event.ProcessCodeChange(p, *evtp, devMode, &wg, &err)
	wg.Wait()
	return err
}

// ProcessCI runs once for the change request described by provider p's CI
// environment (eg: GitHub Actions). Commit statuses are skipped there; the
// returned error is the job's verdict.
func ProcessCI(p scm.Provider) error {
	log.Debug().Msgf("ProcessCI(%s)", p.Name())
	logEnvironmentVariables()
	evt, err := p.EventFromCIEnv()
	if err != nil {
		return err
	}
	wg := sync.WaitGroup{}
	wg.Add(1)
	go process_event.ProcessCodeChange(p, evt, true, &wg, &err)
	wg.Wait()
	return err
}
