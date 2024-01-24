package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/server"
)

const debug = true // TODO: switch to command line arg for trace logs
const gitRevTxt = "git-rev.txt"

func init() {
	// Load GitHub secrets from env and setup logger
	gitRev := "UNKNOWN"

	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// git revision for logger - there's probably a better way to do this at build time
	data, err := os.ReadFile(gitRevTxt)
	if err != nil {
		log.Info().Msg(fmt.Sprintf("Cannot open %s; assuming we're in local development", gitRevTxt))
	} else {
		lines := strings.Split(string(data), "\n")
		gitRev = strings.TrimSpace(lines[0])
		if gitRev == "" {
			log.Warn().Msg(fmt.Sprintf("%s must be empty?", gitRevTxt))
			gitRev = "EMPTY"
		}
	}

	log.Logger = log.With().Str("service", "argo-diff").Str("version", gitRev).Caller().Logger()
}

func main() {
	// TODO add support for command line arguments

	githubWebhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if githubWebhookSecret == "" {
		log.Fatal().Msg("GITHUB_WEBHOOK_SECRET environment variable not set")
	}

	devMode := false
	if os.Getenv("APP_ENV") == "dev" {
		devMode = true
	}
	if devMode {
		// zerolog.SetGlobalLevel(zerolog.TraceLevel)
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	// make sure critical secrets are set in the environment
	if os.Getenv("ARGOCD_AUTH_TOKEN") == "" {
		log.Fatal().Msg("ARGOCD_AUTH_TOKEN environment variable not set")
	}
	if os.Getenv("ARGOCD_BASE_URL") == "" {
		log.Fatal().Msg("ARGOCD_BASE_URL environment variable not set")
	}
	if os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") == "" {
		log.Fatal().Msg("GITHUB_PERSONAL_ACCESS_TOKEN environment variable not set")
	}

	server.StartWebhookProcessor("", 8080, githubWebhookSecret, devMode)
}
