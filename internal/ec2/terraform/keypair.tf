resource "aws_key_pair" "isolarium" {
  key_name   = "isolarium"
  public_key = var.public_key

  tags = {
    Name = "isolarium"
  }
}
