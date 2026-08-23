Feature: Create an isolated environment
  As a developer running a coding agent
  I want to create an isolated environment for my working tree
  So that the agent can work on my repository without reaching my host

  Background:
    Given I am in the root of a git working tree of "owner/repo" on branch "topic"
    And a GitHub App is configured

  Scenario: Create an environment
    Given no environment with the default name "isolarium" and default type "vm" exists
    And pid.yaml declares, for type "vm":
      | section                              | path                    | env             |
      | create.creation_scripts              | scripts/install-go.sh   | CS_ACCESS_TOKEN |
      | create.post_creation_scripts.host    | scripts/host-setup.sh   |                 |
      | create.post_creation_scripts.env     | scripts/env-setup.sh    |                 |
    And CS_ACCESS_TOKEN is set on the host
    When I run "isolarium create"
    Then an environment named "isolarium" of type "vm" was created
    And "owner/repo" is inside the environment, checked out on "topic"
    And any token used to fetch it was short-lived, scoped to "owner/repo", and is not stored inside the environment
    And scripts/install-go.sh ran inside the environment during creation, with CS_ACCESS_TOKEN set to the host's value
    And scripts/host-setup.sh ran on the host after the environment existed
    And scripts/env-setup.sh ran inside the environment after the environment existed
    And the environment's metadata is recorded under ~/.isolarium/isolarium/vm/
    And its state is "running"
    And the command exits successfully

  Scenario: Create an environment without project configuration
    Given no environment with the default name "isolarium" and default type "vm" exists
    And the working tree has no pid.yaml
    When I run "isolarium create"
    Then an environment named "isolarium" of type "vm" was created
    And "owner/repo" is inside the environment, checked out on "topic"
    And no scripts ran
    And the environment's metadata is recorded under ~/.isolarium/isolarium/vm/
    And its state is "running"

  Scenario: Choose the environment name
    Given no environment named "feature-x" of the default type "vm" exists
    When I run "isolarium create --name feature-x"
    Then an environment named "feature-x" of type "vm" was created
    And its metadata is recorded under ~/.isolarium/feature-x/vm/

  Scenario Outline: Choose the isolation type
    Given no environment named "<default name>" of type "<type>" exists
    When I run "isolarium create --type <type>"
    Then an environment named "<default name>" of type "<type>" was created
    And its metadata is recorded under ~/.isolarium/<default name>/<type>/

    Examples:
      | type      | default name        |
      | vm        | isolarium           |
      | container | isolarium-container |
      | ec2       | isolarium-ec2       |

  Scenario Outline: Take the name and type from environment variables
    Given no environment named "feature-x" of type "<type>" exists
    And ISOLARIUM_NAME is "feature-x"
    And ISOLARIUM_TYPE is "<type>"
    When I run "isolarium create"
    Then an environment named "feature-x" of type "<type>" was created

    Examples:
      | type      |
      | vm        |
      | container |
      | ec2       |

  Scenario: Flags override environment variables
    Given no environment named "feature-y" of the default type "vm" exists
    And ISOLARIUM_NAME is "feature-x"
    When I run "isolarium create --name feature-y"
    Then an environment named "feature-y" was created
    And no environment named "feature-x" was created

  Scenario: Creating an environment that already exists
    Given a running environment named "isolarium" of type "vm" exists
    And a file "scratch.txt" was written inside it earlier
    When I run "isolarium create"
    Then the command fails saying the environment already exists
    And no second environment was created
    And "scratch.txt" is still inside the environment

  Scenario: Reject a script that escapes the repository
    Given no environment with the default name "isolarium" and default type "vm" exists
    And pid.yaml declares a script with path "../outside.sh"
    When I run "isolarium create"
    Then the command fails with an error naming "../outside.sh"
    And no environment was created
    And nothing is recorded under ~/.isolarium/
