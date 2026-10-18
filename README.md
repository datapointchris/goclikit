# goclikit

What a [cobra] CLI's `main` calls instead of `rootCmd.Execute()`. It answers
what a bare cobra tree leaves to each program: the exit code for a typing
mistake, the flags near a mistyped one, the `--help` pointer, the command that
recovers from a not-found, and the `update` command. A program that answers
these itself answers them differently from the next one.

Cobra still parses and runs the tree. goclikit asks cobra which command a line
names, then shapes the error that comes back. Updating the binary belongs to
[goselfupdate]. The release lookup, the checksum check and the binary swap live
there, and goclikit wires them into a cobra tree.

## Install

```sh
go get github.com/datapointchris/goclikit
```

It needs the Go version its `go.mod` declares, or newer. It brings cobra and
goselfupdate with it, and a program's `main` imports both.

## Starting from scratch

```bash
mkdir tool && cd tool                      # an empty directory to build in
go mod init example.com/tool               # a module for the CLI
go get github.com/datapointchris/goclikit  # cobra and goselfupdate arrive with it
$EDITOR main.go                            # the program below
go mod tidy                                # records main.go's direct imports
go build -o tool .                         # a binary, so the shell sees its exit code
./tool search --ownd; echo "exit $?"       # the near flag, the pointer, and exit 2
```

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "os"

    "github.com/datapointchris/goclikit"
    "github.com/datapointchris/goselfupdate"
    "github.com/datapointchris/goselfupdate/autoupdate"
    "github.com/spf13/cobra"
)

// A release build sets this: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
    root := &cobra.Command{Use: "tool", SilenceErrors: true, SilenceUsage: true}

    search := &cobra.Command{
        Use:  "search",
        RunE: func(*cobra.Command, []string) error { return nil },
    }
    search.Flags().Bool("owned", false, "only what you own")
    root.AddCommand(search)

    release := goselfupdate.Config{Owner: "you", Repo: "tool", Binary: "tool", Version: version}
    root.AddCommand(goclikit.UpdateCommand(release))

    err := goclikit.Execute(context.Background(), root, autoupdate.Config{Update: release})
    if err != nil {
        if !errors.Is(err, goclikit.ErrReported) {
            fmt.Fprintln(os.Stderr, "error:", err)
        }
        if errors.Is(err, goclikit.ErrUsage) {
            os.Exit(2)
        }
        os.Exit(1)
    }
}
```

The last command prints this:

```text
error: unknown flag: --ownd

Did you mean this?
  --owned

Run 'tool search --help' for usage.
exit 2
```

The root silences cobra's own error and usage printing because `main` prints
the error. Without that, cobra prints the mistake and its usage block, and
`main` prints the mistake again.

Until a release build sets `version` to a plain `vX.Y.Z`, `Execute` never
checks for an update. The first run reaches no network and writes no state, and
`tool update` refuses a development build.

The binary is built rather than run with `go run`, because `go run` reports any
failure of the program as its own exit 1.

## Which surface answers which question

| Ask | Surface | Reads |
| --- | --- | --- |
| Did they type it wrong, or did it run and fail | `Execute`, `ErrUsage`, `UsageError` | cobra's verdict on the line, and a marker a command's own check returns |
| Which flag did they mean | `Execute` | the resolved command's flags, and the suggestion fields up its ancestry |
| Why a mistyped subcommand prints help and exits 0 | `AsNamespace`, `UnknownCommand` | the namespace annotation |
| What a not-found should tell someone to run | `WithRecoveryHints`, `WithNotFound` | the recovery-hints annotation, and the tool's classifier |
| How the tool updates itself, and says when it is behind | `UpdateCommand`, `Execute` | goselfupdate's config, and the repository's releases |

## `ErrUsage` — a typing mistake exits 2, and a command that ran and failed exits 1

Cobra returns a mistyped line and a failed command as the same kind of error,
so both exit 1. A script cannot tell "retry with different arguments" from "it
ran and failed". `Execute` marks the first as `ErrUsage`: an unknown or
malformed flag, a word naming no command, and positional arguments the
command's `Args` validator refused. The caller selects exit 2, which is the
shell convention and what Python's argparse does.

The mark travels in the error graph and the message is left alone. A prefix
would make a caller printing `error:` emit
`error: usage error: unknown flag: --nope`.

An argument refusal is told apart by where it came from. `Execute` wraps the
validator of the command it is about to run, so classifying it reads none of
cobra's text. That wrapper also says what cobra leaves out:

```text
$ tool answer only-a-key
accepts 2 arg(s), received 1

