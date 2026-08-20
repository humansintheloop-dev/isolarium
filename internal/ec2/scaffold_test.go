package ec2

import (
	"os"
	"path/filepath"
	"testing"
)

var expectedScaffoldingFiles = []string{
	"provider.tf",
	"backend.tf",
	"network.tf",
	"security.tf",
	"keypair.tf",
	"ami.tf",
	"variables.tf",
}

func TestExtractScaffolding_WritesAllFiles(t *testing.T) {
	base := t.TempDir()

	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("ExtractScaffolding() error = %v", err)
	}

	assertDirMode(t, TerraformDir(base), 0755)
	for _, name := range expectedScaffoldingFiles {
		assertFileModeAndNotEmpty(t, filepath.Join(TerraformDir(base), name), 0644)
	}
}

func TestExtractScaffolding_DoesNotOverwriteExistingFiles(t *testing.T) {
	base := t.TempDir()
	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("first ExtractScaffolding() error = %v", err)
	}

	providerPath := filepath.Join(TerraformDir(base), "provider.tf")
	edited := []byte("# locally edited by the user\n")
	if err := os.WriteFile(providerPath, edited, 0644); err != nil {
		t.Fatalf("editing provider.tf: %v", err)
	}

	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("second ExtractScaffolding() error = %v", err)
	}

	got, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatalf("reading provider.tf: %v", err)
	}
	if string(got) != string(edited) {
		t.Errorf("provider.tf = %q, want the locally edited content %q", string(got), string(edited))
	}
}

func TestExtractScaffolding_SecurityGroupAllowsOnlySSHFromIngressCIDR(t *testing.T) {
	base := t.TempDir()
	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("ExtractScaffolding() error = %v", err)
	}

	security := readScaffoldingFile(t, base, "security.tf")

	assertContainsAll(t, "security.tf", security,
		"from_port   = 22",
		"to_port     = 22",
		`protocol    = "tcp"`,
		"cidr_blocks = [var.ingress_cidr]",
	)
	if occurrences(security, "ingress {") != 1 {
		t.Errorf("security.tf declares %d ingress blocks, want exactly 1", occurrences(security, "ingress {"))
	}
}

func TestExtractScaffolding_BackendIsPartialWithLockfile(t *testing.T) {
	base := t.TempDir()
	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("ExtractScaffolding() error = %v", err)
	}

	backend := readScaffoldingFile(t, base, "backend.tf")

	assertContainsAll(t, "backend.tf", backend, `backend "s3"`, "use_lockfile = true")
	assertContainsNone(t, "backend.tf", backend, "bucket", "key", "region")
}

func TestExtractScaffolding_ProviderPinsVersionsAndTagsResources(t *testing.T) {
	base := t.TempDir()
	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("ExtractScaffolding() error = %v", err)
	}

	provider := readScaffoldingFile(t, base, "provider.tf")

	assertContainsAll(t, "provider.tf", provider,
		`required_version = ">= 1.10"`,
		"required_providers",
		`source  = "hashicorp/aws"`,
		"region = var.region",
		`ManagedBy = "isolarium"`,
	)
}

func TestExtractScaffolding_NetworkDeclaresPublicSubnetTopology(t *testing.T) {
	base := t.TempDir()
	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("ExtractScaffolding() error = %v", err)
	}

	network := readScaffoldingFile(t, base, "network.tf")

	assertContainsAll(t, "network.tf", network,
		`resource "aws_vpc"`,
		`cidr_block           = "10.42.0.0/16"`,
		"enable_dns_hostnames = true",
		"enable_dns_support   = true",
		`resource "aws_subnet"`,
		`cidr_block              = "10.42.1.0/24"`,
		"map_public_ip_on_launch = true",
		`resource "aws_internet_gateway"`,
		`resource "aws_route_table"`,
		`cidr_block = "0.0.0.0/0"`,
		`resource "aws_route_table_association"`,
	)
}

func TestExtractScaffolding_DeclaresRequiredVariablesAndAMILookup(t *testing.T) {
	base := t.TempDir()
	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("ExtractScaffolding() error = %v", err)
	}

	assertContainsAll(t, "variables.tf", readScaffoldingFile(t, base, "variables.tf"),
		`variable "ingress_cidr"`,
		`variable "public_key"`,
		`variable "region"`,
	)
	assertContainsAll(t, "keypair.tf", readScaffoldingFile(t, base, "keypair.tf"),
		`resource "aws_key_pair"`,
		"public_key = var.public_key",
	)
	assertContainsAll(t, "ami.tf", readScaffoldingFile(t, base, "ami.tf"),
		`data "aws_ssm_parameter" "ubuntu_ami"`,
		"/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id",
	)
}
