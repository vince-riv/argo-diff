# internal/gendiff/

A one-function wrapper around `github.com/akedrou/textdiff`:

```go
func UnifiedDiff(srcFile, destFile, from, to string) string
```

## Status: currently unused

**Nothing outside this package imports it.** Diffs are produced by the `argocd` CLI itself
(`argocd app diff`, with `KUBECTL_EXTERNAL_DIFF=diff -u`), and `internal/argocd` only parses that
output — it never calls `UnifiedDiff()`.

It survives from the era when argo-diff fetched manifests over the ArgoCD HTTP API and diffed them
itself, and it keeps `textdiff` in `go.mod` as a direct dependency.

Before extending it, decide whether the package should exist at all: if a future change needs local
diffing, this is the place; if not, deleting the package and the `textdiff` requirement is the
honest cleanup. Either way, say so in the commit — don't leave it ambiguous.

`gendiff_test.go` still exercises it (unified format, header lines, a Kubernetes manifest case), so
the tests pass and the linter stays quiet.
