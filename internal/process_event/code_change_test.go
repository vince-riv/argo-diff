package process_event

import (
	"os"
	"testing"
	"time"
)

func TestProcessTimeout(t *testing.T) {
	cases := map[string]time.Duration{
		"":        defaultProcessTimeout,
		"   ":     defaultProcessTimeout,
		"5m":      5 * time.Minute,
		"90s":     90 * time.Second,
		" 2m30s ": 150 * time.Second,
		"300":     300 * time.Second,
		"bogus":   defaultProcessTimeout,
		"0":       defaultProcessTimeout,
		"0s":      defaultProcessTimeout,
		"-1m":     defaultProcessTimeout,
	}
	for envVal, want := range cases {
		t.Setenv("ARGO_DIFF_TIMEOUT", envVal)
		if got := processTimeout(); got != want {
			t.Errorf("ARGO_DIFF_TIMEOUT=%q: processTimeout() = %s, want %s", envVal, got, want)
		}
	}
}

func TestProcessTimeoutUnset(t *testing.T) {
	// t.Setenv registers the restore of any pre-existing value for us
	t.Setenv("ARGO_DIFF_TIMEOUT", "")
	if err := os.Unsetenv("ARGO_DIFF_TIMEOUT"); err != nil {
		t.Fatalf("os.Unsetenv() err'd: %v", err)
	}
	if got := processTimeout(); got != defaultProcessTimeout {
		t.Errorf("processTimeout() with ARGO_DIFF_TIMEOUT unset = %s, want %s", got, defaultProcessTimeout)
	}
}
