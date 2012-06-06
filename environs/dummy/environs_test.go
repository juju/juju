package dummy_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/environs"
	"launchpad.net/juju-core/juju/environs/dummy"
	"launchpad.net/juju-core/juju/environs/jujutest"
	"launchpad.net/juju-core/juju/testing"
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
	srv := testing.StartZkServer()
	defer srv.Destroy()
	dummy.SetZookeeper(srv)
	defer dummy.SetZookeeper(nil)
	TestingT(t)
}
