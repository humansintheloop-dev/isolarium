resource "aws_vpc" "isolarium" {
  cidr_block           = "10.42.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "isolarium"
  }
}

resource "aws_subnet" "isolarium" {
  vpc_id                  = aws_vpc.isolarium.id
  cidr_block              = "10.42.1.0/24"
  map_public_ip_on_launch = true

  tags = {
    Name = "isolarium-public"
  }
}

resource "aws_internet_gateway" "isolarium" {
  vpc_id = aws_vpc.isolarium.id

  tags = {
    Name = "isolarium"
  }
}

resource "aws_route_table" "isolarium" {
  vpc_id = aws_vpc.isolarium.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.isolarium.id
  }

  tags = {
    Name = "isolarium-public"
  }
}

resource "aws_route_table_association" "isolarium" {
  subnet_id      = aws_subnet.isolarium.id
  route_table_id = aws_route_table.isolarium.id
}
