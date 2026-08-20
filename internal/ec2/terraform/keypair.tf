# The name is generated rather than fixed because a fixed one makes rotation
# unsafe: AWS refuses two key pairs with the same name, so Terraform has to
# delete the old one before creating the replacement, and it schedules that
# deletion after the instances that reference the key pair have already been
# launched — with the superseded key baked into their authorized_keys. A
# generated name lets the replacement be created first, so every instance is
# launched with the key the host actually holds.
resource "aws_key_pair" "isolarium" {
  key_name_prefix = "isolarium-"
  public_key      = var.public_key

  lifecycle {
    create_before_destroy = true
  }

  tags = {
    Name = "isolarium"
  }
}
