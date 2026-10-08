# internal/comment/

Renders ArgoCD diffs into PR/MR comment bodies, and wraps each body with the operator preamble and
the identifier marker argo-diff finds its own comments by. Provider-neutral: per-provider rules come
in through a `Dialect`. Nothing here calls an API.

| File | Contents |
| ---- | -------- |
| `markdown.go` | `CommentMarkdown` / `ArgoAppMarkdown` — renders diffs into comment bodies and splits them across comments |
| `dialect.go` | `Dialect` (per-provider rules), `GitHub`, `GitLab`, `Dialects` |
| `wrap.go` | `Wrap()`, `Identifier()`, `SetIdentifierRef()`, `boundPreamble()` — what surrounds every posted body |
| `refresh.go` | `IsRefreshComment()` — whether a comment on a change request asks argo-diff to re-run |
| `markdown_test.go` | Rendering, splitting and budget tests; no golden fixtures, assertions are on invariants |
| `dialect_test.go` | Runs `checkBodyWellFormed()` and the budget check under every dialect in `Dialects` |

## Dialects

A `Dialect` carries what differs between providers. Today that is `HardMax`, the largest body the
API accepts, in bytes (GitHub: 262144; GitLab: 1048576, ie: 1 MiB, verified in issue #160
Phase 0). `CommentMarkdown.Dialect` selects one; **the zero value means `GitHub`**, the
default provider. That fallback lives in `Dialect.orDefault()`, which `commentMaxLen()` and
`commentBudget()` apply themselves, so even an `ArgoAppMarkdown` built without `AppMarkdown()` is
never sized against a `HardMax` of 0. `AppMarkdown()` copies the dialect into each `ArgoAppMarkdown`, so set
`Dialect` before the first `AppMarkdown()` call.

Every supported dialect uses the GitHub alert syntax and renders alerts only at the top level, so
alerts are always hoisted (see below). GitLab (17.10+, the first release with alerts) does render
alerts inside `<details>` on gitlab.com, but that was not checked on 17.10, so its dialect hoists
them too. A provider that differs gets a new `Dialect` field, not a
branch on `Name`. Dialects live here rather than in provider packages so tests in this package can
render through all of them (`Dialects`) without an import cycle. Add a new dialect to `Dialects`.

## Refresh comments

`IsRefreshComment()` matches any keyword in `refreshCommentKeywords` (parsed once in `init()` from
`ARGO_DIFF_REFRESH_COMMENT_KEYWORDS`, a comma-separated list defaulting to `argo diff,argo-diff`),
each optionally suffixed with the context string (case-insensitive, trimmed). Every provider's
webhook parser uses it, so the keywords work the same on every provider. Tests assign
`refreshCommentKeywords` / `lowerContextStr` directly.

## Wrapper and marker

`init()` reads `ARGO_DIFF_CONTEXT_STR` and `ARGO_DIFF_COMMENT_PREAMBLE` (falling back to the
context string) into package vars. The marker is `<!-- comment produced by argo-diff[<context>] -->`.
A provider running in CI calls `SetIdentifierRef(ref)` from its own `init()` to embed the change's
ref (GitHub Actions: `GITHUB_REF`), so concurrent runs don't match each other's comments.

## Rendering

One comment reads top to bottom as: the preamble (change counts and timestamp, from
`process_event`), any `Notices`, the summary index table, then per application a `---` rule, that
application's alerts, and a `<details>` block holding one `<details>` block per changed resource.

- **Severity is carried by the GitHub alert type**, because a reader has to tell "this diff is
  untrustworthy" from "this diff is fine, but FYI" at a glance:
  | Field | Alert | Meaning |
  | ----- | ----- | ------- |
  | `CommentMarkdown.Notices` | `> [!NOTE]` (blue) | comment-level advisory, from `config.Notices()` |
  | `ArgoAppMarkdown.NoticeStr` | `> [!NOTE]` (blue) | this app's diff is good, something alongside it degraded |
  | `ArgoAppMarkdown.ErrStr` | `> [!CAUTION]` (red) | this app's diff **failed**; no resources are shown |
  | `timeoutMarkdown()` in `process_event` | `> [!WARNING]` (yellow) | applications left undiffed |
  `ErrStr` and `NoticeStr` are separate fields on purpose — they used to share one, distinguished
  only by a `"Error: "` string prefix at the call site.

### Alerts only render at the top level

