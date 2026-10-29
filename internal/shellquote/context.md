# internal/shellquote/

A copy of [github.com/kballard/go-shellquote](https://github.com/kballard/go-shellquote) at commit
`95032a82bc51` (2018-04-28, the last upstream commit), brought in rather than imported because
upstream is unmaintained. `internal/argocd` uses `Split()` to parse `ARGOCD_APP_DIFF_OPTS` the
same way the argocd CLI parses `ARGOCD_OPTS` (the CLI imports the same library).

## Rules

- **Keep `LICENSE`.** The code is MIT-licensed, and MIT requires its copyright and permission notice
  to travel with every copy. It is compatible with argo-diff's Apache 2.0 license.
- **The code is argo-diff's now**: it follows the repo's `gofmt` and golangci-lint rules like any
  other package, with no lint exclusion. Upstream is dead, so staying diffable against it is not a
  goal. Changes from upstream so far:
  - `unquote.go`'s `Split` doc comment: `$''` became `$'...'`, because `gofmt` rewrites `''` in a
    doc comment to a typographic `”`.
  - The error variables are `ErrUnterminatedSingleQuote`, `ErrUnterminatedDoubleQuote` and
    `ErrUnterminatedEscape` (upstream: `Unterminated*Error`; staticcheck `ST1012`), and their
    messages start lowercase (`ST1005`). Callers compare the error values, not the text.
  - The `if`/`else if` on `c` in `splitWord`'s double-quote state is a `switch` (staticcheck
    `QF1003`).
- Upstream's `README` (a `godoc` text dump of the API) was dropped once the API names changed;
  the doc comments in the `.go` files are the reference.
- The tests (`*_test.go`) are upstream's and run with `go test ./...`.
