package goclikit

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// The defect the annotation exists for. Cobra parses a namespace's flags
// before it validates the namespace's arguments, so an unknown word followed
// by a flag only a leaf declares reports the flag and never the word.
func TestAWordBeforeAnUnknownFlagIsRefusedOnANamespace(t *testing.T) {
	for _, c := range []struct {
		why       string
		args      []string
		refused   string
		namespace string
		named     string
	}{
		{why: "in a namespace", args: []string{"admin", "uodate", "--json"}, refused: "uodate", namespace: "demo admin", named: "update"},
		{why: "at the root", args: []string{"sarch", "--json"}, refused: "sarch", namespace: "demo", named: "search"},
		{
			why: "past a persistent flag", args: []string{"admin", "--verbose", "uodate", "--json"},
			refused: "uodate", namespace: "demo admin", named: "update",
		},
	} {
		root := namespacedRoot()
		root.PersistentFlags().Bool("verbose", false, "say more")
		withArgs(t, c.args...)

		err := execute(t, root)
		if !errors.Is(err, ErrUsage) {
			t.Fatalf("%s: error is not ErrUsage: %v", c.why, err)
		}
		message := err.Error()
		if want := "unknown command \"" + c.refused + "\" for \"" + c.namespace + "\""; !strings.HasPrefix(message, want) {
			t.Errorf("%s: message does not open refusing the word:\n%s", c.why, message)
		}
		if strings.Contains(message, "--json") {
			t.Errorf("%s: the flag was reported rather than the word:\n%s", c.why, message)
		}
		if !strings.Contains(message, "\t"+c.named) {
			t.Errorf("%s: the near subcommand %q was not named:\n%s", c.why, c.named, message)
		}
	}
}

// With no word ahead of it, the flag is the mistake, and it is answered as one.
func TestAnUnknownFlagOnANamespaceWithNoWordIsAFlagError(t *testing.T) {
	message := messageOf(t, namespacedRoot(), "admin", "--json")

	if !strings.HasPrefix(message, "unknown flag: --json") {
		t.Errorf("the flag was not reported:\n%s", message)
	}
	if !strings.HasSuffix(message, "Run 'demo admin --help' for usage.") {
		t.Errorf("message does not name the namespace's help:\n%s", message)
	}
}

// The annotation is what declares that every leftover word is a subcommand. A
// group with subcommands may also take arguments of its own, so having
// subcommands is not enough to refuse a word on.
func TestAnUnmarkedGroupStillReportsTheFlag(t *testing.T) {
	root := searchRoot()
	group := &cobra.Command{Use: "group", Args: cobra.ArbitraryArgs, Run: func(*cobra.Command, []string) {}}
	group.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)

	message := messageOf(t, root, "group", "lst", "--json")
	if !strings.HasPrefix(message, "unknown flag: --json") {
		t.Errorf("an unmarked group refused its argument:\n%s", message)
	}
}

// handMarkedRoot is a tree whose one namespace carries the annotation a CLI
// writes itself, with an empty value and no run function.
func handMarkedRoot() *cobra.Command {
	root := &cobra.Command{Use: "demo", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	group := &cobra.Command{Use: "hand", Annotations: map[string]string{NamespaceAnnotation: ""}}
	group.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)
	return root
}

// Cobra answers a command with no run function by printing its help and
// returning nil, whatever was typed after it. Execute supplies one to a
// namespace AsNamespace never saw.
func TestAHandMarkedNamespaceWithNoRunFunctionRefusesAWord(t *testing.T) {
	withArgs(t, "hand", "lst")

	err := execute(t, handMarkedRoot())
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("error is not ErrUsage: %v", err)
	}
	if want := `unknown command "lst" for "demo hand"`; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("message does not open %q:\n%s", want, err)
	}
}

// A consumer's tests drive the tree it builds through cobra's own Execute, and
// a marked namespace refuses a word there as well.
func TestAsNamespaceRefusesAWordUnderCobrasOwnExecute(t *testing.T) {
	root := namespacedRoot()
	root.SetArgs([]string{"admin", "uodate"})

	err := root.Execute()
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("error is not ErrUsage: %v", err)
	}
	if want := `unknown command "uodate" for "demo admin"`; !strings.HasPrefix(err.Error(), want) {
		t.Errorf("message does not open %q:\n%s", want, err)
	}
}

func TestABareNamespaceShowsItsHelp(t *testing.T) {
	withArgs(t, "admin")
	root := namespacedRoot()
	var out bytes.Buffer
	root.SetOut(&out)

	if err := execute(t, root); err != nil {
		t.Fatalf("a bare namespace failed: %v", err)
	}
	if !strings.Contains(out.String(), "Available Commands:") {
		t.Errorf("a bare namespace did not print its help:\n%s", out.String())
	}
}

