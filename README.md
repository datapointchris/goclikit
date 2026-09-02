# goclikit

[![Go Reference](https://pkg.go.dev/badge/github.com/datapointchris/goclikit.svg)](https://pkg.go.dev/github.com/datapointchris/goclikit)
[![CI](https://github.com/datapointchris/goclikit/actions/workflows/validate.yml/badge.svg)](https://github.com/datapointchris/goclikit/actions/workflows/validate.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/datapointchris/goclikit)](https://goreportcard.com/report/github.com/datapointchris/goclikit)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

What a [cobra] CLI's `main` calls instead of `rootCmd.Execute()`.

```go
func main() {
    root := newRootCommand()
    root.AddCommand(goclikit.UpdateCommand(cfg))

    if err := goclikit.Execute(context.Background(), root, autoCfg); err != nil {
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

## Install

```sh
go get github.com/datapointchris/goclikit
```

## What it answers

A bare cobra tree leaves four things to each program. Every program that
answers them separately answers them differently.

### Exit codes

Cobra returns a mistyped command line and a command that ran and failed as the
same kind of error, so both exit 1. A script cannot tell "you typed it wrong,
retry with different arguments" from "it ran and failed".

`Execute` classifies the first as `ErrUsage`, and the caller selects exit 2 —
the shell convention, and what Python's argparse does.

Cobra cannot classify everything. An argument-count failure and a custom
`Args` validator come back indistinguishable from a `RunE` failure without
matching on message text, which a caller must never have to do. Mark those at
the source:

```go
Args: func(cmd *cobra.Command, args []string) error {
    if len(args) > 1 {
        return goclikit.UsageError(fmt.Errorf("unknown command %q", args[0]))
    }
    return nil
},
```

### The alternatives, and the next command

Cobra names the token it rejected and stops. The flag set it had just consulted
goes unnamed, and the `--help` pointer is printed only where the resolved
command has not silenced its own error output — a field set for unrelated
reasons, which is why two CLIs sharing one bootstrap answer the same mistake
differently.

```text
$ tool search --ownd
error: unknown flag: --ownd

Did you mean this?
  --owned

Run 'tool search --help' for usage.
```

Suggestions come from cobra's own rule for commands: within
`SuggestionsMinimumDistance` edits of a real flag, or a prefix of one, and off
entirely under `DisableSuggestions`. One rule then answers a mistyped flag and
a mistyped command on the same line. Near matches rather than the whole flag
set, so a wide flag surface does not answer one typo with a wall.

The prose is a suffix on the error, so `errors.Is` and `errors.As` still reach
whatever cobra, pflag or a caller's own `FlagErrorFunc` produced.

### Recovery from a missing resource

An error is read by someone who is stuck right now, so it owes what failed,
what was expected, and the one command that changes the situation. A
not-found usually gives only the first.

A tool small enough to list its corpus should list it — naming a `list` command
is a worse error than printing the valid values. This is for the tools that
cannot: a store with more ids than fit on a screen, where the answer is the
command that searches it.

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

The classifier is the only part a tool has to supply, because it is the only
part cobra cannot know. A tool backed by a local store branches on its own
sentinel instead of a status code; the mechanism is the same.

Hints are read from the nearest annotated ancestor, and do not accumulate up
the tree. A nested resource is not its parent, so adding the outer group's
hints would send someone after the wrong noun.

### The update check

`UpdateCommand` returns the `update` subcommand, and `Execute` races the
version check alongside whatever else was typed — the check starts before the
command and the notice prints after it, so a fast command pays nothing and the
line is not buried in the output. This is `gh`'s shape, and it is why there is
no blocking mode.

The check never fires for `update`, `version`, `completion`, `help`, or any
line carrying `--help`. Cobra's shell-completion callback runs on every TAB
press, so a check there would add latency to something that must feel instant.

The machinery underneath is [goselfupdate], which is where the release fetch,
the checksum verification, the atomic binary replacement and the interval gate
live.

## One import, one line

Nothing else in a consuming CLI imports this package. That is the constraint
every feature here is designed against.

A tool that wants to answer a mistake its own way deletes two lines from
`main`, and everything it wrote itself still compiles. A feature that reached
out to call sites — a wrapper each command had to remember, a helper spread
across twenty files — would trade that away.

`WithRecoveryHints` is therefore a convenience and never a requirement. The
contract is the annotation, and its key is exported, so a CLI can write it
directly and keep this package out of its command files:

```go
cmd.Annotations = map[string]string{
    goclikit.RecoveryHintsAnnotation: strings.Join(hints, "\n"),
}
```

## Design decisions

**The not-found is classified by the consumer, not detected here.** Cobra
cannot tell a missing resource from a timeout, and the subject is the tool's
too — only the server knows which of the two ids in `tool items untag 8 4` was
absent, so a subject composed here from the arguments would name the wrong
number half the time.

**A not-found carrying no subject is left alone.** A proxy error page or an
auth redirect decodes to a status and nothing else, and the resource was never
reached. Rewriting that into a resource claim would name a thing the tool does
not have, and a wrong base URL and a missing id have different remedies.

**The hint attaches once, to the command `Execute` already resolved.** Cobra
has been asked which command the line names before anything runs, so wrapping
every `RunE` in the tree would be redoing work already done. It also reaches a
not-found raised before `RunE` — in a `PersistentPreRunE` resolving a flag —
which a `RunE` wrapper cannot see.

**A usage error is never converted to a not-found.** A line cobra rejected
never reached a resource. Exit code 2 survives a consumer's classifier saying
otherwise.

**`Execute` takes options rather than more parameters.** A CLI wanting none of
this calls it with three arguments, and a feature added later does not reach
the call sites that never asked for it.

**Options are composed with what the caller already set, never replacing it.**
A consumer's own `FlagErrorFunc` still runs, and its error type still survives
`errors.As`.

## License

MIT

[cobra]: https://github.com/spf13/cobra
[goselfupdate]: https://github.com/datapointchris/goselfupdate
