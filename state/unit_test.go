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

func (s *UnitSuite) TestUnitCharm(c *C) {
	testcurl, err := s.unit.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.charm.URL().String())

	// TODO BUG surely we shouldn't be able to (1) set a charm that isn't in state
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
	c.Assert(err, ErrorMatches, `cannot inform unit "wordpress/0" about upgrade: upgrade already enabled`)
}

func (s *UnitSuite) TestGetSetClearResolved(c *C) {
	setting, err := s.unit.Resolved()
	c.Assert(err, IsNil)
	c.Assert(setting, Equals, state.ResolvedNone)

	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, IsNil)
	err = s.unit.SetResolved(state.ResolvedNoHooks)
	c.Assert(err, ErrorMatches, `cannot set resolved mode for unit "wordpress/0": flag already set`)
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

type unitWatchNeedsUpgradeTest struct {
	test func(*state.Unit) error
	want state.NeedsUpgrade
}

var unitWatchNeedsUpgradeTests = []unitWatchNeedsUpgradeTest{
	{func(u *state.Unit) error { return nil }, state.NeedsUpgrade{false, false}},
	{func(u *state.Unit) error { return u.SetNeedsUpgrade(false) }, state.NeedsUpgrade{true, false}},
	{func(u *state.Unit) error { return u.ClearNeedsUpgrade() }, state.NeedsUpgrade{false, false}},
	{func(u *state.Unit) error { return u.SetNeedsUpgrade(true) }, state.NeedsUpgrade{true, true}},
}

func (s *UnitSuite) TestUnitWatchNeedsUpgrade(c *C) {
	needsUpgradeWatcher := s.unit.WatchNeedsUpgrade()
	defer func() {
		c.Assert(needsUpgradeWatcher.Stop(), IsNil)
	}()

	for i, test := range unitWatchNeedsUpgradeTests {
		c.Logf("test %d", i)
		err := test.test(s.unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-needsUpgradeWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got := <-needsUpgradeWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

type unitWatchResolvedTest struct {
	test func(*state.Unit) error
	want state.ResolvedMode
}

var unitWatchResolvedTests = []unitWatchResolvedTest{
	{func(u *state.Unit) error { return nil }, state.ResolvedNone},
	{func(u *state.Unit) error { return u.SetResolved(state.ResolvedRetryHooks) }, state.ResolvedRetryHooks},
	{func(u *state.Unit) error { return u.ClearResolved() }, state.ResolvedNone},
	{func(u *state.Unit) error { return u.SetResolved(state.ResolvedNoHooks) }, state.ResolvedNoHooks},
}

func (s *UnitSuite) TestUnitWatchResolved(c *C) {
	resolvedWatcher := s.unit.WatchResolved()
	defer func() {
		c.Assert(resolvedWatcher.Stop(), IsNil)
	}()

	for i, test := range unitWatchResolvedTests {
		c.Logf("test %d", i)
		err := test.test(s.unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-resolvedWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got := <-resolvedWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

type unitWatchPortsTest struct {
	test func(*state.Unit) error
	want []state.Port
}

var unitWatchPortsTests = []unitWatchPortsTest{
	{func(u *state.Unit) error { return nil }, nil},
	{func(u *state.Unit) error { return u.OpenPort("tcp", 80) }, []state.Port{{"tcp", 80}}},
	{func(u *state.Unit) error { return u.OpenPort("udp", 53) }, []state.Port{{"tcp", 80}, {"udp", 53}}},
	{func(u *state.Unit) error { return u.ClosePort("tcp", 80) }, []state.Port{{"udp", 53}}},
}

func (s *UnitSuite) TestUnitWatchPorts(c *C) {
	portsWatcher := s.unit.WatchPorts()
	defer func() {
		c.Assert(portsWatcher.Stop(), IsNil)
	}()

	for i, test := range unitWatchPortsTests {
		c.Logf("test %d", i)
		err := test.test(s.unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-portsWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got := <-portsWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}
