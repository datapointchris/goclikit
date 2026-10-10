package goclikit

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// NamespaceAnnotation is the [cobra.Command] annotation marking a command as a
// namespace: one that only groups subcommands, so any word left over once
// cobra has matched them is a subcommand that does not exist.
//
// Its presence marks the command and its value is not read. [AsNamespace]
// writes "true". Exported so a CLI can write the annotation itself and keep
// this package out of its command files.
const NamespaceAnnotation = "goclikit.namespace"

// AsNamespace marks cmd as a namespace, and returns cmd so it can be attached
// inline in an AddCommand list.
//
// [Execute] then answers a marked command line three ways:
//
//   - Bare, it prints the namespace's help and succeeds.
//   - With a word naming no subcommand, it refuses the word with
//     [UnknownCommand], marked [ErrUsage].
//   - With that word followed by a flag the namespace does not declare, or by
//     --help, it refuses the word as well. Cobra parses flags and answers
//     --help before it validates arguments, so it would report the flag or
//     print the namespace's help, and never mention the word.
//
// The first two need a run function. AsNamespace gives cmd one where it has
// none of its own, so a tree driven by cobra's own Execute answers them too, as
// a test building the tree directly does. Execute gives one to a namespace
// whose annotation a CLI writes itself. The third needs Execute.
//
// Annotations are not inherited, so each namespace in a tree is marked, the
// root included where it is one.
func AsNamespace(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[NamespaceAnnotation] = "true"
	giveRunFunction(cmd)
	return cmd
}

func isNamespace(cmd *cobra.Command) bool {
	_, marked := cmd.Annotations[NamespaceAnnotation]
	return marked
}

// giveRunFunction installs runNamespace on a namespace with no run function,
// and leaves one that has its own alone.
func giveRunFunction(cmd *cobra.Command) {
	if !cmd.Runnable() {
		cmd.RunE = runNamespace
	}
}

// runNamespace is the run function a namespace with none of its own is given.
//
// Cobra's own answer to a command with no run function is its help screen and
// a nil error, for a bare line and a mistyped one alike. A script then reads a
// typing mistake as success.
func runNamespace(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	return UnknownCommand(cmd, args[0])
}

// refuseWordWithHelp answers a request for help on a namespace for the word
// typed with it, and returns nil where there is no such word.
//
// Cobra answers --help before it validates arguments, so `tool admin uodate
// --help` prints the namespace's help and exits 0. Whoever typed it reads that
// screen as the help for `uodate`, and nothing on it says the word was never
// matched. rest is what cobra's Find left once it had matched every
// subcommand it could.
func refuseWordWithHelp(cmd *cobra.Command, rest []string) error {
	if cmd == nil || !isNamespace(cmd) || !helpRequested(rest) {
		return nil
	}
	if word := firstWord(cmd, rest); word != "" {
		return UnknownCommand(cmd, word)
	}
	return nil
}

// firstWord returns the first argument in args that is neither a flag nor a
// flag's value, read against the flags cmd declares and inherits, or "" where
// there is none before a `--`.
//
// An unknown flag is read as taking no value, which is how pflag would refuse
// it, so the word after it is still found.
func firstWord(cmd *cobra.Command, args []string) string {
	lookup := func(name string) *pflag.Flag {
		if flag := cmd.Flags().Lookup(name); flag != nil {
			return flag
		}
		return cmd.InheritedFlags().Lookup(name)
	}
	shorthand := func(letter string) *pflag.Flag {
		if flag := cmd.Flags().ShorthandLookup(letter); flag != nil {
			return flag
		}
		return cmd.InheritedFlags().ShorthandLookup(letter)
	}
	takesValue := func(flag *pflag.Flag) bool { return flag != nil && flag.NoOptDefVal == "" }

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return ""
		case strings.HasPrefix(arg, "--"):
			name, _, inline := strings.Cut(arg[2:], "=")
			if !inline && takesValue(lookup(name)) {
				i++
			}
		case strings.HasPrefix(arg, "-") && arg != "-":
			// In a cluster the first letter taking a value takes the rest of the
			// cluster, or the next argument where it is the last letter.
			letters := arg[1:]
			for at, letter := range letters {
				if takesValue(shorthand(string(letter))) {
					if at == len(letters)-1 {
						i++
					}
					break
				}
			}
		default:
			return arg
		}
	}
	return ""
}

// refuseWordBeforeFlag answers a flag error on a namespace for the word typed
// ahead of the flag, and returns nil where there is no such word.
//
// pflag keeps the words it parsed before the flag it failed on, so the word is
// still there to name. `tool admin uodate --json` would otherwise read as a
// missing --json, and the --json was never the mistake: no subcommand was
// found to own it.
func refuseWordBeforeFlag(cmd *cobra.Command) error {
	if cmd == nil || !isNamespace(cmd) {
		return nil
	}
	words := cmd.Flags().Args()
	if len(words) == 0 {
		return nil
	}
	return UnknownCommand(cmd, words[0])
}
