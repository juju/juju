package ec2

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"launchpad.net/juju/go/juju/jujutest"
)

// register juju tests
func init() {
	envs, err := juju.ReadEnvironsBytes([]byte(`
environments:
  sample:
    type: ec2
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