Usage: tool answer <key> <n> [flags]
Run 'tool answer --help' for usage.
```

A command given none of the arguments it needs prints its help on stderr
instead, and returns an error marked `ErrReported` as well, so `main` prints
nothing more. A validator that marks its own refusal with `UsageError` keeps its
wording.

Cobra's `NoArgs` calls a word on a command with no subcommands an unknown
command, as though the command had subcommands to choose from. `Execute` words
that refusal as `tool status takes no arguments, and was given "extra"`, so a
CLI needs no validator of its own to say it.

`Execute` cannot see a required-flag or mutually-exclusive rule a command checks
in its `RunE`. Mark that one at the source:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if all && len(args) > 0 {
        return goclikit.UsageError(errors.New("--all takes no names"))
    }
    ...
},
```

## Flag suggestions — a mistyped flag gets the near ones and the command that lists them all

Cobra names the flag it rejected and stops. The flag set it had just consulted
goes unnamed. `Execute` composes a `FlagErrorFunc` that adds the near flags and
the `--help` pointer, as the first run above shows.

Suggestions follow cobra's own rule for commands. A flag is offered within
`SuggestionsMinimumDistance` edits of what was typed, or where the typed text is
a prefix of it. `DisableSuggestions` turns them off. Both fields are read up the
ancestry, so setting them on the root reaches every command, as it does for
cobra. One rule then answers a mistyped flag and a mistyped command on the same
line.

Only near matches are offered, so a wide flag surface does not answer one typo
with a wall. Hidden and deprecated flags are never offered. A mistyped shorthand
gets no guesses, because nearly every long flag sits within two edits of one
character. The pointer still names the command that lists them all.

Cobra prints the pointer for an unknown command only where the resolved command
has not silenced its own errors. That field is set for unrelated reasons, which
is why two CLIs sharing one bootstrap answer the same mistake differently.
`Execute` puts the pointer in the error on a silenced command, so every tool
prints it exactly once.

A caller's own `FlagErrorFunc` still runs first, and its error type survives
`errors.As`. The added prose is a suffix, so `errors.Is` and `errors.As` still
reach whatever cobra, pflag or the caller produced.

## `AsNamespace` — a mistyped subcommand under a group is refused, not answered with help

Cobra suggests an unknown command only from its root validator, which runs
where the root declares no `Args`. Below the root, a command that only groups
subcommands answers a mistyped one with its help screen and exit 0. A script
reads the typing mistake as success. Mark each such namespace, and `Execute`
refuses the word:

```go
root.AddCommand(goclikit.AsNamespace(newAdminCommand()))
```

```text
$ tool admin uodate
error: unknown command "uodate" for "tool admin"

Did you mean this?
  update

Run 'tool admin --help' for usage.
```

A bare namespace still shows its help and exits 0. A word followed by a flag the
namespace does not declare is refused the same way. Cobra parses flags before it
validates arguments, so `tool admin uodate --json` would otherwise answer
`unknown flag: --json` and never mention the word. A caller's own
`FlagErrorFunc` does not see that line, because it reports no flag error.

A word typed with `--help` is refused too. Cobra answers `--help` before it
validates arguments, so `tool admin uodate --help` would print the help for
`tool admin` and exit 0. The reader would take it for help on `uodate`.

A namespace is declared, never inferred. Having subcommands does not make a
command a namespace, since a group may take arguments of its own, and refusing
its first argument would break it. Annotations are not inherited, so each
namespace is marked, the root included where it is one.

`AsNamespace` gives the namespace a run function where it has none. A
consumer's tests drive the tree through cobra's own `Execute`, and there a bare
namespace and an unknown word are answered exactly as in the shipped binary.
Only a word typed with an unknown flag or with `--help` needs
`goclikit.Execute`. For an annotation written by hand, `Execute` installs the
run function on the command it resolved, which is the one cobra runs.

A namespace keeps a run function of its own. `UnknownCommand` is the refusal it
returns, carrying the near subcommands by the same rule, and the pointer:

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if len(args) == 0 {
        return printSummary(cmd)
    }
    return goclikit.UnknownCommand(cmd, args[0])
},
```

## Recovery hints — a not-found names the command that finds a real id

An error is read by someone who is stuck right now. It owes what failed, what
was expected, and the one command that changes the situation. A not-found
usually gives only the first.

A tool small enough to list its corpus should list it, since naming a `list`
command is a worse error than printing the valid values. Recovery hints are for
the tools that cannot: a store with more ids than fit on a screen, where the
answer is the command that searches it.

Annotate the resource commands, and tell `Execute` what a not-found looks like:

```go
items := goclikit.WithRecoveryHints(newItemsCommand(),
    "Search items by title: tool items search <query>",
    "Completed items are hidden: tool items list --status all",
)

goclikit.Execute(ctx, root, autoCfg, goclikit.WithNotFound(notFound))
```

```go
func notFound(err error) (subject string, ok bool) {
    var apiErr *api.APIError
    if !errors.As(err, &apiErr) || !apiErr.NotFound() {
        return "", false
    }
    return apiErr.Message, true
}
```

```text
$ tool items show 999999
error: item 999999 not found
  Search items by title: tool items search <query>
  Completed items are hidden: tool items list --status all
