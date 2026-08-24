package ec2

import (
	"fmt"
	"strings"
)

const userDataHeredocTag = "USER_DATA"

// RenderInstanceFile produces the whole Terraform description of one
// environment: the instance itself plus the two outputs the metadata file is
// built from. SSH ingress lives in the shared security group, so nothing here
// opens a port.
func RenderInstanceFile(name, userData string) string {
	return fmt.Sprintf(`resource "aws_instance" %q {
  ami                         = data.aws_ssm_parameter.ubuntu_ami.value
  instance_type               = "t3.large"
  key_name                    = aws_key_pair.isolarium.key_name
  vpc_security_group_ids      = [aws_security_group.isolarium.id]
  subnet_id                   = aws_subnet.isolarium.id
  associate_public_ip_address = true
%s
  root_block_device {
    volume_size           = 50
    volume_type           = "gp3"
    encrypted             = true
    delete_on_termination = true
  }

  tags = {
    Name = %q
  }
}

output "instance_id_%s" {
  value = aws_instance.%s.id
}

output "public_dns_%s" {
  value = aws_instance.%s.public_dns
}
`, name, renderUserDataAttribute(userData), name, name, name, name, name)
}

func renderUserDataAttribute(userData string) string {
	if userData == "" {
		return ""
	}
	return fmt.Sprintf("\n  user_data = <<-%s\n%s\n  %s\n",
		userDataHeredocTag, indentHeredocBody(userData), userDataHeredocTag)
}

func indentHeredocBody(userData string) string {
	lines := strings.Split(strings.TrimRight(userData, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
