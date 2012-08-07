package juju_test

import (
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
)

var _ = Suite(&UnexposeSuite{})

type UnexposeSuite struct {
	DeploySuite
}

func (s *UnexposeSuite) SetUpTest(c *C) {
	s.DeploySuite.SetUpTest(c)
}

func (s *UnexposeSuite) TearDownTest(c *C) {
	s.DeploySuite.TearDownTest(c)
}

func (s *UnexposeSuite) TestUnexposeService(c *C) {
	curl := testing.Charms.ClonedURL(s.repo.Path, "riak")
	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)

	_, err = s.conn.AddService("testriak", sch)
	c.Assert(err, IsNil)

	err = s.conn.Expose("testriak")
	c.Assert(err, IsNil)

	err = s.conn.Unexpose("testriak")
	c.Assert(err, IsNil)

	err = s.conn.Unexpose("unknown-service")
	c.Assert(err, ErrorMatches, `.*service with name "unknown-service" not found`)
}
