package testing_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/environs/jujutest"
	_ "launchpad.net/juju/go/testing"
	stdtesting "testing"
)

func init() {
	config := `
environments:
    only:
        type: testing
        name: foo
`
	envs, err := environs.ReadEnvironsBytes([]byte(config))
	if err != nil {
		panic(fmt.Errorf("cannot parse testing config: %v", err))
	}
	Suite(jujutest.LiveTests{
		Environs: envs,
		Name:     "only",
	})
	Suite(jujutest.Tests{
		Environs: envs,
		Name:     "only",
	})
}

func TestSuite(t *stdtesting.T) {
	TestingT(t)
}
