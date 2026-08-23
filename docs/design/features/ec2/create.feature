@ec2
Feature: Create an EC2 environment
  Everything in ../create.feature holds for --type ec2. This file adds what is
  specific to placing the environment on an EC2 instance and does not restate
  the shared outcomes.

  Background:
    Given I am in the root of a git working tree of "owner/repo" on branch "feature-x"
    And a GitHub App is configured
    And AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and AWS_REGION are configured

  Scenario: Create the first EC2 environment
    Given no environment named "isolarium-ec2" of type "ec2" exists
    And no shared EC2 infrastructure exists in this account and region
    And ~/.isolarium/ec2/ does not exist
    When I run "isolarium create --type ec2"
    Then a Terraform state bucket "isolarium-tfstate-<account>-<region>" exists, versioned, encrypted, with public access blocked
    And ~/.isolarium/ec2/terraform/ holds the extracted Terraform configuration
    And an Ed25519 key pair exists at ~/.isolarium/ec2/id_ed25519 (mode 0600) and id_ed25519.pub (mode 0644)
    And a shared VPC, subnet, internet gateway, route table, security group and key pair were created
    And the security group allows SSH only from my host's current public address as a /32
    And a "t3.large" Ubuntu 24.04 instance was launched with an encrypted root volume that is deleted on termination
    And isolarium waited for SSH and then for cloud-init to finish first-boot provisioning
    And the branch "feature-x" was pushed before the clone token was minted
    And "owner/repo" is cloned at /home/ubuntu/repo, checked out on "feature-x"
    And the clone's origin URL contains no token
    And git user.name inside the clone is my host's user.name suffixed with " - i2code"
    And .claude/settings.local.json and CLAUDE.md were copied from the host into /home/ubuntu/repo
    And ~/.isolarium/isolarium-ec2/ec2/metadata.json records the instance ID and public DNS name
    And "isolarium status" reports "isolarium-ec2", "ec2", "running", "owner/repo (feature-x)"

  Scenario: Creating a second EC2 environment reuses the shared infrastructure
    Given a running environment named "one" of type "ec2" exists
    And the shared EC2 infrastructure and state bucket exist
    When I run "isolarium create --type ec2 --name two"
    Then a second instance was launched for "two"
    And no second VPC, subnet, security group, key pair or state bucket was created
    And "one" is still running

  Scenario: Host-side state survives across creates
    Given ~/.isolarium/ec2/terraform/ contains a file I edited
    And ~/.isolarium/ec2/id_ed25519 exists
    When I run "isolarium create --type ec2 --name two"
    Then my edited file is unchanged
    And the same key pair was used

  Scenario: The SSH ingress rule follows the host's address
    Given an environment of type "ec2" was created when my public address was 203.0.113.5
    And my public address is now 198.51.100.7
    When I run "isolarium create --type ec2 --name two"
    Then the security group allows SSH from 198.51.100.7/32
    And no longer allows 203.0.113.5/32

  Scenario: AWS_REGION is required
    Given no environment named "isolarium-ec2" of type "ec2" exists
    And AWS_REGION is not set
    When I run "isolarium create --type ec2"
    Then the command fails before contacting AWS, saying AWS_REGION is not set
    And no instance, state bucket or host-side state was created

  Scenario: Invalid environment name
    Given no environment of type "ec2" exists
    When I run "isolarium create --type ec2 --name 'bad name!'"
    Then the command fails before contacting AWS, saying the name is invalid
    And no instance was created

  Scenario: --work-directory is rejected
    When I run "isolarium create --type ec2 --work-directory /elsewhere"
    Then the command fails saying --work-directory is not supported with --type ec2
    And no instance was created

  Scenario: A script that escapes the repository is rejected before AWS is touched
    Given pid.yaml declares an ec2 creation script with path "../outside.sh"
    When I run "isolarium create --type ec2"
    Then the command fails naming "../outside.sh"
    And no instance, state bucket or host-side state was created

  Scenario: Provisioning does not finish in time
    Given the instance never answers on SSH
    When I run "isolarium create --type ec2"
    Then the command fails after the 5-minute SSH wait
    And ~/.isolarium/isolarium-ec2/ec2/metadata.json was still recorded
    And "isolarium status" lists "isolarium-ec2" so that "isolarium destroy --type ec2" can terminate it
