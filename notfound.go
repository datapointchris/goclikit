package goclikit

import (
	"strings"

	"github.com/spf13/cobra"
)

// RecoveryHintsAnnotation is the [cobra.Command] annotation carrying the
// commands a not-found under it should name, one per line.
//
// Exported so a CLI can write the annotation itself and keep this package out
// of its command files. [WithRecoveryHints] is the same thing with the join
// done for you.
const RecoveryHintsAnnotation = "goclikit.recovery-hints"

// NotFoundFunc reports whether err is this tool's not-found, and the line
// naming what was missing.
//
// Two answers from one call because they come from the same place. Cobra
// cannot tell a missing resource from a timeout, and the subject is the tool's
// too: only the server knows which of the two ids in `tool items untag 8 4`
// was the one that was absent, so a subject composed here from the arguments
// would name the wrong number half the time.
//
// Returning an empty subject is the same as returning false. A tool whose
// not-found sometimes carries no detail — a proxy error page, an auth redirect
// decoding to a status and nothing else — reports it that way, and the error
// is left alone rather than rewritten into a resource claim nothing
// established.
type NotFoundFunc func(err error) (subject string, ok bool)

// WithRecoveryHints records on cmd the commands a not-found under it should
// name, and returns cmd so it can be attached inline in an AddCommand list.
//
// Each hint is a sentence, a colon, and the command to run.
//
// Cobra does not inherit annotations and the lookup takes the nearest ancestor
// carrying them, so a subcommand acting on a second kind of id names every way
// in: a verb taking both an item and one of its tasks lists both.
func WithRecoveryHints(cmd *cobra.Command, hints ...string) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[RecoveryHintsAnnotation] = strings.Join(hints, "\n")
	return cmd
}

// recoveryHintsFor returns the hints of the nearest command in cmd's ancestry
// carrying any, starting at cmd itself.
//
// The nearest set replaces the outer one rather than adding to it. A nested
// resource sits under its parent and is not the same thing, so accumulating
// would send someone after the wrong noun.
func recoveryHintsFor(cmd *cobra.Command) []string {
	for current := cmd; current != nil; current = current.Parent() {
		if joined := current.Annotations[RecoveryHintsAnnotation]; joined != "" {
			return strings.Split(joined, "\n")
		}
	}
	return nil
}

// notFoundError is a not-found written as the help screen for the failure in
// hand: the subject that was missing, then the commands that find a real one.
type notFoundError struct {
	subject string
	hints   []string
	cause   error
}

func (e *notFoundError) Error() string {
	return e.subject + "\n  " + strings.Join(e.hints, "\n  ")
}

// Unwrap keeps the original reachable, so a caller can still branch on its type
// or a sentinel rather than matching the rendered message.
func (e *notFoundError) Unwrap() error { return e.cause }

// hintNotFound rewrites a not-found leaving cmd into the three things an error
// owes: what failed, what was expected, and the command that changes the
// situation. Anything else is returned untouched.
//
// Called once on the command [Execute] resolved, rather than from a wrapper
// around every RunE in the tree. Cobra has already been asked which command
// the line names, so the walk that a per-RunE wrapper needs is work the
// bootstrap has done. It also reaches a not-found raised before RunE — in a
// PersistentPreRunE resolving a flag, say — which a RunE wrapper cannot see.
func hintNotFound(cmd *cobra.Command, classify NotFoundFunc, err error) error {
	if err == nil || cmd == nil || classify == nil {
		return err
	}
	subject, ok := classify(err)
	if !ok || subject == "" {
		return err
	}
	hints := recoveryHintsFor(cmd)
	if len(hints) == 0 {
		return err
	}
	return &notFoundError{subject: subject, hints: hints, cause: err}
}