**GitHub renders its coloured alert boxes only at the top level of a document.** Inside any HTML
element — `<details>` included — the extension is skipped and the block degrades to a plain
blockquote whose first line is a literal `[!NOTE]`. Nothing changes that: not extra blank lines,
not a blank line before `<summary>`, not a `<div>` wrapper, and not nesting inside another
blockquote. Verified against GitHub's own renderer (`POST /markdown`, and the `body_html` of a real
PR).

So `ArgoAppMarkdown.alerts()` is emitted by `String()` **outside** the app's `<details>`, between
the `---` rule and the block. Consequences worth knowing:

- A hoisted alert is detached from the block, so it **names its application**. `NoticeStr` from
  `internal/argocd` also names the app, so the rendered line repeats it once — accepted, because a
  notice that is self-contained at the `argocd` layer is worth more than the tidier sentence.
- An application with `ErrStr` and no resources renders as **just its alert** — no `<details>` at
  all. There are no diffs to fold, and the alert already carries the name, both statuses, the error
  text and the ArgoCD link.
- `appOpen()` never folds an application carrying a `NoticeStr` or `ErrStr`, whatever the auto
  thresholds say (an explicit `ARGO_DIFF_COMMENT_COLLAPSE=collapsed` still wins).
- `checkBodyWellFormed()` — a **test helper** in `markdown_test.go`, not a runtime check — walks
  `<details>` depth and fails on any `> [!` line nested inside one. That is the regression guard,
  and it only runs under `go test`. Nothing validates this at runtime, which is why the guard
  matters: the emitted string looks perfectly fine from Go.
- A fenced block inside an alert only renders when **every** one of its lines carries the `> `
  prefix, blank lines included — that is what `blockquote()` is for.
- `syncString()` / `healthString()` emit literal Unicode emoji, not `:shortcodes:`. These strings
  also land inside raw HTML `<summary>` elements, where shortcode substitution is not reliable.
- Blocks are folded per `ARGO_DIFF_COMMENT_COLLAPSE` (`expanded` | `collapsed` | `auto`); an empty
  or unrecognized value both fall back to `expanded`, the default. `auto` splits the fold decision
  in two: `appOpen()` folds an application's own block once the comment covers more than
  `ARGO_DIFF_COMMENT_COLLAPSE_APP_COUNT` (default 3) applications, and `resourceOpen()` folds that
  application's individual resource (diff) blocks once it has more than
  `ARGO_DIFF_COMMENT_COLLAPSE_RESOURCE_COUNT` (default 5) changed resources — the two thresholds are
  independent counts read via `collapseAppCount()`/`collapseResourceCount()`, which warn and fall
  back to the default on a non-positive value, the same guard `lineMaxChars()` uses. Because that
  decision needs totals `AddResourceDiff()` doesn't have, a resource stores its `Summary` and
  `Body` separately and `String()` renders it — nothing pre-renders a `<details>` tag.
- **Ignorable folding** (`auto` only). While `config.CollapseIgnorableActive()` is true — mode `auto`
  and `ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE` not `false` — the ignorable rule **replaces** the count
  thresholds: `resourceOpenFor()` folds a resource iff it is `Ignorable`, and `appOpen()` folds an
  application iff `allIgnorable()` (the Notice/Err guard still wins). `collapseAppCount()` and
  `collapseResourceCount()` are not consulted. With the rule off, `auto` is unchanged. This package
  does not know about regexes: `process_event` calls `AddIgnorableResourceDiff()` for resources that
  `internal/ignorable` classified. `addResourceDiff()` drops the flag unless the rule is active, so in
  `expanded`/`collapsed` an ignorable resource renders exactly like an ordinary one.
  Rendering: ` · 🔕 ignorable` is appended to the resource summary **before** `maxResourceBodyLen()`
  counts it; the app summary reads `N changed · 🔕 M ignorable`; the index cell reads `N (🔕 M)`; and
  one `<sub>🔕 M of N changed resources …</sub>` line follows the index table, outside every
  `<details>`, so a reader can see that argo-diff folded things for them. All of these appear only
  when the ignorable count is greater than 0.
- The index table is gated by `ARGO_DIFF_COMMENT_INDEX_COUNT`: `-1` always, `0` never, any other `n`
  once `n` applications have an entry (default 2). Capped at `maxIndexRows` (50).

## The comment size budget

`String()` splits its output across as many bodies as it takes, and **every** append is checked
against a budget:

```
budget := commentMaxLen() - commentWrapperLen()
limit  := budget - splitReserve
```

