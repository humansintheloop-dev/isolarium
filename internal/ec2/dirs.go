package ec2

import "path/filepath"

func EC2Dir(base string) string {
	return filepath.Join(base, "ec2")
}

func TerraformDir(base string) string {
	return filepath.Join(EC2Dir(base), "terraform")
}

func PrivateKeyPath(base string) string {
	return filepath.Join(EC2Dir(base), "id_ed25519")
}

func PublicKeyPath(base string) string {
	return filepath.Join(EC2Dir(base), "id_ed25519.pub")
}

func KnownHostsPath(base string) string {
	return filepath.Join(EC2Dir(base), "known_hosts")
}

func TfvarsPath(base string) string {
	return filepath.Join(TerraformDir(base), "isolarium.auto.tfvars")
}
