// Command webhook-parse reads one raw HTTP webhook request from stdin, parses
// it with a provider's scm.WebhookHandler, and prints the result as JSON to
// stdout. It is a CI-only tool for the webhook integration test: it is not
// built into the release binary or the image.
//
//	go run ./cmd/webhook-parse --provider github --verify < request.http
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/vince-riv/argo-diff/internal/github"
	"github.com/vince-riv/argo-diff/internal/gitlab"
	"github.com/vince-riv/argo-diff/internal/scm"
	"github.com/vince-riv/argo-diff/internal/webhook"
)

// knownProviders are the providers whose webhook handlers this tool can run.
// They are not registered with scm, so no credentials are needed.
var knownProviders = []scm.Provider{github.Provider{}, gitlab.Provider{}}

// result is the JSON document printed to stdout.
type result struct {
	Provider string `json:"provider"`
	// Name is the provider's own event type (eg: X-GitHub-Event).
	Name string `json:"name"`
	// Kind is "change", "ping" or "ignored".
	Kind     string             `json:"kind"`
	Verified bool               `json:"verified"`
	Info     *webhook.EventInfo `json:"info,omitempty"`
	Error    string             `json:"error,omitempty"`
}

func kindString(k scm.WebhookKind) string {
	switch k {
	case scm.WebhookChange:
		return "change"
	case scm.WebhookPing:
		return "ping"
	default:
		return "ignored"
	}
}

func main() {
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	if lvl, err := zerolog.ParseLevel(strings.ToLower(os.Getenv("LOG_LEVEL"))); err == nil && lvl != zerolog.NoLevel {
		zerolog.SetGlobalLevel(lvl)
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses the request on stdin and writes the result to stdout. It always
// writes a result once the request is read, and returns an error when
// verification or parsing failed, so the caller can exit non-zero.
func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("webhook-parse", flag.ContinueOnError)
	providerName := fs.String("provider", scm.DefaultProvider, "provider whose webhook handler parses the request")
	verify := fs.Bool("verify", false, "verify the request signature first (eg: needs GITHUB_WEBHOOK_SECRET)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: webhook-parse [flags] < request\n\n"+
			"Reads one raw HTTP request (request line, headers, blank line, body) from stdin\n"+
			"and prints the parsed event as JSON.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	idx := slices.IndexFunc(knownProviders, func(p scm.Provider) bool { return p.Name() == *providerName })
	if idx < 0 {
		return fmt.Errorf("unknown provider %q", *providerName)
	}
	p := knownProviders[idx]

	req, err := http.ReadRequest(bufio.NewReader(stdin))
	if err != nil {
		return fmt.Errorf("reading HTTP request from stdin: %w", err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("reading HTTP request body: %w", err)
	}

	handler := p.WebhookHandler()
	res := result{Provider: p.Name(), Name: handler.EventName(req.Header)}
	runErr := func() error {
		if *verify {
			if err := handler.Verify(req.Header, body); err != nil {
				return fmt.Errorf("verify: %w", err)
			}
			res.Verified = true
		}
		evt, err := handler.Parse(req.Header, body)
		res.Name = evt.Name
		res.Kind = kindString(evt.Kind)
		if evt.Kind == scm.WebhookChange {
			res.Info = &evt.Info
		}
		if err != nil {
			return fmt.Errorf("parse: %w", err)
		}
		return nil
	}()
	if runErr != nil {
		res.Error = runErr.Error()
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return errors.Join(runErr, enc.Encode(res))
}
