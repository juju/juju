package dummy_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func init() {
	config := `
environments:
    only:
        type: dummy
        zookeeper: true
`
	envs, err := environs.ReadEnvironsBytes([]byte(config))
	if err != nil {
		panic(fmt.Errorf("cannot parse testing config: %v", err))
	}
	Suite(&jujutest.LiveTests{
		Environs:     envs,
		Name:         "only",
		CanOpenState: true,
	})
	Suite(&jujutest.Tests{
		Environs: envs,
		Name:     "only",
	})
}

func TestSuite(t *stdtesting.T) {
	testing.ZkTestPackage(t)
}
