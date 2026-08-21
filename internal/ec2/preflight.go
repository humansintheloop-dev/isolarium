package ec2

import "fmt"

const regionEnvVar = "AWS_REGION"

// defaultEnvFile names where the caller is expected to keep AWS credentials, so
// the failure tells them the file to edit rather than just the variable.
const defaultEnvFile = ".env.local"

func RequireRegion(lookupEnv func(string) (string, bool)) (string, error) {
	region, ok := lookupEnv(regionEnvVar)
	if !ok || region == "" {
		return "", fmt.Errorf("%s is required for --type ec2; set it in %s", regionEnvVar, defaultEnvFile)
	}
	return region, nil
}
