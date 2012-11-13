package dummy_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/testing"
	stdtesting "testing"
)

func init() {
	cfg, err := config.New(map[string]interface{}{
		"name": "only",
		"type": "dummy",
		"state-server": true,
		"secret": "pork",
		"admin-secret": "fish",
	})
	if err != nil {
		panic(fmt.Errorf("cannot create testing config: %v", err))
	}
	Suite(&jujutest.LiveTests{
		Config:       cfg,
		CanOpenState:   true,
		HasProvisioner: false,
	})
	Suite(&jujutest.Tests{
		Config:       cfg,
	})
}

func TestSuite(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
