package scm

import (
	"context"
	"testing"

	"github.com/vince-riv/argo-diff/internal/comment"
)

// namedProvider is a Provider that only knows its name.
type namedProvider struct {
	*fakeCommenter
	name string
}

func (p namedProvider) Name() string             { return p.name }
func (p namedProvider) Dialect() comment.Dialect { return comment.GitHub }
func (p namedProvider) ListChangedFiles(context.Context, RepoRef, int) ([]string, error) {
	return nil, nil
}
func (p namedProvider) SetStatus(context.Context, RepoRef, string, Status, string, bool) error {
	return nil
}
func (p namedProvider) WebhookHandler() WebhookHandler { return nil }

// withRegistry gives a test an empty registry and restores the real one after.
func withRegistry(t *testing.T) {
	t.Helper()
	registryMu.Lock()
	orig := registry
	registry = map[string]Provider{}
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry = orig
		registryMu.Unlock()
	})
}

func TestRegistry(t *testing.T) {
	withRegistry(t)
	if _, err := Lookup(""); err == nil {
		t.Error("Lookup(\"\") succeeded on an empty registry")
	}

	Register(namedProvider{name: "gitlab"})
	Register(namedProvider{name: DefaultProvider})

	p, err := Lookup("")
	if err != nil || p.Name() != DefaultProvider {
		t.Errorf("Lookup(\"\") = %v, %v; want the default provider %q", p, err, DefaultProvider)
	}
	if p, err := Lookup("gitlab"); err != nil || p.Name() != "gitlab" {
		t.Errorf("Lookup(\"gitlab\") = %v, %v", p, err)
	}
	if _, err := Lookup("bitbucket"); err == nil {
		t.Error("Lookup() of an unregistered provider succeeded")
	}

	var names []string
	for _, p := range Providers() {
		names = append(names, p.Name())
	}
	if len(names) != 2 || names[0] != "github" || names[1] != "gitlab" {
		t.Errorf("Providers() = %v, want [github gitlab]", names)
	}
}
