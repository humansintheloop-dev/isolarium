Feature: Show the status of environments
  As a developer
  I want to see which environments exist and their state
  So that I know what is running and what to clean up

  Scenario: No environments
    Given no environments exist under ~/.isolarium
    When I run "isolarium status"
    Then it prints "No environments found"

  Scenario: List all environments
    Given a running environment "a" of type "vm" exists
    And a stopped environment "b" of type "container" exists
    When I run "isolarium status"
    Then it prints a table with columns NAME, TYPE, STATE, DETAILS
    And one row is "a", "vm", "running"
    And one row is "b", "container", "stopped"
    And there are no other rows

  Scenario: Details show what the environment holds
    Given an environment exists holding "owner/repo" on branch "main"
    When I run "isolarium status"
    Then its details identify the repository and branch it holds

  Scenario: Filter by name
    Given environments "a" and "b" exist
    When I run "isolarium status --name a"
    Then only "a" is listed

  Scenario: Filter by a name that matches nothing
    Given environments "a" and "b" exist
    When I run "isolarium status --name c"
    Then it prints "No environments found"

  Scenario: Filter by isolation type
    Given a vm environment and a container environment exist
    When I run "isolarium status --type container"
    Then only the container environment is listed

  Scenario: State reflects the environment as it is now
    Given an environment exists that isolarium last saw running
    And it has since been stopped outside isolarium
    When I run "isolarium status"
    Then its state is reported as "stopped"

  Scenario: Status changes nothing
    Given environments exist
    When I run "isolarium status"
    Then every environment is in the same state as before
