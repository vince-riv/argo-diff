package scm

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/vince-riv/argo-diff/internal/comment"
)

// DefaultProvider serves events that don't name a provider, so event files
// and webhooks from before multi-provider support keep working.
const DefaultProvider = "github"

// Provider is everything argo-diff needs from one source control provider to
// process a change request. process_event.ProcessCodeChange() talks to the
// provider only through this interface.
type Provider interface {
	Commenter
	// Name is the registry key, eg: "github". Lower case.
	Name() string
	// Dialect is how comments are rendered for this provider.
	Dialect() comment.Dialect
	// ListChangedFiles returns the paths the change request touches.
	ListChangedFiles(ctx context.Context, repo RepoRef, num int) ([]string, error)
	// SetStatus sets the commit status for sha. dryRun (dev mode) logs instead
	// of calling the API.
	SetStatus(ctx context.Context, repo RepoRef, sha string, state Status, description string, dryRun bool) error
	// WebhookHandler verifies and parses this provider's webhook requests.
	WebhookHandler() WebhookHandler
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Provider{}
)

// Register adds p to the registry, replacing any provider of the same name.
// cmd/main.go registers every provider at startup.
func Register(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[p.Name()] = p
}

// Lookup returns the provider registered as name; an empty name means
// DefaultProvider.
func Lookup(name string) (Provider, error) {
	if name == "" {
		name = DefaultProvider
	}
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown or disabled source control provider %q", name)
	}
	return p, nil
}

// Providers returns every registered provider, ordered by name.
func Providers() []Provider {
	registryMu.RLock()
	defer registryMu.RUnlock()
	res := make([]Provider, 0, len(registry))
	for _, p := range registry {
		res = append(res, p)
	}
	slices.SortFunc(res, func(a, b Provider) int {
		switch {
		case a.Name() < b.Name():
			return -1
		case a.Name() > b.Name():
			return 1
		}
		return 0
	})
	return res
}
