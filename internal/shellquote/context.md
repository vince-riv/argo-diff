# internal/shellquote/

A copy of [github.com/kballard/go-shellquote](https://github.com/kballard/go-shellquote) at commit
`95032a82bc51` (2018-04-28, the last upstream commit), brought in rather than imported because
upstream is unmaintained. `internal/argocd` uses `Split()` to parse `ARGOCD_APP_DIFF_OPTS` the
same way the argocd CLI parses `ARGOCD_OPTS` (the CLI imports the same library).

## Rules

- **Keep `LICENSE`.** The code is MIT-licensed, and MIT requires its copyright and permission notice
  to travel with every copy. It is compatible with argo-diff's Apache 2.0 license.
- **Keep the code as upstream wrote it**, so it stays diffable against the original. The one change
  is in `unquote.go`'s `Split` doc comment: `$''` became `$'...'`, because `gofmt` rewrites `''` in
  a doc comment to a typographic `”`. `README` is upstream's own.
- `.golangci.yaml` excludes this directory from lint: upstream's capitalized error strings
  (`ST1005`) and an `if` chain (`QF1003`) would otherwise fail CI, and fixing them would change
  the error messages and drift from upstream.
- The tests (`*_test.go`) are upstream's and run with `go test ./...`.
