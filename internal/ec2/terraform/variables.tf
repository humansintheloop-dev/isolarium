variable "ingress_cidr" {
  type        = string
  description = "The single /32 permitted to reach SSH on every isolarium instance."
}

variable "public_key" {
  type        = string
  description = "OpenSSH-formatted public half of the host-side Ed25519 keypair."
}

variable "region" {
  type        = string
  description = "AWS region hosting the isolarium network and instances."
}
