Feature: Open a shell inside an isolated environment
  As a developer
  I want an interactive shell inside an environment
  So that I can inspect and work in it directly

  Background:
    Given a running environment named "isolarium" of type "vm" holds this working tree

  Scenario: Open a shell
    Given Claude Code credentials exist on the host
    When I run "isolarium shell --env FOO=bar"
    Then an interactive shell opened inside the environment "isolarium"
    And its working directory was the copy of the working tree
    And it had FOO set to "bar"
    And the host's Claude credentials were available inside the environment
    And when I exited the shell with code 2, isolarium exited with code 2

  Scenario: Skip offering Claude credentials
    Given Claude Code credentials exist on the host
    When I run "isolarium shell --copy-session=false"
    Then a shell opened inside the environment
    And no credentials were copied

  Scenario Outline: Isolation type is detected from the name
    Given exactly one environment named "feature-x" exists, of type "<type>"
    When I run "isolarium shell --name feature-x"
    Then the shell opened in the "<type>" environment "feature-x" without --type being given

    Examples:
      | type      |
      | vm        |
      | container |
      | ec2       |

  Scenario: Ambiguous environment name
    Given environments named "feature-x" exist under two isolation types
    When I run "isolarium shell --name feature-x"
    Then the command fails asking me to pass --type
    And no shell opened

  Scenario: A flag the isolation type does not support is rejected
    Given a flag that only applies to another isolation type
    When I run "isolarium shell" with that flag
    Then the command fails saying the flag is not supported with this --type
    And no shell opens
