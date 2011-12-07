package ec2

import (
	"fmt"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/ec2/ec2test"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"launchpad.net/juju/go/juju/jujutest"
)

func registerJujuFunctionalTests() {
	srv, err := ec2test.NewServer()
	if err != nil {
		panic(fmt.Errorf("cannot start ec2 test server: %v", err))
	}
	Regions["test"] = aws.Region{
		EC2Endpoint: srv.Address(),
	}
	envs, err := juju.ReadEnvironsBytes([]byte(`
environments:
  sample:
    type: ec2
    region: test
`))
	if err != nil {
		panic(fmt.Errorf("cannot read test_environments.yaml: %v", err))
	}
	if envs == nil {
		panic(fmt.Errorf("got nil environs with no error"))
	}
	for _, name := range envs.Names() {
		Suite(&jujutest.Tests{
			Environs: envs,
			Name:     name,
		})
	}
}

// integration_test_environments holds the environments configuration
// for running the amazon EC2 integration tests.
//
// This is missing keys for security reasons; set the following environment variables
// to make the integration testing work:
//  access-key: $AWS_ACCESS_KEY_ID
//  admin-secret: $AWS_SECRET_ACCESS_KEY
var integration_test_environments = []byte(`
environments:
  sample:
    type: ec2
`)

func registerJujuIntegrationTests() {
	envs, err := juju.ReadEnvironsBytes(integration_test_environments)
	if err != nil {
		panic(fmt.Errorf("cannot read integration_test_environments.yaml: %v", err))
	}
	for _, name := range envs.Names() {
		Suite(&jujutest.Tests{
			Environs: envs,
			Name:     name,
		})
	}
}
