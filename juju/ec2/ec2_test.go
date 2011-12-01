package ec2_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	_ "launchpad.net/juju/go/juju/ec2"
	"launchpad.net/juju/go/juju/jujutest"
	"testing"
)

var test_environments = []byte(`
environments:
  sample:
    type: ec2
`)

func TestEC2(t *testing.T) {
	envs, err := juju.ReadEnvironsBytes(test_environments)
	if err != nil {
		t.Fatalf("cannot read test_environments.yaml: %v", err)
	}
	if envs == nil {
		t.Fatalf("got nil environs with no error")
	}
	for _, name := range envs.Names() {
		Suite(&jujutest.Tests{
			Environs: envs,
			Name:     name,
		})
	}
	TestingT(t)
}
