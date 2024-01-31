package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/server"
)

const gitRevTxt = "git-rev.txt"

func init() {
	// Load GitHub secrets from env and setup logger
	gitRev := "UNKNOWN"

	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	default:
		log.Info().Msg("LOG_LEVEL env var not set or set to an unknown value. Defaulting to INFO level logging.")
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

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

	// make sure critical secrets are set in the environment
	if os.Getenv("ARGOCD_AUTH_TOKEN") == "" {
		log.Fatal().Msg("ARGOCD_AUTH_TOKEN environment variable not set")
	}
	if os.Getenv("ARGOCD_SERVER_ADDR") == "" {
		log.Fatal().Msg("ARGOCD_SERVER_ADDR environment variable not set")
	}
	if os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") == "" {
		log.Info().Msg("GITHUB_PERSONAL_ACCESS_TOKEN environment variable not set - assuming Github App installation")
		for _, e := range []string{"GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY"} {
			if os.Getenv(e) == "" {
				log.Fatal().Msgf("%s environment variable is not set for Github App installations", e)
			}
		}
	}

	if err := argocd.ConnectivityCheck(); err != nil {
		log.Fatal().Err(err).Msg("Connectivity check to ArgoCD failed")
	}

	if err := github.ConnectivityCheck(); err != nil {
		log.Fatal().Err(err).Msg("Connectivity check to Github API failed")
	}

	server.StartWebhookProcessor("", 8080, githubWebhookSecret, devMode)
}