- `commentMaxLen(d)` is `ARGO_DIFF_COMMENT_MAX_CHARS`, defaulting to — and clamped at — the
  dialect's `HardMax` (GitHub: `githubCommentHardMax`, 262144; GitLab: `gitlabNoteHardMax`,
  1048576). Every length here is `len()`, in **bytes**. GitLab counts its limit in bytes, so
  measuring runes would let multi-byte text overshoot it.
- `commentBudget()` has a `minResourceLen` floor. Below roughly
  `len(truncatedMarker)+truncTailReserve` there is no room for `finalize()` to truncate *into*, so
  it would emit a marker that is itself over budget. The warning is the useful part, since nothing
  else tells an operator their preamble has eaten the comment.
  **The floor is itself clamped against the dialect's `HardMax`.** Raising the budget above the
  headroom that genuinely remains turns a useless-but-postable body into a 422, and posting nothing
  is worse than posting something tiny. `boundPreamble()` puts that out of reach for the preamble,
  but `commentIdentifier` carries `ARGO_DIFF_CONTEXT_STR`, which stays unbounded because comment
  matching depends on it — so the clamp is a live guard, not belt-and-braces.
- `boundPreamble()` in `wrap.go` caps `ARGO_DIFF_COMMENT_PREAMBLE` at `maxPreambleLen` (4000).
  It takes the variable name as an argument because the preamble falls back to
  `ARGO_DIFF_CONTEXT_STR` — an operator with a long context string and no preamble set would
  otherwise be told to shorten a variable they never configured.
  Every input to the budget is now bounded; the preamble was the last one that was not, and
  `commentWrapperLen()` subtracts it from every body. README documents "150 chars or less", so the
  cap is generous by design — it exists to bound the input, not to police the guideline.
- `commentWrapperLen()` is what `Wrap()` adds around each body: `ARGO_DIFF_COMMENT_PREAMBLE`
  (unbounded operator input) plus `commentIdentifier`. It is `len(Wrap(""))`, derived from the
  same function every provider posts with, so the two cannot drift. **This is the leak that used to
  produce a 422 and no comment at all**: a long preamble plus a full-size body.
- `splitReserve` covers everything appended *after* a fit check has already passed — the closing
  `</details>`, the continuation marker, and the `part i of n` header `finalize()` prepends.
- `ArgoAppMarkdown.maxResourceBodyLen()` bounds a single resource to what is left of one body once
  **everything `String()` emits ahead of it** is subtracted — the `---` rule, this app's alerts
  (up to `maxNoticeLen` each, so ~8KB), the `<details>` header and the resource's own markup — so a
  diff can never be too big for any body. Over it, the resource renders as
  `<<< DIFF TOO LARGE TO DISPLAY >>>`. Missing the alerts here is what used to overshoot the budget
  and drop a body into `finalize()` mid-markup.
- `ErrStr`, `NoticeStr` and each entry in `Notices` are bounded to `maxNoticeLen` (4000) by
  `truncateNotice()`; `HealthMsg` to `maxHealthMsgLen` (500). All three come from unbounded input
  (`err.Error()`, operator config, the cluster) and share the budget with the diffs.
- Individual lines longer than `COMMENT_LINE_MAX_CHARS` (default 175) get `...[TRUNCATED]`.
  `truncateLines()` always returns a string ending in exactly one newline, which is what lets a
  caller close a code fence on its own line — an unclosed fence swallows every diff below it.
- `finalize()` is the last guard: any body still over budget is hard-truncated with a visible
  marker. A 422 means no comment at all, which is worse than a truncated one. The cut lands at an
  arbitrary byte — almost always inside a resource, since resources are the bulk of a body — so it
  **closes what it cuts**: an odd ` ``` ` count gets a closing fence, then the marker, then one
  `</details>` per unclosed tag. Without that the fence swallows the marker and the reader sees the
  truncation notice as diff text. `truncTailReserve` holds the room for it.
- Every body is self-contained: balanced `<details>` tags and an even number of code fences. A
  mid-application split closes the app block before the continuation marker.
- `ARGOCD_UI_BASE_URL` adds a link to each app; the app path is hardcoded to `/applications/argocd/`.

## Tests

`markdown_test.go` sets the package vars `commentPreamble`, `commentIdentifier` and `argocdUiUrl`
directly (`init()` reads them, so `t.Setenv` can't reach them) and drives the rest through
`t.Setenv`. `checkBodyWellFormed()` asserts the per-body invariants above on every body of every
splitting test; `TestEveryDialectWellFormed` repeats it for each entry in `Dialects`.