// A namespace that does something of its own when bare keeps doing it.
func TestANamespacesOwnRunFunctionIsKept(t *testing.T) {
	withArgs(t, "admin", "anything")
	theirs := errors.New("the namespace's own answer")

	root := namespacedRoot()
	admin := find(t, root, "admin")
	admin.RunE = func(*cobra.Command, []string) error { return theirs }

	if err := execute(t, root); !errors.Is(err, theirs) {
		t.Errorf("the namespace's own run function was replaced: %v", err)
	}
}

// The exported key is the contract, and its value is not read, so a CLI
// writing the annotation by hand gets the same answer as one calling
// AsNamespace.
func TestAHandWrittenNamespaceAnnotationWorksTheSame(t *testing.T) {
	withArgs(t, "hand", "lst", "--json")

	err := execute(t, handMarkedRoot())
	if want := `unknown command "lst" for "demo hand"`; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("got:\n%v\n\nwant it to open:\n%s", err, want)
	}
}

// flaggedNamespacedRoot is namespacedRoot with a persistent string flag and a
// persistent bool, so a word can sit behind a flag's value, inherited by the
// namespace rather than declared on it.
func flaggedNamespacedRoot() *cobra.Command {
	root := namespacedRoot()
	root.PersistentFlags().StringP("format", "o", "", "output format")
	root.PersistentFlags().BoolP("verbose", "v", false, "say more")
	return root
}

func TestFirstWordSkipsEveryFlagAndItsValue(t *testing.T) {
	root := flaggedNamespacedRoot()
	admin := find(t, root, "admin")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"uodate"}, "uodate"},
		{[]string{"--json", "uodate"}, "uodate"},
		{[]string{"--format", "x", "uodate"}, "uodate"},
		{[]string{"--format=x", "uodate"}, "uodate"},
		{[]string{"--verbose", "uodate"}, "uodate"},
		{[]string{"-o", "x", "uodate"}, "uodate"},
		{[]string{"-vo", "x", "uodate"}, "uodate"},
		{[]string{"-ox", "uodate"}, "uodate"},
		{[]string{"--", "uodate"}, ""},
		{[]string{"--help"}, ""},
	} {
		if got := firstWord(admin, c.args); got != c.want {
			t.Errorf("firstWord(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}

// Cobra answers --help before it validates arguments, so the namespace's help
// would print for a word it never matched, and exit 0.
func TestAWordWithHelpIsRefusedOnANamespace(t *testing.T) {
	for _, c := range []struct {
		args      []string
		refused   string
		namespace string
	}{
		{[]string{"admin", "uodate", "--help"}, "uodate", "demo admin"},
		{[]string{"admin", "--help", "uodate"}, "uodate", "demo admin"},
		{[]string{"admin", "-o", "x", "uodate", "-h"}, "uodate", "demo admin"},
		{[]string{"sarch", "--help"}, "sarch", "demo"},
	} {
		withArgs(t, c.args...)
		err := execute(t, flaggedNamespacedRoot())
		if !errors.Is(err, ErrUsage) {
			t.Fatalf("%v: error is not ErrUsage: %v", c.args, err)
		}
		if want := "unknown command \"" + c.refused + "\" for \"" + c.namespace + "\""; !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%v: message does not open %q:\n%s", c.args, want, err)
		}
	}
}

// Help on the namespace itself, on a subcommand it matched, or on a group that
// is not marked, is still the help that was asked for.
func TestHelpWithNoUnmatchedWordStillPrints(t *testing.T) {
	unmarked := func() *cobra.Command {
		root := searchRoot()
		group := &cobra.Command{Use: "group", Args: cobra.ArbitraryArgs, Run: func(*cobra.Command, []string) {}}
		group.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
		root.AddCommand(group)
		return root
	}
	for _, c := range []struct {
		why  string
		args []string
		root func() *cobra.Command
	}{
		{"the namespace", []string{"admin", "--help"}, flaggedNamespacedRoot},
		{"a matched subcommand", []string{"admin", "update", "--help"}, flaggedNamespacedRoot},
		{"after a double dash", []string{"admin", "--help", "--", "uodate"}, flaggedNamespacedRoot},
		{"an unmarked group", []string{"group", "lst", "--help"}, unmarked},
	} {
		withArgs(t, c.args...)
		root := c.root()
		var out bytes.Buffer
		root.SetOut(&out)
		if err := execute(t, root); err != nil {
			t.Errorf("%s: help failed: %v", c.why, err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Errorf("%s: no help printed:\n%s", c.why, out.String())
		}
	}
}
