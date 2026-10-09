package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/argocd"
	"github.com/vince-riv/argo-diff/internal/config"
	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/gitlab"
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
// registers the ones ARGO_DIFF_SCM_PROVIDERS lists.
var knownProviders = []scm.Provider{github.Provider{}, gitlab.Provider{}}

// selectProviders returns the providers of known that ARGO_DIFF_SCM_PROVIDERS
// lists (default: github), after checking each one has credentials and a
// valid configuration. A provider that is not listed is skipped even when its
// credentials are set. A listed provider without credentials, an invalid
// configuration, or an unknown name in the list is an error.
func selectProviders(known []scm.Provider) ([]scm.Provider, error) {
	var knownNames []string
	for _, p := range known {
		knownNames = append(knownNames, p.Name())
	}
	names, isDefault, err := config.ScmProviders(knownNames)
	if err != nil {
		return nil, err
	}
	var selected []scm.Provider
	for _, p := range known {
		if !slices.Contains(names, p.Name()) {
			log.Debug().Msgf("%s is not in %s - %s support is disabled", p.Name(), config.ScmProvidersEnvVar, p.Name())
			continue
		}
		if !p.Enabled() {
			if isDefault {
				return nil, fmt.Errorf("%s is the default source control provider (%s is not set) but has no credentials; set one of: %s; or set %s to the providers you use, eg: gitlab",
					p.Name(), config.ScmProvidersEnvVar, p.CredentialsHint(), config.ScmProvidersEnvVar)
			}
			return nil, fmt.Errorf("%s is listed in %s but has no credentials; set one of: %s", p.Name(), config.ScmProvidersEnvVar, p.CredentialsHint())
		}
		if err := p.ValidateConfig(); err != nil {
			return nil, fmt.Errorf("invalid %s configuration: %w", p.Name(), err)
		}
		log.Info().Msgf("%s support is enabled", p.Name())
		selected = append(selected, p)
	}
	return selected, nil
}

// registerProviders registers the selected providers and fatals when the
// selection fails.
func registerProviders() []scm.Provider {
	selected, err := selectProviders(knownProviders)
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot enable source control providers")
	}
	for _, p := range selected {
		scm.Register(p)
	}
	return scm.Providers()
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

	// application sources on a provider's hosts match that provider's
	// changes by owner/repo exactly
	for _, p := range providers {
		argocd.SetRepoHosts(p.Name(), p.RepoHosts())
	}

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
