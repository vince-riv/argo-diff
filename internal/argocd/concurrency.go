package argocd

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// How many `argocd app diff`/`app manifests` calls GetApplicationChanges() is
// willing to have in flight at once. Kept conservative by default so a large
// monorepo doesn't send a burst of concurrent manifest generations at the
// ArgoCD repo-server.
const defaultMaxWorkers = 4
const hardMaxWorkers = 32

// maxWorkers returns the diff worker pool size from ARGO_DIFF_MAX_WORKERS.
// It's a plain function rather than an init()-cached value so tests can use
// t.Setenv. Invalid or non-positive values fall back to the default; values
// above hardMaxWorkers are clamped, since an accidentally huge value would
// hammer the repo-server.
func maxWorkers() int {
	envVal := strings.TrimSpace(os.Getenv("ARGO_DIFF_MAX_WORKERS"))
	if envVal == "" {
		return defaultMaxWorkers
	}
	n, err := strconv.Atoi(envVal)
	if err != nil || n <= 0 {
		log.Warn().Msgf("Invalid value for ARGO_DIFF_MAX_WORKERS: %s; must be a positive integer; using %d", envVal, defaultMaxWorkers)
		return defaultMaxWorkers
	}
	if n > hardMaxWorkers {
		log.Warn().Msgf("ARGO_DIFF_MAX_WORKERS %d exceeds max of %d; using %d", n, hardMaxWorkers, hardMaxWorkers)
		return hardMaxWorkers
	}
	return n
}

// runWithLimit runs work(0), work(1), ..., work(n-1), with at most limit of
// them executing concurrently, and returns once they've all finished. limit
// is clamped to at least 1. Each call to work(i) runs in its own goroutine,
// so callers must not mutate shared state from within work without their own
// synchronization; the recommended pattern is for work(i) to write only to
// index i of a pre-sized results slice.
func runWithLimit(n, limit int, work func(i int)) {
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			work(i)
		}(i)
	}
	wg.Wait()
}
