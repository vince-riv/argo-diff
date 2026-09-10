package config

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

func TestNoticesFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{"unset", "", nil},
		{"whitespace only", "   ", nil},
		{"single", "feature X is deprecated", []string{"feature X is deprecated"}},
		{
			"several, separated and trimmed",
			" first notice | second notice ",
			[]string{"first notice", "second notice"},
		},
		{"empty segments dropped", "a||b|", []string{"a", "b"}},
		{"commas are not separators", "one, two, three", []string{"one, two, three"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(resetNotices)
			t.Setenv(NoticeEnvVar, tt.env)
			if got := Notices(); !slices.Equal(got, tt.want) {
				t.Errorf("Notices() = %v, want %v", got, tt.want)
			}
		})
	}
}

// AddNotice is the seam a future check (an argocd CLI too old for a feature)
// raises an advisory through. It must dedupe, because such a check naturally
// sits on a per-application code path.
func TestAddNotice(t *testing.T) {
	t.Cleanup(resetNotices)
	t.Setenv(NoticeEnvVar, "from the operator")

	AddNotice("from argo-diff")
	AddNotice("from argo-diff") // duplicate
	AddNotice("  ")             // blank
	AddNotice(" from argo-diff ")

	want := []string{"from the operator", "from argo-diff"}
	if got := Notices(); !slices.Equal(got, want) {
		t.Errorf("Notices() = %v, want %v", got, want)
	}
}

func TestAddNoticeCap(t *testing.T) {
	t.Cleanup(resetNotices)
	t.Setenv(NoticeEnvVar, "")
	for i := range maxNotices + 5 {
		AddNotice(fmt.Sprintf("notice %d", i))
	}
	if got := len(Notices()); got != maxNotices {
		t.Errorf("held %d notices, want them capped at %d", got, maxNotices)
	}
}

// internal/argocd diffs applications concurrently, so AddNotice has to be safe
// to call from any goroutine. Run with -race.
func TestAddNoticeConcurrent(t *testing.T) {
	t.Cleanup(resetNotices)
	t.Setenv(NoticeEnvVar, "")
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			AddNotice(fmt.Sprintf("notice %d", i%3))
			_ = Notices()
		}()
	}
	wg.Wait()
	if got := len(Notices()); got != 3 {
		t.Errorf("got %d distinct notices, want 3", got)
	}
}
