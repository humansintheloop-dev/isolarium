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
	repositoryEnvironmentName = "isolarium-ec2-repository-test"
	tokenEnvironmentName      = "isolarium-ec2-token-test"
	isolationNameSuffix       = " - i2code"
	grepFoundNothingExitCode  = 1
	instanceHomeDir           = "/home/ubuntu"
)

// TestEC2Instance_HasRepositoryAtBranch proves that create leaves a real
// instance holding the branch it was run from, attributed to the isolated
// author, with the host's project config alongside the clone.
func TestEC2Instance_HasRepositoryAtBranch(t *testing.T) {
	environment := startEC2Environment(t, repositoryEnvironmentName)

	environment.assertCheckedOutBranchIsTheOneCreateRanFrom()
	environment.assertCommitsAreAttributedToTheIsolatedAuthor()
	environment.assertTheCloneCarriesNoModifications()
	environment.assertProjectConfigTravelledFromTheHost()
}

// TestEC2Instance_HasNoPersistedToken proves the clone token lives only in the
// argument list of the single git clone that needs it, and never lands on the
// instance's disk.
func TestEC2Instance_HasNoPersistedToken(t *testing.T) {
	environment := startEC2Environment(t, tokenEnvironmentName)

	for _, search := range secretSearches(environment.repository.Token) {
		environment.assertNothingOnTheInstanceContains(search)
	}
}

func (e *ec2Environment) assertCheckedOutBranchIsTheOneCreateRanFrom() {
	e.t.Helper()

	branch := e.gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if branch != e.repository.Branch {
		e.t.Errorf("the instance is on branch %q, want %q", branch, e.repository.Branch)
	}
}

func (e *ec2Environment) assertCommitsAreAttributedToTheIsolatedAuthor() {
	e.t.Helper()

	wantName := e.repository.AuthorName + isolationNameSuffix
	if name := e.gitOutput("config", "user.name"); name != wantName {
		e.t.Errorf("git user.name on the instance = %q, want %q", name, wantName)
	}

	wantEmail := git.TransformEmailForIsolation(e.repository.AuthorEmail)
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
		if !isCopiedProjectConfig(entry) {
			e.t.Errorf("unexpected untracked entry %q in the clone", entry)
		}
	}
}

func (e *ec2Environment) assertProjectConfigTravelledFromTheHost() {
	e.t.Helper()

	for _, name := range projectConfigFiles() {
		if !hostHasProjectConfig(e.repository.HostDir, name) {
			e.t.Logf("%s does not exist on the host, so nothing should have travelled", name)
			continue
		}
		exitCode, _ := e.run("test", "-f", name)
		if exitCode != 0 {
			e.t.Errorf("%s exists on the host but not in the clone on the instance", name)
		}
	}
}

func (e *ec2Environment) assertNothingOnTheInstanceContains(search secretSearch) {
	e.t.Helper()

	exitCode, output := e.run(search.args...)
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

// secretSearches covers the token both by value and by the shape of the URL it
// travelled in. The token search reads every file, because a token has no
// legitimate home anywhere on the instance. The URL searches skip binaries and
// the checkout, because this repository's own source and the Docker binary both
// carry the literal x-access-token for reasons that have nothing to do with the
// clone — but they do read the clone's git metadata, which is where git records
// the authenticated remote it fetched from.
func secretSearches(token string) []secretSearch {
	return []secretSearch{
		{
			what: "the installation token",
			args: []string{"grep", "-rlF", "--", token, instanceHomeDir},
		},
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

// isCopiedProjectConfig accepts a copied file itself as well as the collapsed
// directory git reports when that file is the first thing inside it.
func isCopiedProjectConfig(entry string) bool {
	for _, name := range projectConfigFiles() {
		if entry == name || strings.HasPrefix(name, entry) {
			return true
		}
	}
	return false
}

func (e *ec2Environment) gitOutput(args ...string) string {
	e.t.Helper()

	exitCode, output := e.run(append([]string{"git"}, args...)...)
	if exitCode != 0 {
		e.t.Fatalf("git %s on the instance exited %d, want 0", strings.Join(args, " "), exitCode)
	}
	return strings.TrimSpace(output)
}
