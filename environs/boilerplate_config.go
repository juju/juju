package environs

import (
	"crypto/rand"
	"fmt"
	"io"
	"strings"
)

func randomKey() string {
	buf := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

const (
	adminSecretKey   = "${admin-secret}"
	controlBucketKey = "${control-bucket}"
)

func generateAdminSecretValues(envConfig string) string {
	for strings.Contains(envConfig, adminSecretKey) {
		envConfig = strings.Replace(envConfig, adminSecretKey, randomKey(), 1)
	}
	return envConfig
}

func generateControlBucketSecretValues(envConfig string) string {
	for strings.Contains(envConfig, controlBucketKey) {
		envConfig = strings.Replace(envConfig, controlBucketKey, "juju-"+randomKey(), 1)
	}
	return envConfig
}

// BoilerPlateConfig returns a sample juju configuration which is written to a boilerplate environments.yaml file.
func BoilerPlateConfig() string {
	config := `## This is the Juju config file, which you can use to specify multiple environments in which to deploy.
## By default we ship AWS (default), HP Cloud, OpenStack.
## See http://juju.ubuntu.com/docs for more information

## An environment configuration must always specify at least the following information:
##
## - name (to identify the environment)
## - type (to specify the provider)
## - admin-secret (an arbitrary "password" identifying an client with administrative-level access to system state)

## Values in <brackets> below need to be filled in by the user.

default: amazon
environments:
`
	for _, p := range providers {
		providerConfig := p.BoilerPlateConfig()
		if providerConfig != "" {
			config += providerConfig
		}
	}
	config += "\n"

	config = generateAdminSecretValues(config)
	config = generateControlBucketSecretValues(config)
	return config
}
