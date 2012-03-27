package dummy_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	_ "launchpad.net/juju/go/environs/dummy"
	"launchpad.net/juju/go/environs/jujutest"
	"testing"
)

func init() {
	config := `
environments:
    only:
        type: dummy
        zookeeper: false
`
	envs, err := environs.ReadEnvironsBytes([]byte(config))
	if err != nil {
		panic(fmt.Errorf("cannot parse testing config: %v", err))
	}
	Suite(&jujutest.LiveTests{
		Environs: envs,
		Name:     "only",
	})
	Suite(&jujutest.Tests{
		Environs: envs,
		Name:     "only",
	})
}

func TestSuite(t *testing.T) {
	TestingT(t)
}
