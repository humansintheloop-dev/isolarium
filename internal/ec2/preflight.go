package ec2

import "fmt"

const regionEnvVar = "AWS_REGION"

func RequireRegion(lookupEnv func(string) (string, bool)) (string, error) {
	region, ok := lookupEnv(regionEnvVar)
	if !ok || region == "" {
		return "", fmt.Errorf("%s is not set", regionEnvVar)
	}
	return region, nil
}
