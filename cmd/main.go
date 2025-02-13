package main

import (
	"fmt"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/server"
)

const gitRevTxt = "git-rev.txt"

var serverListenHost string
var serverListenPort int
var serverDevMode bool
var eventFile string

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

	flag.StringVarP(&serverListenHost, "host", "H", "", "Listen ip/host for server")
	flag.IntVarP(&serverListenPort, "port", "p", 8080, "Listen port for server")
	flag.StringVarP(&eventFile, "event-file", "f", "", "Run once and read event data from file")
}

func startServer(listenHost string, listenPort int, githubWebhookSecret string, devMode bool) {
	addr := fmt.Sprintf("%s:%d", listenHost, listenPort)
	if addr == ":0" {
		addr = ":8080"
	}
	server.StartWebhookProcessor(addr, githubWebhookSecret, devMode)
}

func main() {
	var err error
	flag.Parse()

	githubWebhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")

	// make sure critical secrets are set in the environment
	if os.Getenv("ARGOCD_AUTH_TOKEN") == "" {
		log.Fatal().Msg("ARGOCD_AUTH_TOKEN environment variable not set")
	}
	if os.Getenv("ARGOCD_SERVER_ADDR") == "" {
		log.Fatal().Msg("ARGOCD_SERVER_ADDR environment variable not set")
	}
	if os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN") == "" && os.Getenv("GITHUB_TOKEN") == "" {
		log.Info().Msg("GITHUB_PERSONAL_ACCESS_TOKEN or GITHUB_TOKEN environment variable not set - assuming Github App installation")
		for _, e := range []string{"GITHUB_APP_ID", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY"} {
			if os.Getenv(e) == "" {
				log.Fatal().Msgf("%s environment variable is not set for Github App installations", e)
			}
		}
	}

	if os.Getenv("APP_ENV") == "dev" {
		serverDevMode = true
	}

	if err = argocd.ConnectivityCheck(); err != nil {
		log.Fatal().Err(err).Msg("Connectivity check to ArgoCD failed")
	}

	// if running under Github Actions, skip github connectivity check
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		log.Info().Msg("GITHUB_ACTIONS set in the environemtn - running once with event data from environment")
		err = server.ProcessGithubAction()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// check github connectivity for run-once and server modes
	if err = github.ConnectivityCheck(); err != nil {
		log.Fatal().Err(err).Msg("Connectivity check to Github API failed")
	}

	// if event file is defined, process it and exit
	if eventFile != "" {
		err = server.ProcessFileEvent(eventFile, serverDevMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// other assume we're running as a web server
	if githubWebhookSecret == "" {
		log.Fatal().Msg("GITHUB_WEBHOOK_SECRET environment variable not set")
	}
	startServer(serverListenHost, serverListenPort, githubWebhookSecret, serverDevMode)
}
