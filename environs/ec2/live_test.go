package ec2_test

import (
	"crypto/rand"
	"fmt"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/jujutest"
)

// integrationConfig holds the environments configuration
// for running the amazon EC2 integration tests.
//
// This is missing keys for security reasons; set the following environment variables
// to make the integration testing work:
//  access-key: $AWS_ACCESS_KEY_ID
//  admin-secret: $AWS_SECRET_ACCESS_KEY
var integrationConfig = `
environments:
  sample:
    type: ec2
    control-bucket: '%s'
`

func registerIntegrationTests() {
	cfg := fmt.Sprintf(integrationConfig, bucketName)
	envs, err := environs.ReadEnvironsBytes([]byte(cfg))
	if err != nil {
		panic(fmt.Errorf("cannot parse integration tests config data: %v", err))
	}
	for _, name := range envs.Names() {
		Suite(&jujutest.LiveTests{
			Environs: envs,
			Name:     name,
		})
	}
}

func bucketName() string {
	buf := make([]byte, 8)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Sprintf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("juju-test-%x", buf)
}
