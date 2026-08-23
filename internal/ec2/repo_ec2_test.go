//go:build ec2

package ec2_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humansintheloop-dev/isolarium/internal/git"
)

const (
	isolationNameSuffix      = " - i2code"
	grepFoundNothingExitCode = 1
	instanceHomeDir          = "/home/ubuntu"
)

// TestEC2Instance_HasRepositoryAtBranch proves that create leaves a real
// instance holding the branch it was run from, attributed to the isolated
// author, with the host's project config alongside the clone.
//
// Its clone-cleanliness assertion needs an instance nothing has written to yet,
// which is why this file sorts ahead of the ones that do.
func TestEC2Instance_HasRepositoryAtBranch(t *testing.T) {
	environment := sharedInstance(t)

	environment.assertCheckedOutBranchIsTheOneCreateRanFrom()
	environment.assertCommitsAreAttributedToTheIsolatedAuthor()
	environment.assertTheCloneCarriesNoModifications()
	environment.assertProjectConfigTravelledFromTheHost()
}

// TestEC2Instance_HasNoPersistedToken proves the clone token lives only in the
// argument list of the single git clone that needs it, and never lands on the
// instance's disk. The binary minted that token itself when it created the
// instance, so the suite never sees it and searches instead for the shape of
// the URL it travelled in, which is the only form git would ever record it in.
// It reads the whole home directory, so it too has to run before anything else
// puts a file there.
func TestEC2Instance_HasNoPersistedToken(t *testing.T) {
	environment := sharedInstance(t)

	for _, search := range secretSearches() {
		environment.assertNothingOnTheInstanceContains(search)
	}
}

func (e *ec2Environment) assertCheckedOutBranchIsTheOneCreateRanFrom() {
	e.t.Helper()

	branch := e.gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if branch != e.repository.branch {
		e.t.Errorf("the instance is on branch %q, want %q", branch, e.repository.branch)
	}
}

func (e *ec2Environment) assertCommitsAreAttributedToTheIsolatedAuthor() {
	e.t.Helper()

	wantName := e.repository.authorName + isolationNameSuffix
	if name := e.gitOutput("config", "user.name"); name != wantName {
		e.t.Errorf("git user.name on the instance = %q, want %q", name, wantName)
	}

	wantEmail := git.TransformEmailForIsolation(e.repository.authorEmail)
	if email := e.gitOutput("config", "user.email"); email != wantEmail {
		e.t.Errorf("git user.email on the instance = %q, want %q", email, wantEmail)
	}
}

// assertTheCloneCarriesNoModifications separates the two things git status
// reports: no tracked file may differ from the branch, and the only untracked
// entries may be the project config create deliberately copied in.
func (e *ec2Environment) assertTheCloneCarriesNoModifications() {
	e.t.Helper()

	if modified := e.gitOutput("status", "--porcelain", "--untracked-files=no"); modified != "" {
		e.t.Errorf("the clone has modified tracked files:\n%s", modified)
	}
	for _, entry := range untrackedEntries(e.gitOutput("status", "--porcelain")) {
		if !isLeftInTheCloneByCreate(entry) {
			e.t.Errorf("unexpected untracked entry %q in the clone", entry)
		}
	}
}

func (e *ec2Environment) assertProjectConfigTravelledFromTheHost() {
	e.t.Helper()

	for _, name := range projectConfigFiles() {
		if !hostHasProjectConfig(e.workDir, name) {
			e.t.Logf("%s does not exist on the host, so nothing should have travelled", name)
			continue
		}
		exitCode, _ := e.askInstanceInRepo("test", "-f", name)
		if exitCode != 0 {
			e.t.Errorf("%s exists on the host but not in the clone on the instance", name)
		}
	}
}

func (e *ec2Environment) assertNothingOnTheInstanceContains(search secretSearch) {
	e.t.Helper()

	exitCode, output := e.askInstance(search.args...)
	if exitCode == grepFoundNothingExitCode && strings.TrimSpace(output) == "" {
		return
	}
	if exitCode == 0 {
		e.t.Errorf("%s is persisted on the instance, in: %s", search.what, strings.Join(strings.Fields(output), ", "))
		return
	}
	e.t.Fatalf("searching the instance for %s exited %d, want %d", search.what, exitCode, grepFoundNothingExitCode)
}

// secretSearch is one recursive grep over the instance's home directory that
// has to come back empty, labelled so a failure names what leaked rather than
// echoing the secret itself.
type secretSearch struct {
	what string
	args []string
}

// secretSearches covers the shape of the URL the token travelled in. They skip
// binaries and the checkout, because this repository's own source and the
// Docker binary both carry the literal x-access-token for reasons that have
// nothing to do with the clone — but they do read the clone's git metadata,
// which is where git records the authenticated remote it fetched from.
func secretSearches() []secretSearch {
	return []secretSearch{
		{
			what: "an authenticated clone URL outside the checkout",
			args: []string{"grep", "-rlI", "--exclude-dir=repo", "x-access-token", instanceHomeDir},
		},
		{
			what: "an authenticated clone URL in the clone's git metadata",
			args: []string{"grep", "-rlI", "x-access-token", instanceHomeDir + "/repo/.git"},
		},
	}
}

// projectConfigFiles mirrors the list create carries from the host checkout,
// which is deliberately untracked and so never arrives with the clone.
func projectConfigFiles() []string {
	return []string{".claude/settings.local.json", "CLAUDE.md"}
}

func hostHasProjectConfig(hostDir, name string) bool {
	_, err := os.Stat(filepath.Join(hostDir, name))
	return err == nil
}

// untrackedEntries strips the "?? " status prefix, leaving the paths git
// reports as new. Git collapses a wholly new directory into a single entry.
func untrackedEntries(status string) []string {
	var entries []string
	for _, line := range strings.Split(status, "\n") {
		if path, found := strings.CutPrefix(line, "?? "); found {
			entries = append(entries, path)
		}
	}
	return entries
}

// isLeftInTheCloneByCreate accepts everything create deliberately puts in the
// clone — the copied project config and the markers the pid.yaml scripts write —
// including the collapsed directory git reports when a copied file is the first
// thing inside it.
func isLeftInTheCloneByCreate(entry string) bool {
	for _, name := range append(projectConfigFiles(), pidScriptMarkers()...) {
		if entry == name || strings.HasPrefix(name, entry) {
			return true
		}
	}
	return false
}

func (e *ec2Environment) gitOutput(args ...string) string {
	e.t.Helper()

	exitCode, output := e.askInstanceInRepo(append([]string{"git"}, args...)...)
	if exitCode != 0 {
		e.t.Fatalf("git %s on the instance exited %d, want 0", strings.Join(args, " "), exitCode)
	}
	return strings.TrimSpace(output)
}
