resource "aws_security_group" "isolarium" {
  name        = "isolarium"
  description = "SSH from the isolarium host only; unrestricted egress"
  vpc_id      = aws_vpc.isolarium.id

  ingress {
    description = "SSH from the detected host address"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ingress_cidr]
  }

  egress {
    description      = "Unrestricted egress"
    from_port        = 0
    to_port          = 0
    protocol         = "-1"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = {
    Name = "isolarium"
  }
}
