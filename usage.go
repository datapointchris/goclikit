package goclikit

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// explained carries a message appended to err's own, leaving the error graph
// underneath it intact.
//
// The suffix is prose for whoever reads the failure. Nothing branches on it, so
// a caller matching with [errors.Is] or [errors.As] still finds whatever cobra,
// pflag or its own FlagErrorFunc produced.
type explained struct {
	err   error
	extra string
}

// Error trims the message before appending, because the suffix opens with its
// own blank line and cobra's suggestion block closes with a newline — joined
// raw they leave a two-line gap in the middle of one error.
func (e explained) Error() string {
	return strings.TrimRight(e.err.Error(), "\n") + e.extra
}

func (e explained) Unwrap() error { return e.err }

// helpPointer names the one command that changes the situation.
//
// Worded exactly as cobra words it, so a tool reads the same whether the line
// came from here or from cobra printing it itself.
func helpPointer(cmd *cobra.Command) string {
	return fmt.Sprintf("Run '%v --help' for usage.", cmd.CommandPath())
}

// suggestionBlock formats names the way cobra formats its own command
// suggestions, so both halves of a mistyped command line look alike.
func suggestionBlock(names []string) string {
	var block strings.Builder
	block.WriteString("Did you mean this?")
	for _, name := range names {
		_, _ = fmt.Fprintf(&block, "\n\t%s", name)
	}
	return block.String()
}

// explainFlagError adds the near-matching flags and the help pointer to a flag
// parse failure.
//
// Cobra names the token it rejected and stops, which states what the parser
// concluded rather than what was expected. The flag set it just consulted and
// the command that prints that set are both what someone who mistyped goes
// looking for next.
func explainFlagError(cmd *cobra.Command, err error) error {
	if err == nil || cmd == nil {
		return err
	}

	sections := make([]string, 0, 2)
	if names := flagSuggestions(cmd, err); len(names) > 0 {
		sections = append(sections, suggestionBlock(names))
	}
	sections = append(sections, helpPointer(cmd))

	return explained{err: err, extra: "\n\n" + strings.Join(sections, "\n\n")}
}

// explainCommandError adds the help pointer to a command line naming nothing
// cobra can run.
//
// Cobra prints this pointer itself, and only when the resolved command has not
// silenced its own error output — so whether a tool shows it is decided by a
// field its author set for an unrelated reason, which is why two CLIs sharing
// this bootstrap answer the same mistake differently. Putting it in the error
// is what makes every tool show it exactly once: silenced, and it travels with
// the error to whoever prints that; not silenced, and cobra has already
// printed it.
func explainCommandError(cmd *cobra.Command, err error) error {
	if err == nil || cmd == nil || !cmd.SilenceErrors {
		return err
	}
	return explained{err: err, extra: "\n\n" + helpPointer(cmd)}
}

// UnknownCommand is the error for a word that names none of cmd's subcommands:
// the word, the subcommands near it, and the command that lists every one,
// marked [ErrUsage].
//
// Cobra's unknown-command suggestion comes only from its root validator, which
// runs where the root declares no Args. A namespace that validates its own
// arguments, so that bare shows help and an unknown word exits 2, has to say
// the word is unknown itself. This is that sentence with the alternatives and
// the pointer in it, worded the way cobra words both.
//
//	RunE: func(cmd *cobra.Command, args []string) error {
//		if len(args) == 0 {
//			return cmd.Help()
//		}
//		return goclikit.UnknownCommand(cmd, args[0])
//	},
//
// The alternatives are in the error rather than added by [Execute] afterwards,
// because only the namespace knows the word was refused for naming nothing.
// Read off the error's presence instead, a flag mistake or an argument-count
// failure on the same line would be answered as a command it never was.
func UnknownCommand(cmd *cobra.Command, word string) error {
	refused := fmt.Errorf("unknown command %q for %q", word, cmd.CommandPath())
	sections := make([]string, 0, 2)
	if names := commandSuggestions(cmd, word); len(names) > 0 {
		sections = append(sections, suggestionBlock(names))
	}
	sections = append(sections, helpPointer(cmd))
	return usageError{explained{err: refused, extra: "\n\n" + strings.Join(sections, "\n\n")}}
}

