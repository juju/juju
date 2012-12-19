package openstack_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/openstack"
)

func registerLocalTests() {
	Suite(&LocalSuite{})
}

type LocalSuite struct {
}

func (s *LocalSuite) SetUpSuite(c *C) {
	openstack.UseTestMetadata(true)
	openstack.ShortTimeouts(true)
}

func (s *LocalSuite) TearDownSuite(c *C) {
	openstack.UseTestMetadata(false)
	openstack.ShortTimeouts(false)
}

//TODO(wallyworld) - add any necessary tests
