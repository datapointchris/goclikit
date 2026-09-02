// Package goclikit is what a cobra CLI's main calls instead of
// [cobra.Command.Execute].
//
// A bare cobra tree leaves four things to each program, and every program
// that answers them separately answers them differently. This package answers
// them once:
//
//   - Exit codes. Cobra returns a mistyped command line and a command that ran
//     and failed as the same kind of error, so both exit 1. [Execute]
//     classifies the first as [ErrUsage] and the caller selects 2.
//   - Alternatives. Cobra names the token it rejected and stops, without the
//     flags it had just consulted. [Execute] appends the near matches.
//   - The next command. Cobra prints "Run '... --help' for usage" only where
//     the resolved command has not silenced its own error output, which is a
//     field set for unrelated reasons. [Execute] puts it in the error, so
//     every tool prints it exactly once.
//   - Recovery from a missing resource. A tool that cannot list its corpus can
//     still name the command that searches it. [WithNotFoundHints] records
//     those commands and [Execute] attaches them, with the tool supplying a
//     [NotFoundFunc] to say what a not-found looks like.
//
// The update command is the fifth thing, and it is the reason
// [github.com/datapointchris/goselfupdate] is a dependency rather than a
// sibling: [UpdateCommand] returns the subcommand and [Execute] races the
// version check alongside whatever else was typed.
//
// # One import, one line
//
//	func main() {
//		root := newRootCommand()
//		root.AddCommand(goclikit.UpdateCommand(cfg))
//
//		if err := goclikit.Execute(context.Background(), root, autoCfg); err != nil {
//			if !errors.Is(err, goclikit.ErrReported) {
//				fmt.Fprintln(os.Stderr, "error:", err)
//			}
//			if errors.Is(err, goclikit.ErrUsage) {
//				os.Exit(2)
//			}
//			os.Exit(1)
//		}
//	}
//
// Nothing else in a consuming CLI imports this package. That is deliberate and
// it is the constraint every feature here is designed against: a tool that
// wants to answer a mistake its own way deletes two lines, and everything it
// wrote itself still compiles. A feature reaching out to call sites — an
// annotation helper spread across twenty files, a wrapper each command has to
// remember — would trade that away, so [WithNotFoundHints] is offered for
// convenience and never required. A CLI may write the annotation itself.
//
// [cobra]: https://github.com/spf13/cobra
package goclikit
