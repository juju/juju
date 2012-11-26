package openstack_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/openstack"
	coretesting "launchpad.net/juju-core/testing"
	"testing"
)

func init() {
}

type LocalSuite struct {
	env environs.Environ
}

var _ = Suite(&LocalSuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *LocalSuite) SetUpSuite(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "test",
		"type":            "openstack",
		"authorized-keys": "foo",
		"ca-cert":         coretesting.CACertPEM,
		"ca-private-key":  coretesting.CAKeyPEM,
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
