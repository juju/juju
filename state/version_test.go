package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/version"
)

type versioner interface {
	AgentVersion() (version.Number, error)
	SetAgentVersion(v version.Number) error
	ProposedAgentVersion() (version.Number, error)
	ProposeAgentVersion(v version.Number) error
}

var _ = Suite(&VersionSuite{})

type VersionSuite struct {
	ConnSuite
}

func testVersion(c *C, obj versioner, agent string) {
	// object starts with no versions
	_, err := obj.AgentVersion()
	c.Assert(err, ErrorMatches, agent+" agent version not found")
	_, err = obj.ProposedAgentVersion()
	c.Assert(err, ErrorMatches, agent+" agent proposed version not found")

	// check that we can set the version
	err = obj.ProposeAgentVersion(version.Number{5, 6, 7})
	c.Assert(err, IsNil)
	v, err := obj.ProposedAgentVersion()
	c.Assert(err, IsNil)
	c.Assert(v, Equals, version.Number{5, 6, 7})

	err = obj.SetAgentVersion(version.Number{3, 4, 5})
	c.Assert(err, IsNil)
	v, err = obj.AgentVersion()
	c.Assert(err, IsNil)
	c.Assert(v, Equals, version.Number{3, 4, 5})

	// check there's no cross-talk
	v, err = obj.ProposedAgentVersion()
	c.Assert(err, IsNil)
	c.Assert(v, Equals, version.Number{5, 6, 7})
}

func (s *VersionSuite) TestMachineVersion(c *C) {
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	testVersion(c, m, "machine")
}

func (s *VersionSuite) TestUnitVersion(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	unit, err := svc.AddUnit()
	c.Assert(err, IsNil)
	testVersion(c, unit, "unit")
}
