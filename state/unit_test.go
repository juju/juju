package state_test

import (
	. "launchpad.net/gocheck"
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
	s.charm = s.AddTestingCharm(c, "dummy")
	svc, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	s.unit, err = svc.AddUnit()
	c.Assert(err, IsNil)
}

func (s *UnitSuite) TestGetSetPublicAddress(c *C) {
	address, err := s.unit.PublicAddress()
	c.Assert(err, ErrorMatches, `public address of unit "wordpress/0" not found`)
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)
	address, err = s.unit.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s *UnitSuite) TestGetSetPrivateAddress(c *C) {
	address, err := s.unit.PrivateAddress()
	c.Assert(err, ErrorMatches, `private address of unit "wordpress/0" not found`)
	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	address, err = s.unit.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.local")
}

func (s *UnitSuite) TestRefresh(c *C) {
	unit1, err := s.State.Unit(s.unit.Name())
	c.Assert(err, IsNil)

	err = s.unit.SetPrivateAddress("example.local")
	c.Assert(err, IsNil)
	err = s.unit.SetPublicAddress("example.foobar.com")
	c.Assert(err, IsNil)

	address, err := unit1.PrivateAddress()
	c.Assert(err, ErrorMatches, `private address of unit "wordpress/0" not found`)
	address, err = unit1.PublicAddress()
	c.Assert(err, ErrorMatches, `public address of unit "wordpress/0" not found`)

	err = unit1.Refresh()
	c.Assert(err, IsNil)
	address, err = unit1.PrivateAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.local")
	address, err = unit1.PublicAddress()
	c.Assert(err, IsNil)
	c.Assert(address, Equals, "example.foobar.com")
}

func (s *UnitSuite) TestGetSetStatus(c *C) {
	fail := func() { s.unit.SetStatus(state.UnitPending, "") }
	c.Assert(fail, PanicMatches, "unit status must not be set to pending")

	status, info, err := s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitPending)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(state.UnitStarted, "")
	c.Assert(err, IsNil)

	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitDown)
	c.Assert(info, Equals, "")

	p, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	defer func() {
		c.Assert(p.Kill(), IsNil)
	}()

	s.State.StartSync()
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitStarted)
	c.Assert(info, Equals, "")

	err = s.unit.SetStatus(state.UnitError, "test-hook failed")
	c.Assert(err, IsNil)
	status, info, err = s.unit.Status()
	c.Assert(err, IsNil)
	c.Assert(status, Equals, state.UnitError)
	c.Assert(info, Equals, "test-hook failed")
}

func (s *UnitSuite) TestUnitCharm(c *C) {
	_, err := s.unit.Charm()
	c.Assert(err, ErrorMatches, `charm URL of unit "wordpress/0" not found`)

	err = s.unit.SetCharm(s.charm)
	c.Assert(err, IsNil)
	ch, err := s.unit.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, s.charm.URL())

	err = s.unit.EnsureDying()
	c.Assert(err, IsNil)
	err = s.unit.SetCharm(s.charm)
	c.Assert(err, IsNil)
	ch, err = s.unit.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.URL(), DeepEquals, s.charm.URL())

	err = s.unit.EnsureDead()
	c.Assert(err, IsNil)
	err = s.unit.SetCharm(s.charm)
	c.Assert(err, ErrorMatches, `cannot set charm for unit "wordpress/0": not found or not alive`)
}

func (s *UnitSuite) TestPathKey(c *C) {
	c.Assert(s.unit.PathKey(), Equals, "unit-wordpress-0")
}

func (s *UnitSuite) TestUnitSetAgentAlive(c *C) {
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)
	c.Assert(pinger, Not(IsNil))
	defer pinger.Stop()

	s.State.Sync()
	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)
}

func (s *UnitSuite) TestUnitWaitAgentAlive(c *C) {
	timeout := 200 * time.Millisecond
	alive, err := s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)

	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, ErrorMatches, `waiting for agent of unit "wordpress/0": still not alive after timeout`)

	pinger, err := s.unit.SetAgentAlive()
	c.Assert(err, IsNil)

	s.State.StartSync()
	err = s.unit.WaitAgentAlive(timeout)
	c.Assert(err, IsNil)

	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, true)

	err = pinger.Kill()
	c.Assert(err, IsNil)

	s.State.Sync()
	alive, err = s.unit.AgentAlive()
	c.Assert(err, IsNil)
	c.Assert(alive, Equals, false)
}

func (s *UnitSuite) TestGetSetClearResolved(c *C) {
	setting, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)

	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": already resolved`)
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
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": invalid error resolution mode: 999`)
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

func (s *UnitSuite) TestOpenClosePortWhenDying(c *C) {
	testWhenDying(c, s.unit, "", notAliveErr, func() error {
		return s.unit.OpenPort("tcp", 20)
	}, func() error {
		return s.unit.ClosePort("tcp", 20)
	})
}

func (s *UnitSuite) TestSetClearResolvedWhenDying(c *C) {
	testWhenDying(c, s.unit, notAliveErr, notAliveErr, func() error {
		err := s.unit.SetResolved(state.ResolvedNoHooks)
		cerr := s.unit.ClearResolved()
		c.Assert(cerr, IsNil)
		return err
	})
}

func (s *UnitSuite) TestSubordinateChangeInPrincipal(c *C) {
	subCharm := s.AddTestingCharm(c, "logging")
	logService, err := s.State.AddService("logging", subCharm)
	c.Assert(err, IsNil)
	_, err = logService.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)
	su1, err := logService.AddUnitSubordinateTo(s.unit)
	c.Assert(err, IsNil)

	doc := make(map[string][]string)
	s.ConnSuite.units.FindId(s.unit.Name()).One(&doc)
	subordinates, ok := doc["subordinates"]
	if !ok {
		c.Errorf(`unit document does not have a "subordinates" field`)
	}
	c.Assert(subordinates, DeepEquals, []string{"logging/0", "logging/1"})

	err = su1.EnsureDead()
	c.Assert(err, IsNil)
	err = logService.RemoveUnit(su1)
	c.Assert(err, IsNil)
	doc = make(map[string][]string)
	s.ConnSuite.units.FindId(s.unit.Name()).One(&doc)
	subordinates, ok = doc["subordinates"]
	if !ok {
		c.Errorf(`unit document does not have a "subordinates" field`)
	}
	c.Assert(subordinates, DeepEquals, []string{"logging/0"})
}
