package goclikit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/spf13/cobra"

	"github.com/datapointchris/goselfupdate/autoupdate"
)

// apiError is the shape an HTTP-backed CLI has: a status and the server's own
// message.
type apiError struct {
	status  int
	message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("API request failed (%d): %s", e.status, e.message)
}

func apiClassifier(err error) (string, bool) {
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.status != http.StatusNotFound {
		return "", false
	}
	return apiErr.message, true
}

// errMissing is the shape a locally-backed CLI has: a bare sentinel, with
// whatever context the call site wrapped around it.
var errMissing = errors.New("not found")

func sentinelClassifier(err error) (string, bool) {
	if !errors.Is(err, errMissing) {
		return "", false
	}
	return err.Error(), true
}

const (
	searchHint = "Search items by title: demo items search <query>"
	listHint   = "Completed items are hidden: demo items list --status all"
	groupHint  = "List every project: demo projects list"
)

// ran records the command cobra handed to RunE, so a test can compare it to
// the one Execute resolved.
var ran *cobra.Command

// notFoundRoot is three deep, with hints on both the outer group and the inner
// one, so the ancestry lookup has something to choose between.
func notFoundRoot(failWith error) *cobra.Command {
	ran = nil
	root := &cobra.Command{Use: "demo", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.PersistentFlags().Bool("no-input", false, "")

	projects := WithRecoveryHints(&cobra.Command{Use: "projects"}, groupHint)
	items := WithRecoveryHints(&cobra.Command{Use: "items"}, searchHint, listHint)
	show := &cobra.Command{
		Use:     "show",
		Aliases: []string{"get"},
		Args:    cobra.ArbitraryArgs,
		RunE:    func(c *cobra.Command, _ []string) error { ran = c; return failWith },
	}
	items.AddCommand(show)
	projects.AddCommand(items)
	root.AddCommand(projects)
	return root
}

func executeWith(t *testing.T, root *cobra.Command, options ...Option) error {
	t.Helper()
	return Execute(context.Background(), root, autoupdate.Config{Suppress: true}, options...)
}

func missing(message string) *apiError {
	return &apiError{status: http.StatusNotFound, message: message}
}

// TestTheCommandExecuteResolvesIsTheCommandCobraRuns is the claim the whole
// design rests on. Resolving before the run and hinting after it is only
// correct if cobra's own Find names the command cobra then executes.
func TestTheCommandExecuteResolvesIsTheCommandCobraRuns(t *testing.T) {
	lines := map[string][]string{
		"plain":                 {"projects", "items", "show", "1"},
		"alias":                 {"projects", "items", "get", "1"},
		"persistent flag first": {"--no-input", "projects", "items", "show", "1"},
		"flag between":          {"projects", "items", "--no-input", "show", "1"},
		"flag after the leaf":   {"projects", "items", "show", "--no-input", "1"},
	}
	for name, args := range lines {
		t.Run(name, func(t *testing.T) {
			withArgs(t, args...)
			root := notFoundRoot(missing("item 1 not found"))
			resolved, _ := resolveTarget(root)

			_ = executeWith(t, root, WithNotFound(apiClassifier))

			if ran == nil {
				t.Fatal("nothing ran")
			}
			if resolved != ran {
				t.Fatalf("Execute resolved %q, cobra ran %q",
					resolved.CommandPath(), ran.CommandPath())
			}
		})
	}
}

func TestANotFoundNamesTheSubjectAndTheNearestHints(t *testing.T) {
	withArgs(t, "projects", "items", "show", "999999")
	err := executeWith(t, notFoundRoot(missing("item 999999 not found")), WithNotFound(apiClassifier))

	want := "item 999999 not found\n  " + searchHint + "\n  " + listHint
	if err == nil || err.Error() != want {
		t.Fatalf("got:\n%v\n\nwant:\n%s", err, want)
	}
}

// The nearest annotated ancestor replaces the outer one. Accumulating would
// send someone looking for a missing item after the project list.
func TestTheOuterGroupsHintsAreNotAdded(t *testing.T) {
	withArgs(t, "projects", "items", "show", "999999")
	err := executeWith(t, notFoundRoot(missing("item 999999 not found")), WithNotFound(apiClassifier))

	if err == nil {
		t.Fatal("expected the not-found")
	}
	if got := err.Error(); contains(got, groupHint) {
		t.Fatalf("the outer group's hint accumulated:\n%s", got)
	}
}

func TestTheOriginalErrorStaysReachable(t *testing.T) {
	withArgs(t, "projects", "items", "show", "1")
	cause := missing("item 1 not found")
	err := executeWith(t, notFoundRoot(cause), WithNotFound(apiClassifier))

	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As no longer reaches the cause: %v", err)
	}
	if apiErr.status != http.StatusNotFound {
		t.Fatalf("status = %d", apiErr.status)
	}
}

// A CLI backed by a local store has no status code and no server message. The
// classifier is the seam that lets it use the same mechanism.
func TestASentinelBackedToolUsesTheSameMechanism(t *testing.T) {
	withArgs(t, "projects", "items", "show", "inbox")
	wrapped := fmt.Errorf("board %q: %w", "inbox", errMissing)
	err := executeWith(t, notFoundRoot(wrapped), WithNotFound(sentinelClassifier))

	want := "board \"inbox\": not found\n  " + searchHint + "\n  " + listHint
	if err == nil || err.Error() != want {
		t.Fatalf("got:\n%v\n\nwant:\n%s", err, want)
	}
}

