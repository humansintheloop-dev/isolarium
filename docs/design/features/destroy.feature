Feature: Destroy an isolated environment
  As a developer
  I want to tear an environment down
  So that its resources and any credentials inside it are gone

  Scenario: Destroy an existing environment
    Given a running environment named "isolarium" of type "vm" exists
    When I run "isolarium destroy"
    Then the environment "isolarium" no longer exists
    And its metadata under ~/.isolarium/isolarium/vm/ is removed
    And "isolarium status" no longer lists it
    And the command exits successfully

  Scenario: Destroy a stopped environment
    Given a stopped environment named "isolarium" of type "vm" exists
    When I run "isolarium destroy"
    Then the environment "isolarium" no longer exists

  Scenario: Destroying takes credentials with it
    Given an environment exists with Claude credentials copied into it
    When I run "isolarium destroy"
    Then the copied credentials no longer exist anywhere

  Scenario: Destroy by name
    Given environments named "isolarium" and "feature-x" exist
    When I run "isolarium destroy --name feature-x"
    Then the environment "feature-x" no longer exists
    And the environment "isolarium" still exists

  Scenario Outline: Isolation type is detected from the name
    Given exactly one environment named "feature-x" exists, of type "<type>"
    When I run "isolarium destroy --name feature-x"
    Then the "<type>" environment "feature-x" no longer exists, without --type being given

    Examples:
      | type      |
      | vm        |
      | container |
      | ec2       |

  Scenario: Ambiguous environment name
    Given environments named "feature-x" exist under two isolation types
    When I run "isolarium destroy --name feature-x"
    Then the command fails asking me to pass --type
    And neither environment is destroyed

  Scenario: Nothing to destroy
    Given no environment with the default name "isolarium" and default type "vm" exists
    When I run "isolarium destroy"
    Then the command reports there is nothing to destroy
    And exits successfully
    And no environment was created

  Scenario: Other environments are untouched
    Given running environments "one" and "two" exist
    When I run "isolarium destroy --name one"
    Then "one" no longer exists
    And "two" still exists and is still running
