package openstack_test

import (
	"flag"
	. "launchpad.net/gocheck"
	"launchpad.net/goose/identity"
	"testing"
)

var live = flag.Bool("live", false, "Include live OpenStack (Canonistack) tests")

// TODO(wallyworld): local tests should always be run but at the moment, some fail as the code is still WIP.
var local = flag.Bool("local", false, "Include local OpenStack (service double) tests")

func Test(t *testing.T) {
	if *live {
		cred, err := identity.CompleteCredentialsFromEnv()
		if err != nil {
			t.Fatalf("Error setting up test suite: %s", err.Error())
		}
		registerOpenStackTests(cred)
	}
	if *local {
		registerServiceDoubleTests()
	}
	TestingT(t)
}
