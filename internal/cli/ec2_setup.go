package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/humansintheloop-dev/isolarium/internal/backend"
	"github.com/humansintheloop-dev/isolarium/internal/ec2"
	"github.com/humansintheloop-dev/isolarium/internal/git"
)

func createAndSetupEC2(b backend.Backend, name, workDirectory string) error {
	return b.Create(backend.CreateOptions{
		Name:          name,
		WorkDirectory: workDirectory,
		Repository:    resolveEC2Repository,
	})
}

// resolveEC2Repository is the host-side work that must happen before the
// instance can be given the repository: resolve it, push the branch being worked
// on, and mint a short-lived clone token. The backend calls it only once an
// instance exists, so a create that fails earlier neither pushes nor mints.
var resolveEC2Repository = func() (ec2.RepositorySpec, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return ec2.RepositorySpec{}, fmt.Errorf("failed to get current directory: %w", err)
	}

	info, err := resolveRepoInfo(cwd)
	if err != nil {
		return ec2.RepositorySpec{}, err
	}

	fmt.Printf("Pushing branch %s to remote...\n", info.branch)
	if err := git.PushBranch(cwd, info.branch); err != nil {
		return ec2.RepositorySpec{}, fmt.Errorf("failed to push branch: %w", err)
	}

	token, err := mintGitHubToken()
	if err != nil {
		return ec2.RepositorySpec{}, err
	}

	author, err := resolveHostGitAuthor(cwd)
	if err != nil {
		return ec2.RepositorySpec{}, err
	}

	return ec2.RepositorySpec{
		Owner:       info.owner,
		Repo:        info.repo,
		Branch:      info.branch,
		Token:       token,
		HostDir:     cwd,
		AuthorEmail: author.email,
		AuthorName:  author.name,
	}, nil
}

type hostGitAuthor struct {
	email string
	name  string
}

func resolveHostGitAuthor(cwd string) (hostGitAuthor, error) {
	email, err := git.GetUserEmail(cwd)
	if err != nil {
		return hostGitAuthor{}, fmt.Errorf("failed to get git user.email: %w", err)
	}
	name, err := git.GetUserName(cwd)
	if err != nil {
		return hostGitAuthor{}, fmt.Errorf("failed to get git user.name: %w", err)
	}
	return hostGitAuthor{email: email, name: name}, nil
}

func destroyEC2(out io.Writer, name string) error {
	b := backend.NewEC2Backend()
	b.Out = out
	return b.Destroy(name)
}