func TestAnErrorTheClassifierRejectsIsUntouched(t *testing.T) {
	withArgs(t, "projects", "items", "show", "1")
	boom := errors.New("connection refused")
	err := executeWith(t, notFoundRoot(boom), WithNotFound(apiClassifier))

	if !errors.Is(err, boom) || err.Error() != boom.Error() {
		t.Fatalf("a non-not-found was rewritten: %v", err)
	}
}

// A not-found carrying no detail — a proxy error page, an auth redirect
// decoding to a status and nothing else — is not evidence that any resource
// was missing. Claiming one would name a thing the tool does not have.
func TestANotFoundWithNoSubjectIsUntouched(t *testing.T) {
	withArgs(t, "projects", "items", "show", "1")
	bare := missing("")
	err := executeWith(t, notFoundRoot(bare), WithNotFound(apiClassifier))

	if err == nil || err.Error() != bare.Error() {
		t.Fatalf("a subjectless not-found was rewritten: %v", err)
	}
}

// The ten CLIs that never adopt this pass no option, and nothing about their
// errors may change.
func TestWithoutTheOptionNothingChanges(t *testing.T) {
	withArgs(t, "projects", "items", "show", "1")
	cause := missing("item 1 not found")
	err := executeWith(t, notFoundRoot(cause))

	if err == nil || err.Error() != cause.Error() {
		t.Fatalf("a CLI supplying no classifier had its error rewritten: %v", err)
	}
}

func TestACommandWithNoHintsInItsAncestryIsUntouched(t *testing.T) {
	withArgs(t, "orphan")
	root := &cobra.Command{Use: "demo", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	cause := missing("thing 1 not found")
	root.AddCommand(&cobra.Command{
		Use:  "orphan",
		RunE: func(*cobra.Command, []string) error { return cause },
	})

	err := executeWith(t, root, WithNotFound(apiClassifier))
	if err == nil || err.Error() != cause.Error() {
		t.Fatalf("a command with no hints was rewritten: %v", err)
	}
}

// A not-found can be raised while resolving a flag, before RunE is reached. A
// wrapper around each RunE cannot see it; resolving the target up front and
// hinting the returned error can.
func TestANotFoundRaisedBeforeRunEIsStillHinted(t *testing.T) {
	withArgs(t, "projects", "items", "show", "7")
	root := notFoundRoot(nil)
	target, _ := resolveTarget(root)
	target.PersistentPreRunE = func(*cobra.Command, []string) error {
		return missing("item 7 not found")
	}

	err := executeWith(t, root, WithNotFound(apiClassifier))
	want := "item 7 not found\n  " + searchHint + "\n  " + listHint
	if err == nil || err.Error() != want {
		t.Fatalf("got:\n%v\n\nwant:\n%s", err, want)
	}
}

// A line cobra rejected never reached a resource, so it must stay a usage
// error and keep its exit code 2 rather than becoming a not-found.
func TestAnUnknownCommandStaysAUsageError(t *testing.T) {
	withArgs(t, "nope")
	err := executeWith(t, notFoundRoot(missing("item not found")), WithNotFound(apiClassifier))

	if !errors.Is(err, ErrUsage) {
		t.Fatalf("an unknown command stopped being a usage error: %v", err)
	}
}

// A consumer's classifier is the consumer's, and a loose one says yes to
// errors it should not. Exit code 2 has to survive that: a line cobra rejected
// stays a usage error however eagerly the classifier claims it.
func TestALooseClassifierCannotTurnAUsageErrorIntoANotFound(t *testing.T) {
	withArgs(t, "nope")
	greedy := func(err error) (string, bool) { return err.Error(), true }

	// Hints on the root, so an unresolvable line resolves to a command that
	// has some. Without that the branch is never reached and the test passes
	// for the wrong reason.
	root := notFoundRoot(nil)
	WithRecoveryHints(root, groupHint)

	err := executeWith(t, root, WithNotFound(greedy))
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("a greedy classifier swallowed the usage classification: %v", err)
	}
}

func TestWithRecoveryHintsWritesTheExportedAnnotation(t *testing.T) {
	cmd := WithRecoveryHints(&cobra.Command{Use: "items"}, searchHint, listHint)

	got := cmd.Annotations[RecoveryHintsAnnotation]
	if want := searchHint + "\n" + listHint; got != want {
		t.Fatalf("annotation = %q, want %q", got, want)
	}
}

// The annotation is the contract, not the helper. A CLI keeping this package
// out of its command files writes the key itself and must get the same result.
func TestAHandWrittenAnnotationWorksTheSame(t *testing.T) {
	withArgs(t, "hand", "show", "1")
	root := &cobra.Command{Use: "demo", SilenceErrors: true, SilenceUsage: true}
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	group := &cobra.Command{
		Use:         "hand",
		Annotations: map[string]string{RecoveryHintsAnnotation: searchHint},
	}
	group.AddCommand(&cobra.Command{
		Use:  "show",
		Args: cobra.ArbitraryArgs,
		RunE: func(*cobra.Command, []string) error { return missing("item 1 not found") },
	})
	root.AddCommand(group)

	err := executeWith(t, root, WithNotFound(apiClassifier))
	if want := "item 1 not found\n  " + searchHint; err == nil || err.Error() != want {
		t.Fatalf("got:\n%v\n\nwant:\n%s", err, want)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
