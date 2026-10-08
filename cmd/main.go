package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/config"
	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/ignorable"
	"github.com/vince-riv/argo-diff/internal/process_event"
	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/server"
)

// Version is set via -ldflags at build time
var Version = "dev"

var serverListenHost string
var serverListenPort int
var serverDevMode bool
var eventFile string

func init() {
	// Load GitHub secrets from env and setup logger
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

	// Version is set via -ldflags at build time, fallback to git describe for local development
	if Version == "dev" {
		cmd := exec.Command("git", "describe", "--always", "--dirty", "--exclude", "chart-*", "--exclude", "actions-*")
		output, err := cmd.Output()
		if err != nil {
			log.Info().Msg(fmt.Sprintf("Cannot run git describe; using default version: %s", Version))
		} else {
			Version = strings.TrimSpace(string(output))
		}
	}

	log.Logger = log.With().Str("service", "argo-diff").Str("version", Version).Caller().Logger()

	flag.StringVarP(&serverListenHost, "host", "H", "", "Listen ip/host for server")
	flag.IntVarP(&serverListenPort, "port", "p", 8080, "Listen port for server")
	flag.StringVarP(&eventFile, "event-file", "f", "", "Run once and read event data from file")
}

func startServer(listenHost string, listenPort int, devMode bool) {
	addr := fmt.Sprintf("%s:%d", listenHost, listenPort)
	if addr == ":0" {
		addr = ":8080"
	}
	server.StartWebhookProcessor(addr, devMode)
}

// knownProviders lists every scm provider argo-diff supports. main()
// registers the ones whose credentials are present.
var knownProviders = []scm.Provider{github.Provider{}}

// registerProviders registers every enabled provider and fatals when none is,
// or when an enabled one is misconfigured.
func registerProviders() []scm.Provider {
	var hints []string
	for _, p := range knownProviders {
		if !p.Enabled() {
			log.Debug().Msgf("%s credentials not set - %s support is disabled", p.Name(), p.Name())
			hints = append(hints, p.CredentialsHint())
			continue
		}
		if err := p.ValidateConfig(); err != nil {
			log.Fatal().Err(err).Msgf("Invalid %s configuration", p.Name())
		}
		log.Info().Msgf("%s support is enabled", p.Name())
		scm.Register(p)
	}
	providers := scm.Providers()
	if len(providers) == 0 {
		log.Fatal().Msgf("No source control provider credentials are set; set one of: %s", strings.Join(hints, "; "))
	}
	return providers
}

func main() {
	var err error
	flag.Parse()

	// make sure critical secrets are set in the environment
	if os.Getenv("ARGOCD_AUTH_TOKEN") == "" {
		log.Fatal().Msg("ARGOCD_AUTH_TOKEN environment variable not set")
	}
	if os.Getenv("ARGOCD_SERVER_ADDR") == "" {
		log.Fatal().Msg("ARGOCD_SERVER_ADDR environment variable not set")
	}
	providers := registerProviders()

	if os.Getenv("APP_ENV") == "dev" {
		serverDevMode = true
	}

	config.LogBypassConfig()
	ignorable.LogConfig()

	if config.BypassConnectivityCheck(config.ComponentArgoCD) {
		log.Warn().Msgf("Skipping ArgoCD connectivity check per %s", config.BypassEnvVar)
	} else if err = argocd.ConnectivityCheck(); err != nil {
		log.Fatal().Err(err).Msg("Connectivity check to ArgoCD failed")
	}

	// application sources on these hosts match a change by owner/repo exactly
	var repoHosts []string
	for _, p := range providers {
		repoHosts = append(repoHosts, p.RepoHosts()...)
	}
	argocd.SetRepoHosts(repoHosts)

	// in a provider's CI (eg: GitHub Actions), run once with event data from
	// the environment and skip the provider connectivity checks
	for _, p := range providers {
		if !p.DetectCI() {
			continue
		}
		log.Info().Msgf("Running in %s CI - running once with event data from the environment", p.Name())
		if process_event.RequireAppMatch() {
			log.Warn().Msg("ARGO_DIFF_REQUIRE_APP_MATCH is set but has no effect in CI, which skips commit statuses already")
		}
		err = server.ProcessCI(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// check provider connectivity for run-once and server modes
	for _, p := range providers {
		if err = p.ConnectivityCheck(); err != nil {
			log.Fatal().Err(err).Msgf("Connectivity check to %s API failed", p.Name())
		}
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
	for _, p := range providers {
		if err = p.WebhookHandler().CheckConfig(); err != nil {
			log.Fatal().Err(err).Msgf("%s webhooks are not configured", p.Name())
		}
	}
	startServer(serverListenHost, serverListenPort, serverDevMode)
}
