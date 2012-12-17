package openstack_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/openstack"
)

func registerLocalTests() {
	Suite(&LocalSuite{})
}

type LocalSuite struct {
	env environs.Environ
}

func (s *LocalSuite) SetUpSuite(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":        "test",
		"type":        "openstack",
		"username":    "testuser",
		"password":    "secret",
		"tenant-name": "sometenant",
		"region":      "someregion",
		"auth-method": "userpass",
		"auth-url":    "http://somehost",
	})
	c.Assert(err, IsNil)
	s.env = env
	openstack.UseTestMetadata(true)
	openstack.ShortTimeouts(true)
}

func (s *LocalSuite) TearDownSuite(c *C) {
	openstack.UseTestMetadata(false)
	openstack.ShortTimeouts(false)
	s.env = nil
}

func (s *LocalSuite) TestPrivateAddress(c *C) {
	p := s.env.Provider()
	addr, err := p.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "private.dummy.address.example.com")
}

func (s *LocalSuite) TestPublicAddress(c *C) {
	p := s.env.Provider()
	addr, err := p.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(addr, Equals, "public.dummy.address.example.com")
}
