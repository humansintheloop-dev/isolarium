package ec2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderInstanceFile_DeclaresOneInstanceAndItsOutputs(t *testing.T) {
	rendered := RenderInstanceFile("my-work", "")

	assertContainsAll(t, "instance-my-work.tf", rendered,
		`resource "aws_instance" "my-work"`,
		"ami                         = data.aws_ssm_parameter.ubuntu_ami.value",
		`instance_type               = "t3.large"`,
		"key_name                    = aws_key_pair.isolarium.key_name",
		"vpc_security_group_ids      = [aws_security_group.isolarium.id]",
		"subnet_id                   = aws_subnet.isolarium.id",
		"associate_public_ip_address = true",
		"root_block_device {",
		"volume_size           = 50",
		`volume_type           = "gp3"`,
		"encrypted             = true",
		"delete_on_termination = true",
		`Name = "my-work"`,
		`output "instance_id_my-work"`,
		"value = aws_instance.my-work.id",
		`output "public_dns_my-work"`,
		"value = aws_instance.my-work.public_dns",
	)
	if got := occurrences(rendered, `resource "aws_instance"`); got != 1 {
		t.Errorf("instance-my-work.tf declares %d aws_instance resources, want exactly 1", got)
	}
}

func TestRenderInstanceFile_CarriesNoIngressRule(t *testing.T) {
	rendered := RenderInstanceFile("my-work", "")

	assertContainsNone(t, "instance-my-work.tf", rendered, "ingress", "cidr_blocks")
}

func TestRenderInstanceFile_OmitsUserDataWhenEmpty(t *testing.T) {
	rendered := RenderInstanceFile("my-work", "")

	assertContainsNone(t, "instance-my-work.tf", rendered, "user_data")
}

func TestRenderInstanceFile_EmbedsUserDataWhenSupplied(t *testing.T) {
	rendered := RenderInstanceFile("my-work", "#cloud-config\npackages:\n  - tmux\n")

	assertContainsAll(t, "instance-my-work.tf", rendered,
		"user_data = <<-USER_DATA",
		"#cloud-config",
		"  - tmux",
		"USER_DATA",
	)
}

func TestWriteInstanceFile_WritesRenderedInstance(t *testing.T) {
	base := t.TempDir()
	mustExtractScaffolding(t, base)

	if err := WriteInstanceFile(base, "my-work", ""); err != nil {
		t.Fatalf("WriteInstanceFile() error = %v", err)
	}

	assertFileModeAndNotEmpty(t, InstanceFilePath(base, "my-work"), 0644)
	content := readScaffoldingFile(t, base, "instance-my-work.tf")
	assertContainsAll(t, "instance-my-work.tf", content, `resource "aws_instance" "my-work"`)
}

func TestWriteInstanceFile_RefusesToOverwriteAnExistingInstance(t *testing.T) {
	base := t.TempDir()
	mustExtractScaffolding(t, base)
	mustWriteInstanceFile(t, base, "my-work")

	err := WriteInstanceFile(base, "my-work", "")

	if err == nil {
		t.Fatal("WriteInstanceFile() returned nil error for an existing instance file")
	}
	want := "instance-my-work.tf already exists; run isolarium destroy --type ec2 --name my-work first"
	if err.Error() != want {
		t.Errorf("WriteInstanceFile() error = %q, want %q", err.Error(), want)
	}
}

func TestRemoveInstanceFile_DeletesTheFileAndToleratesItsAbsence(t *testing.T) {
	base := t.TempDir()
	mustExtractScaffolding(t, base)
	mustWriteInstanceFile(t, base, "my-work")

	if err := RemoveInstanceFile(base, "my-work"); err != nil {
		t.Fatalf("RemoveInstanceFile() error = %v", err)
	}
	if _, err := os.Stat(InstanceFilePath(base, "my-work")); !os.IsNotExist(err) {
		t.Errorf("instance-my-work.tf still exists after RemoveInstanceFile()")
	}

	if err := RemoveInstanceFile(base, "my-work"); err != nil {
		t.Errorf("RemoveInstanceFile() on an absent file error = %v, want nil", err)
	}
}

func TestInstanceFileExists_ReportsPresence(t *testing.T) {
	base := t.TempDir()
	mustExtractScaffolding(t, base)

	if InstanceFileExists(base, "my-work") {
		t.Error("InstanceFileExists() = true before the instance file was written")
	}

	mustWriteInstanceFile(t, base, "my-work")

	if !InstanceFileExists(base, "my-work") {
		t.Error("InstanceFileExists() = false after the instance file was written")
	}
}

func TestListInstanceNames_ReturnsSortedNamesIgnoringScaffolding(t *testing.T) {
	base := t.TempDir()
	mustExtractScaffolding(t, base)
	mustWriteInstanceFile(t, base, "zulu")
	mustWriteInstanceFile(t, base, "alpha")

	names, err := ListInstanceNames(base)

	if err != nil {
		t.Fatalf("ListInstanceNames() error = %v", err)
	}
	if got := strings.Join(names, ","); got != "alpha,zulu" {
		t.Errorf("ListInstanceNames() = %q, want %q", got, "alpha,zulu")
	}
}

func TestListInstanceNames_ReturnsNothingWhenTheDirectoryIsAbsent(t *testing.T) {
	names, err := ListInstanceNames(filepath.Join(t.TempDir(), "never-created"))

	if err != nil {
		t.Fatalf("ListInstanceNames() error = %v, want nil for an absent directory", err)
	}
	if len(names) != 0 {
		t.Errorf("ListInstanceNames() = %v, want empty", names)
	}
}

func mustExtractScaffolding(t *testing.T, base string) {
	t.Helper()

	if err := ExtractScaffolding(base); err != nil {
		t.Fatalf("ExtractScaffolding() error = %v", err)
	}
}

func mustWriteInstanceFile(t *testing.T, base, name string) {
	t.Helper()

	if err := WriteInstanceFile(base, name, ""); err != nil {
		t.Fatalf("WriteInstanceFile(%q) error = %v", name, err)
	}
}
