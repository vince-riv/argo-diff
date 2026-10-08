# internal/ignorable/

Decides which resource diffs are **ignorable**: every changed line matches a configured regex (the
default case is a Helm chart bump that only touches version labels). `internal/comment` renders those
resources folded in `auto` collapse mode. Pure package: imports `internal/config` only.

## Files

| File | Contents |
| ---- | -------- |
| `ignorable.go` | Defaults, list parsing, `LoadGlobal()`, `Global.ForApp()`, `Policy.Ignorable()`, `LogConfig()` |
| `ignorable_test.go` | Table tests |
| `ignorable_testdata/` | `DiffStr` fixtures (header line removed, `diff -u` file headers kept) |

## Gating

The feature is active only when `config.CollapseIgnorableActive()`: `ARGO_DIFF_COMMENT_COLLAPSE=auto`
and `ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE` not `false`. `internal/comment` asks the same function, so the
two cannot disagree. When inactive, `LoadGlobal()` parses nothing, and annotations are ignored: they
can never turn the feature on.

## Decision (`Policy.Ignorable`)

1. Policy inactive (feature off, or app annotation `collapse-ignorable: "false"`) → no.
2. Group/kind excluded and not lifted by the app's include list → no (regexes not evaluated).
3. No effective regexes → no.
4. No changed lines → no (an empty or headers-only diff does not fold).
5. Every changed line matches at least one regex → yes. Unanchored `MatchString` on the line with one
   leading `+`/`-` stripped; indentation kept.

Classify the raw `DiffStr`, before `truncateLines()` or the "too large" replacement.

## Configuration

- Regexes: newline-separated RE2 (`ARGO_DIFF_COMMENT_COLLAPSE_IGNORABLE_REGEXES`). Unset/empty →
  `DefaultRegexes`. A set value **replaces** the defaults. Blank lines and `#` lines are skipped.
- Exclude kinds: comma/newline-separated (`..._EXCLUDE_KINDS`), default `argoproj.io/*`. Syntax:
  `group/Kind`, `group/*`, `Kind` or `/Kind` (core group). `*` as a group is invalid, and so is a bare
  token containing `.` (the warning suggests `group/*`). Matching is exact on the group and
  case-insensitive on both parts.
- `[]` (trimmed) means an explicit empty list. If every entry of a non-`[]` value is invalid, the list
  is empty; it never falls back to the defaults.
- Limits: 32 entries per list, 1024 bytes per entry. Excess entries warn and are skipped.
- Annotations (on the live Application, `argo-diff.vince-riv.io/`): `collapse-ignorable` (`false` opts
  out), `collapse-ignorable-regexes` (appended to the global list), `collapse-ignorable-include-kinds`
  (lifts global exclusions). Invalid entries are skipped with a warning; `process_event` turns the
  per-app warnings into an app-level notice.

## Gotchas

- The changed-line header rule (`+++ ` / `--- `, trailing space significant) must stay in step with
  `diffStats()` in `internal/comment/markdown.go`. A removed YAML separator renders as `----`.
- `ArgoApp` is the live Application, so an annotation a PR adds only takes effect after the Application
  syncs.
- Go's `regexp` is RE2 (linear time), so an annotation regex cannot cause catastrophic backtracking.
- Config is read on call, not in `init()`, so tests use `t.Setenv`. Global config is parsed once per
  event in `ProcessCodeChange()`.
