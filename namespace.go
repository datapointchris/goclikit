package goclikit

import "github.com/spf13/cobra"

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
//   - With that word followed by a flag the namespace does not declare, it
//     refuses the word as well. Cobra parses flags before it validates
//     arguments, so it would report the flag and never mention the word.
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
