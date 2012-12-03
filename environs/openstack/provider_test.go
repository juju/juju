package openstack_test

import (
	"flag"
	. "launchpad.net/gocheck"
	"testing"
)

var live = flag.Bool("live", false, "Include live OpenStack (Canonistack) tests")

func Test(t *testing.T) {
	if *live {
		registerOpenStackTests()
	}
	registerLocalTests()
	TestingT(t)
}
