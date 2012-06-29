package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"time"
)

type UnitSuite struct {
	ConnSuite
	charm *state.Charm
	unit  *state.Unit
}

var _ = Suite(&UnitSuite{})

func (s *UnitSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.Charm(c, "dummy")
	svc, err := s.St.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	s.unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestGetSetPublicAddress(c *C) {
	address, err := s.unit.PublicAddress()
	c.Assert(err, ErrorMatches, "unit has no public address")
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, err = s.unit.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s *UnitSuite) TestGetSetPrivateAddress(c *C) {
	address, err := s.unit.PrivateAddress()
	c.Assert(err, ErrorMatches, "unit has no private address")
	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, err = s.unit.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.local")
}

func (s *UnitSuite) TestUnitCharm(c *C) {
	testcurl, err := s.unit.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.charm.URL().String())

	// TODO surely we shouldn't be able to (1) set a charm that isn't in state
	// or (2) change a unit to run a charm that bears no apparent relation to
	// it service?
	testcurl, err = charm.ParseURL("local:myseries/mydummy-1")
	c.Assert(err, IsNil)
	err = s.unit.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = s.unit.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s *UnitSuite) TestGetSetClearUnitUpgrade(c *C) {
	// Defaults to false and false.
	needsUpgrade, err := s.unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{false, false})

	// Can be set.
	err = s.unit.SetNeedsUpgrade(false)
	c.Assert(err, IsNil)
	needsUpgrade, err = s.unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, false})

	// Can be set multiple times.
	err = s.unit.SetNeedsUpgrade(false)
	c.Assert(err, IsNil)
	needsUpgrade, err = s.unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, false})

	// Can be cleared.
	err = s.unit.ClearNeedsUpgrade()
	c.Assert(err, IsNil)
	needsUpgrade, err = s.unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{false, false})

	// Can be cleared multiple times
	err = s.unit.ClearNeedsUpgrade()
	c.Assert(err, IsNil)
	needsUpgrade, err = s.unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{false, false})

	// Can be set forced.
	err = s.unit.SetNeedsUpgrade(true)
	c.Assert(err, IsNil)
	needsUpgrade, err = s.unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, true})

	// Can be set forced multiple times.
	err = s.unit.SetNeedsUpgrade(true)
	c.Assert(err, IsNil)
	needsUpgrade, err = s.unit.NeedsUpgrade()
	c.Assert(err, IsNil)
	c.Assert(needsUpgrade, DeepEquals, &state.NeedsUpgrade{true, true})

	// Can't be set multipe with different force flag.
	err = s.unit.SetNeedsUpgrade(false)
	c.Assert(err, ErrorMatches, `can't inform unit "wordpress/0" about upgrade: upgrade already enabled`)
}

func (s *UnitSuite) TestGetSetClearResolved(c *C) {
	setting, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)

	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `can't set resolved mode for unit "wordpress/0": flag already set`)
	retry, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(retry, Equals, state.ResolvedNoHooks)

	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)
	setting, err = s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)
	err = s.unit.ClearResolved()
	c.Assert(err, IsNil)

	err = s.unit.SetResolved(state.ResolvedMode(999))
	c.Assert(err, ErrorMatches, `can't set resolved mode for unit "wordpress/0": invalid error resolution mode: 999`)
}

func (s *UnitSuite) TestGetOpenPorts(c *C) {
	// Verify no open ports before activity.
	open, err := s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, HasLen, 0)

	// Now open and close port.
	err = s.unit.OpenPort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
	})

	err = s.unit.OpenPort("udp", 53)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
	})

	err = s.unit.OpenPort("tcp", 53)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
	})

	err = s.unit.OpenPort("tcp", 443)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"tcp", 80},
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})

	err = s.unit.ClosePort("tcp", 80)
	c.Assert(err, IsNil)
	open, err = s.unit.OpenPorts()
	c.Assert(err, IsNil)
	c.Assert(open, DeepEquals, []state.Port{
		{"udp", 53},
		{"tcp", 53},
		{"tcp", 443},
	})
}

func (s *UnitSuite) TestUnitSetAgentAlive(c *C) {
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Kill()

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *UnitSuite) TestUnitWaitAgentAlive(c *C) {
	timeout := 5 * time.Second
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of unit "wordpress/0": presence: still not alive after timeout`)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))

	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	pinger.Kill()

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}
