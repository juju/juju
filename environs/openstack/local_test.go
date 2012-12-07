package openstack_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/openstack"
	coretesting "launchpad.net/juju-core/testing"
	"os"
)

func init() {
	// HEADS UP: Please do not break trunk tests. Before committing changes,
	// make sure that tests in trunk are actually passing against whatever
	// revision of the packages depended upon that are *currently* public,
	// so that other people can continue to rely on trunk for their work.
	//
	// Note that this is just an ugly hack to get trunk building again. Tests
	// shouldn't depend on external variables like this, and should also not
	// hack the environment permanently as done here. It is fine to depend on
	// the environment when running with a flag that enables specific tests
	// though (such as -amazon).
	os.Setenv("OS_AUTH_URL", "PLEASE FIX ME")
	os.Setenv("OS_REGION_NAME", "PLEASE FIX ME")
	os.Setenv("OS_TENANT_NAME", "PLEASE FIX ME")
	os.Setenv("OS_USERNAME", "PLEASE FIX ME")
	os.Setenv("OS_PASSWORD", "PLEASE FIX ME")
}

func registerLocalTests() {
	Suite(&LocalSuite{})
}

type LocalSuite struct {
	env environs.Environ
}

func (s *LocalSuite) SetUpSuite(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "test",
		"type":            "openstack",
		"authorized-keys": "foo",
		"ca-cert":         coretesting.CACert,
		"ca-private-key":  coretesting.CAKey,
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
