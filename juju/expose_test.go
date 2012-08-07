package juju_test

import (
	. "launchpad.net/gocheck"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
)

var _ = Suite(&ExposeSuite{})

type ExposeSuite struct {
	DeploySuite
}

func (s *ExposeSuite) SetUpTest(c *C) {
	s.DeploySuite.SetUpTest(c)
}

func (s *ExposeSuite) TearDownTest(c *C) {
	s.DeploySuite.TearDownTest(c)
}

func (s *ExposeSuite) TestExposeService(c *C) {
	curl := testing.Charms.ClonedURL(s.repo.Path, "riak")
	sch, err := s.conn.PutCharm(curl, s.repo.Path, false)
	c.Assert(err, IsNil)

	_, err = s.conn.AddService("testriak", sch)
	c.Assert(err, IsNil)

	err = s.conn.Expose("testriak")
	c.Assert(err, IsNil)

	err = s.conn.Expose("unknown-service")
	c.Assert(err, ErrorMatches, `.*service with name "unknown-service" not found`)
}
