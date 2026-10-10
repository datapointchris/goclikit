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

// Cobra answers a command with no run function by printing its help and
// returning nil, whatever was typed after it. Execute supplies one, which is
// the only way a marked namespace with none can refuse a word.
func TestAMarkedNamespaceWithNoRunFunctionRefusesAWord(t *testing.T) {
	withArgs(t, "admin", "uodate")

	err := execute(t, namespacedRoot())
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
	root := &cobra.Command{Use: "demo", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	group := &cobra.Command{Use: "hand", Annotations: map[string]string{NamespaceAnnotation: ""}}
	group.AddCommand(&cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(group)

	err := execute(t, root)
	if want := `unknown command "lst" for "demo hand"`; err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Errorf("got:\n%v\n\nwant it to open:\n%s", err, want)
	}
}
