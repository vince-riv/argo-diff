package process_event

import (
	"fmt"
	"os"
	"strings"
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

func TestReportReserve(t *testing.T) {
	cases := map[time.Duration]time.Duration{
		3 * time.Minute:          defaultReportReserve,
		10 * time.Minute:         defaultReportReserve,
		61 * time.Second:         defaultReportReserve,
		2 * defaultReportReserve: defaultReportReserve, // at exactly 2x the reserve, half
		30 * time.Second:         15 * time.Second,
		10 * time.Second:         5 * time.Second,
	}
	for timeout, want := range cases {
		got := reportReserve(timeout)
		if got != want {
			t.Errorf("reportReserve(%s) = %s, want %s", timeout, got, want)
		}
		// diffing must always be left some time to work with
		if got >= timeout {
			t.Errorf("reportReserve(%s) = %s, leaves nothing for diffing", timeout, got)
		}
	}
}

func TestTimeoutMarkdown(t *testing.T) {
	md := timeoutMarkdown(3*time.Minute, []string{"app-a", "app-b"})
	for _, want := range []string{"[!WARNING]", "3m0s", "2 application(s)", "app-a, app-b", "ARGO_DIFF_TIMEOUT"} {
		if !strings.Contains(md, want) {
			t.Errorf("timeoutMarkdown() = %q, missing %q", md, want)
		}
	}

	// long lists get capped so the diffs aren't crowded out of the comment
	many := make([]string, 25)
	for i := range many {
		many[i] = fmt.Sprintf("app-%02d", i)
	}
	md = timeoutMarkdown(time.Minute, many)
	if !strings.Contains(md, "and 5 more") {
		t.Errorf("timeoutMarkdown() with 25 apps = %q, want it to cap the list", md)
	}
	if strings.Contains(md, "app-24") {
		t.Errorf("timeoutMarkdown() with 25 apps = %q, want names past the cap omitted", md)
	}
	if !strings.Contains(md, "25 application(s)") {
		t.Errorf("timeoutMarkdown() with 25 apps = %q, want the full count reported", md)
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