// commandSuggestions names the subcommands of cmd near typed, and any whose
// SuggestFor lists it, which is cobra's rule for commands.
//
// Not [cobra.Command.SuggestionsFor], which reads the distance off cmd itself.
// A consumer sets it on the root, where cobra reads it, so on a namespace it is
// zero and would offer only prefixes.
func commandSuggestions(cmd *cobra.Command, typed string) []string {
	distance, on := suggestionDistance(cmd)
	if !on {
		return nil
	}
	var names []string
	for _, sub := range cmd.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		named := slices.ContainsFunc(sub.SuggestFor, func(word string) bool { return strings.EqualFold(word, typed) })
		if near(typed, sub.Name(), distance) || named {
			names = append(names, sub.Name())
		}
	}
	return names
}

// suggestionDistance is how many edits a typed word may sit from a name and
// still be offered, and false where suggestions are off.
//
// Read up the ancestry, because cobra reads both fields on the root and a
// consumer sets them there. Suggestions are off when any command from cmd to
// the root turns them off, and the distance is the nearest one set, or cobra's
// default of two.
func suggestionDistance(cmd *cobra.Command) (int, bool) {
	distance := 0
	for current := cmd; current != nil; current = current.Parent() {
		if current.DisableSuggestions {
			return 0, false
		}
		if distance == 0 && current.SuggestionsMinimumDistance > 0 {
			distance = current.SuggestionsMinimumDistance
		}
	}
	if distance == 0 {
		distance = 2
	}
	return distance, true
}

// near reports whether typed is close enough to name to offer it: within
// distance edits, or a prefix of it. It is the one rule both halves of a
// mistyped command line are answered by here.
func near(typed, name string, distance int) bool {
	return editDistance(typed, name) <= distance || strings.HasPrefix(strings.ToLower(name), strings.ToLower(typed))
}

// flagSuggestions names the flags on cmd close enough to what was typed to be
// worth offering.
//
// The rule is cobra's own for commands, applied to flags, so a tool answers a
// mistyped command and a mistyped flag on one command line by one standard. It
// honors the DisableSuggestions and SuggestionsMinimumDistance a consumer set,
// wherever in the ancestry it set them.
//
// Near matches rather than the whole flag set, which is also what Click does:
// measured 2026-08-11, `indy search --nope` named one of its seven flags. A
// command with a wide flag surface would otherwise answer a typo with a wall,
// and the pointer above already names the command that prints all of them.
func flagSuggestions(cmd *cobra.Command, err error) []string {
	distance, on := suggestionDistance(cmd)
	if !on {
		return nil
	}

	var missing *pflag.NotExistError
	if !errors.As(err, &missing) {
		return nil
	}

	// A shorthand is one character, and nearly every long flag sits within two
	// edits of one character, so suggesting from it would name the flag set.
	if missing.GetSpecifiedShortnames() != "" {
		return nil
	}

	typed := missing.GetSpecifiedName()
	if typed == "" {
		return nil
	}

	var names []string
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Deprecated != "" {
			return
		}
		if near(typed, flag.Name, distance) {
			names = append(names, "--"+flag.Name)
		}
	})
	return names
}

// editDistance is the case-insensitive Levenshtein distance between a and b.
//
// Hand-rolled because cobra keeps its own copy unexported and pflag has none.
// Two rows rather than the full matrix: the flag set is walked once per
// failure, and only the previous row is ever read.
func editDistance(a, b string) int {
	first := []rune(strings.ToLower(a))
	second := []rune(strings.ToLower(b))

	previous := make([]int, len(second)+1)
	current := make([]int, len(second)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(first); i++ {
		current[0] = i
		for j := 1; j <= len(second); j++ {
			substitution := previous[j-1]
			if first[i-1] != second[j-1] {
				substitution++
			}
			current[j] = min(substitution, min(previous[j]+1, current[j-1]+1))
		}
		previous, current = current, previous
	}

	return previous[len(second)]
}