```

The classifier is the only part a tool supplies, because it is the only part
cobra cannot know. Cobra cannot tell a missing resource from a timeout. The
subject is the tool's too: only the server knows which of the two ids in
`tool items untag 8 4` was absent, so a subject built here from the arguments
would name the wrong number half the time. A tool backed by a local store
branches on its own sentinel instead of a status code, and the mechanism is the
same. Without `WithNotFound`, a not-found passes through untouched.

Hints come from the nearest annotated ancestor and do not accumulate up the
tree. A nested resource is not its parent, so the outer group's hints would send
someone after the wrong noun. A verb acting on a second kind of id lists the
hints for both.

A not-found carrying no subject is left alone. A proxy error page or an auth
redirect decodes to a status and nothing else, and no resource was reached.
Rewriting it into a resource claim would name a thing the tool does not have,
and a wrong base URL and a missing id have different remedies.

The hint attaches once, to the command `Execute` resolved before the run.
Wrapping every `RunE` in the tree would redo that work. Attaching it there also
reaches a not-found raised before `RunE`, such as in a `PersistentPreRunE`
resolving a flag, which a `RunE` wrapper cannot see.

A line cobra cannot resolve returns as a usage error before the classifier is
asked. Any other usage error a classifier claims keeps `ErrUsage` reachable
through the wrap, so exit 2 survives a loose classifier.

## `UpdateCommand` — the version check runs beside the command, and never on a TAB press

```bash
tool update          # install the latest release, then list the commits it brings
tool update --check  # say whether a newer release exists, installing nothing
```

`UpdateCommand` returns the `update` subcommand for a `goselfupdate.Config`. It
prints its own result, `✓ tool updated: <from> → <to>`, and the commit subjects
in between. A failure prints as `✗ tool update failed: <reason>` and comes back
wrapped in `ErrReported`, which is why `main` skips printing anything matching
it. `Options` renames the command or skips the changelog.

The update needs releases in the named GitHub repository carrying per-platform
archives and a checksums file, which goreleaser's defaults produce. The build
needs its version injected at link time.

`Execute` races the version check against whatever else was typed. The check
starts before the command and the notice prints after it, so the line is not
buried in the command's output. This is `gh`'s shape, and it is why there is no
blocking mode.

Most invocations never check. goselfupdate's `autoupdate` checks at most once
per interval, and skips a development build, output that is not a terminal, CI,
and a set `NO_AUTO_UPDATE`. A command that outlasts the check pays nothing for
it. One that finishes first waits for the check, bounded by
`autoupdate.Config.Timeout`.

`Execute` also never checks on a command named `update`, `version`,
`completion` or `help`, on the command `UpdateCommand` built under whatever
name `Options` gave it, on anything nested under one, on cobra's
shell-completion callback, on a line carrying `--help` or `-h`, or on a line
cobra cannot resolve. The completion callback runs on every TAB press, so a
check there would add latency to something that must feel instant. The whole
ancestry is matched because `completion` owns a subcommand per shell, and
`tool completion zsh` has the leaf name `zsh`.

## One import, in `main` alone — dropping the package is an edit to one file

Nothing outside `main` needs this package. That is the constraint every feature
here is designed against. `main` names it in one `AddCommand`, one `Execute`,
and the two sentinels its exit code reads. A tool that wants to answer a mistake
its own way edits `main`, and everything it wrote itself still compiles. A
feature reaching out to call sites, such as a wrapper each command must remember
or a helper spread across twenty files, would trade that away.

A feature needing per-command data reads a cobra annotation. A feature needing
the tool's own logic takes an `Option` on `Execute`. A CLI that wants none of it
calls `Execute` with three arguments, and a feature added later never reaches a
call site that did not ask for it.

`AsNamespace` and `WithRecoveryHints` are conveniences, never requirements. The
contract is the annotation, so a command file can write the key itself and
never import this package:

```go
admin.Annotations = map[string]string{"goclikit.namespace": ""}
items.Annotations = map[string]string{"goclikit.recovery-hints": strings.Join(hints, "\n")}
```

The exported `NamespaceAnnotation` and `RecoveryHintsAnnotation` hold these
keys. The namespace annotation's value is not read, and its presence marks the
command.

The error constructors, `UsageError` and `UnknownCommand`, are the exception.
Each is a value a command returns rather than a wrapper it must remember, and
each carries a fact only the command has: that its own check refused the line,
and that the word it refused names no subcommand. `Execute` cannot infer either
from an error's presence, because a flag mistake or an argument-count failure on
the same line would read as an unknown word. Dropping the package means
replacing them with plain errors, which is a mechanical edit.

## License

MIT

[cobra]: https://github.com/spf13/cobra
[goselfupdate]: https://github.com/datapointchris/goselfupdate
