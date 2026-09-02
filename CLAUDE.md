# goclikit — Claude Code instructions

A public Go library, not a CLI. There is no `main` package and nothing to
install; it ships as git tags and is consumed by the cobra tools in `~/tools/`
and the CLI modules under `~/webapps/`.

This one is written to be used by strangers. Treat the exported API, the README
and the godoc as the product — a change that is merely convenient for the
internal consumers is not automatically right. `go list -m all` names them.

## Layout

| Path | Holds |
| --- | --- |
| `doc.go` | The package doc: what a consumer's `main` gets |
| `execute.go` | `Execute`, the target resolution, the update suppression list |
| `update.go` | `UpdateCommand` and `ErrReported` |
| `usage.go` | Flag suggestions, the help pointer, the edit distance |
| `notfound.go` | The recovery-hint annotation and the classifier seam |
| `options.go` | `Option` and `WithNotFound` |
| `args.go` | `os.Args` without the program name |

## The constraint every feature is designed against

**A consuming CLI imports this package in exactly one file.** Two lines in
`main`: one `AddCommand`, one `Execute`. Deleting both leaves a CLI that still
compiles and still runs, having lost only what this package added.

That is what makes the dependency optional in practice rather than only on
paper, and it is the property a feature is checked against before it is added:

- A feature that needs per-command data takes it from a **cobra annotation**,
  never from a call this package exports into the command files. The
  annotation key is exported for exactly this reason, and `WithRecoveryHints`
  is sugar over writing it.
- A feature that needs consumer logic takes it as an **`Option` on `Execute`**,
  so a CLI that does not want it passes nothing.
- A feature that would require a wrapper around each command, or an ordering
  the caller has to get right, is the wrong shape. Say so rather than
  documenting the ordering.

**Why**: eleven CLIs depend on this. If dropping it means editing twenty files
in each of them, the dependency has stopped being a choice, and a library
nobody can leave is one nobody can disagree with.

## Constraints that must not regress

- **`Execute` composes with the caller, never replaces it.** Cobra hands back a
  working default `FlagErrorFunc` when nothing is set, so composing is safe
  either way and never silently drops a consumer's own handler.
- **A usage error is never converted into something else.** Exit code 2 is the
  only signal separating "you typed it wrong" from "it ran and failed", and it
  has to survive a consumer's classifier being loose.
- **Errors are marked, never re-prefixed.** `usageError` leaves the message
  alone: a caller printing `error:` would otherwise emit `error: usage error:
  unknown flag: --nope`. The classification travels in the error graph.
- **Every wrapper keeps `Unwrap` intact.** `errors.Is` and `errors.As` must
  still reach whatever cobra, pflag, or the consumer produced.
- **The update check never fires on the completion callback.** It runs on every
  TAB press. `suppressed` walks the whole ancestry rather than the leaf,
  because `completion` owns a subcommand per shell and a leaf-only check misses
  `tool completion zsh` entirely.
- **`go.mod` declares the Go floor and CI tests against it**, reading the
  version from the file rather than repeating it.

## There is no zero-dependency rule here

goselfupdate has one and this package does not. Do not import that constraint
along with the code.

The rule that library holds is about **not linking `x/crypto/openpgp`**, whose
[GO-2026-5932] is the reason it exists at all. That is a deny-one rule and it
applies here too. A deny-all rule does not: this package imports cobra by
definition, and a hand-rolled edit distance sits in `usage.go` only because
cobra keeps its own unexported and pflag has none, not because importing one
would be wrong.

## Testing

`Execute` reads `os.Args` when the root carries no parsed args, which is the
only way to drive both the `Find` and the cobra run from one place. `withArgs`
in `execute_test.go` sets and restores it; every `Execute` test goes through it.

`UpdateCommand` is exercised through `--check`, never through a real update —
the install path resolves the *running* executable, which under test is the
test binary.

**The not-found tests pin behaviour that is invisible from inside.** Three of
them exist because the obvious implementation passes without them:

- `TestTheCommandExecuteResolvesIsTheCommandCobraRuns` is the claim the whole
  design rests on. Resolving before the run and hinting after it is correct
  only if cobra's own `Find` names the command cobra then executes. It runs
  five command-line shapes: plain, alias, and a persistent flag in three
  positions.
- `TestALooseClassifierCannotTurnAUsageErrorIntoANotFound` needs hints **on the
  root**, because an unresolvable line resolves to the root and a root with no
  hints never reaches the branch. Without that the test passes for the wrong
  reason.
- `TestAHandWrittenAnnotationWorksTheSame` is what keeps the exported
  annotation key a contract rather than an implementation detail.

Each gate was proved able to fail before being trusted: starting the ancestry
walk at the root, dropping the `hintNotFound` call from `Execute`, dropping the
empty-subject guard, and moving the hint ahead of the usage branch.

## Releasing

Push to main. The workflow validates on three operating systems and then tags,
so the conventional-commit type is what picks the version. A tag cannot be
retracted once the module proxy caches it, only superseded — which is why the
checks run before the tag exists rather than after it.

`allow-initial-development-versions` holds this on 0.x. Drop that input when
the API is settled enough to promise compatibility.

Then bump the consumers, which are whatever declares the module rather than a
list that goes stale here:

```bash
rg -l datapointchris/goclikit ~/tools/*/go.mod ~/webapps/*/cli/go.mod
```

## Never write the breaking-change trailer in a commit message

Those words — either number, colon or not, subject or body — cut a major
release here, and a major on this repo is an outage rather than a version.
`commit-analyzer-cz` matches them unanchored against the raw message and ORs
the result with the configured major rules, so `.semrelrc` cannot stop it and
it majors even a `fix:` commit.

The module path carries no `/vN` suffix, so once a major exists `go get …@latest`
cannot see it and silently resolves the highest v0 instead. **The ban covers a
commit that merely discusses the trailer.** Name it some other way — "that
marker" — and never quote it.

Deliberate majors use `chore(release-major)`. Full reasoning and the reset
procedure: `standards/release.md` § "Never write the breaking-change trailer in
a Go repo's commit message".

[GO-2026-5932]: https://pkg.go.dev/vuln/GO-2026-5932
