package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/internal/storage/uow"
	"github.com/steveyegge/beads/issueops"
)

// readerProvider is lookupOnlyProvider plus the capability accessor, which is
// the door `bd show --json` now reaches the detail view through on this route.
// The reader it hands back is the REAL one, over the same failing lookup, so
// what these tests observe is the role's own error vocabulary reaching the
// command — not a stub's imitation of it.
type readerProvider struct{ lookupOnlyProvider }

func (p readerProvider) IssueReader() (issueops.Reader, error) {
	return uow.NewIssueReader(p)
}

var _ uow.IssueReaderSource = readerProvider{}

func withStubbedProxiedReader(t *testing.T, hardErr error) {
	t.Helper()
	oldProvider, oldJSON := uowProvider, jsonOutput
	uowProvider = readerProvider{lookupOnlyProvider{issues: stubLookupIssueUC{hardErr: hardErr}}}
	jsonOutput = true
	t.Cleanup(func() {
		uowProvider = oldProvider
		jsonOutput = oldJSON
	})
}

// TestShowProxiedJSONMissingIDKeepsItsContract is the regression guard for
// moving the proxied detail view onto issueops.Reader. A missing ID under
// --json now has one structured error on stdout and no human prose on stderr;
// the role still reports the miss as storage.ErrNotFound rather than as a
// resolve failure the command reported itself.
func TestShowProxiedJSONMissingIDKeepsItsContract(t *testing.T) {
	withStubbedProxiedReader(t, nil)

	stdout, stderr, err := captureShowJSONStreams(t)

	if err == nil {
		t.Error("a batch that found nothing exited zero")
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty under --json", stderr)
	}
	if !strings.Contains(stdout, `"error"`) || !strings.Contains(stdout, "no issues found") {
		t.Errorf("stdout = %q, want a structured missing-issue error", stdout)
	}
	if strings.Contains(stdout, stubRawNoRows) {
		t.Errorf("stdout leaks the raw driver sentinel: %q", stdout)
	}
}

func captureShowJSONStreams(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	stdioMutex.Lock()
	defer stdioMutex.Unlock()

	oldStdout, oldStderr := os.Stdout, os.Stderr
	outR, outW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("stdout pipe: %v", pipeErr)
	}
	errR, errW, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("stderr pipe: %v", pipeErr)
	}
	outDone := make(chan string, 1)
	errDone := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(outR)
		outDone <- string(data)
	}()
	go func() {
		data, _ := io.ReadAll(errR)
		errDone <- string(data)
	}()
	os.Stdout, os.Stderr = outW, errW
	err = runShowProxiedServer(&cobra.Command{}, context.Background(), []string{stubMissingID})
	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	stdout, stderr = <-outDone, <-errDone
	_ = outR.Close()
	_ = errR.Close()
	return stdout, stderr, err
}

// TestShowProxiedJSONBackendFailureAborts pins the one behavior this move
// changed on purpose. Resolution and assembly used to be two calls, and a
// backend failure was reported per id and skipped past when it surfaced in the
// first, aborted on when it surfaced in the second. They are one call now, so
// there is one answer, and abort is the one worth keeping: a JSON array
// missing the rows a database error swallowed is indistinguishable from a
// complete one.
func TestShowProxiedJSONBackendFailureAborts(t *testing.T) {
	withStubbedProxiedReader(t, errors.New(stubBackendError))

	var err error
	_ = captureStderrDuring(t, func() {
		err = runShowProxiedServer(&cobra.Command{}, context.Background(), []string{stubMissingID})
	})

	if err == nil {
		t.Fatal("a backend failure exited zero")
	}
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("backend failure reported as a missing issue: %v", err)
	}
}

// TestShowProxiedTextRouteDoesNotOpenAReader pins the other half of the split:
// the terminal rendering needs the raw issue and the wisp flag, which the
// detail contract does not carry, so it stays on the CLI's own resolution and
// must not pay for a reader it cannot use.
func TestShowProxiedTextRouteDoesNotOpenAReader(t *testing.T) {
	withStubbedProxiedLookup(t, nil) // a provider with NO capability accessor

	var err error
	stderr := captureStderrDuring(t, func() {
		err = runShowProxiedServer(&cobra.Command{}, context.Background(), []string{stubMissingID})
	})

	if err == nil {
		t.Error("a batch that found nothing exited zero")
	}
	if got, want := strings.SplitN(stderr, "\n", 2)[0], "Issue "+stubMissingID+" not found"; got != want {
		t.Errorf("stderr first line = %q, want %q", got, want)
	}
	if !strings.Contains(stderr, "bd history "+stubMissingID) {
		t.Errorf("expected stderr to hint at checking 'bd history %s', got: %q", stubMissingID, stderr)
	}
}
