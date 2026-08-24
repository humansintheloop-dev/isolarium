Feature: Run a command inside an isolated environment
  As a developer running a coding agent
  I want to execute commands inside an environment with repo-scoped credentials
  So that the agent can build, test and push without my personal credentials

  Background:
    Given I am in the root of a git working tree of "owner/repo"
    And a GitHub App is configured
    And a running environment named "isolarium" of type "vm" holds this working tree

  Scenario: Run a command
    Given Claude Code credentials exist on the host
    And pid.yaml declares run env "CS_ACCESS_TOKEN" for type "vm"
    And CS_ACCESS_TOKEN and HOME_VAR are set on the host
    When I run "isolarium run --env FOO=bar --env HOME_VAR -- make test"
    Then "make test" ran inside the environment "isolarium"
    And its working directory was the copy of the working tree
    And it ran with GH_TOKEN set to a short-lived token scoped to "owner/repo"
    And git inside the environment was configured to authenticate with that token
    And the token was not written to disk inside the environment
    And it ran with CS_ACCESS_TOKEN and HOME_VAR set to the host's values
    And it ran with FOO set to "bar"
    And the host's Claude credentials were available inside the environment
    And isolarium exited with the command's exit code

  Scenario: A failing command's exit code is returned unchanged
    When I run "isolarium run -- sh -c 'exit 3'"
    Then isolarium exited with code 3

  Scenario: Opt out of the GitHub token
    When I run "isolarium run --no-gh-token -- make build"
    Then no token was obtained
    And "make build" ran without GH_TOKEN

  Scenario: Do not carry the host's Claude session in
    Given Claude Code credentials exist on the host
    When I run "isolarium run --fresh-login -- claude"
    Then "claude" ran inside the environment
    And the host's Claude credentials were not copied

  Scenario: Run an interactive command
    When I run "isolarium run -i -- claude"
    Then "claude" ran inside the environment with a TTY attached to my terminal
    And isolarium exited with its exit code when it ended

  Scenario: Create the environment first if it does not exist
    Given no environment with the default name "isolarium" and default type "vm" exists
    When I run "isolarium run --create -- make test"
    Then an environment named "isolarium" of type "vm" was created as "isolarium create" would
    And "make test" then ran inside it
    And the environment still exists afterwards

  Scenario: --create on an existing environment does not recreate it
    Given a file "scratch.txt" was written inside the environment earlier
    When I run "isolarium run --create -- make test"
    Then no environment was created
    And "make test" ran inside the existing environment
    And "scratch.txt" is still there

  Scenario Outline: Isolation type is detected from the name
    Given exactly one environment named "feature-x" exists, of type "<type>"
    When I run "isolarium run --name feature-x -- make"
    Then "make" ran in the "<type>" environment "feature-x" without --type being given

    Examples:
      | type      |
      | vm        |
      | container |
      | ec2       |

  Scenario: Ambiguous environment name
    Given environments named "feature-x" exist under two isolation types
    When I run "isolarium run --name feature-x -- make"
    Then the command fails asking me to pass --type
    And "make" did not run in either environment

  Scenario: No command given
    When I run "isolarium run"
    Then the command fails saying no command was specified
    And nothing ran inside the environment

  Scenario: A flag the isolation type does not support is rejected
    Given a flag that only applies to another isolation type
    When I run "isolarium run" with that flag
    Then the command fails saying the flag is not supported with this --type
    And nothing ran
