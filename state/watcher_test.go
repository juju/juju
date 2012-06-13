package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/state"
	"time"
)

var serviceWatchConfigData = []map[string]interface{}{
	{},
	{"foo": "bar", "baz": "yadda"},
	{"baz": "yadda"},
}

func (s *StateSuite) TestServiceWatchConfig(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")

	config, err := wordpress.Config()
	c.Assert(err, IsNil)
	c.Assert(config.Keys(), HasLen, 0)
	configWatcher := wordpress.WatchConfig()

	// Two change events.
	config.Set("foo", "bar")
	config.Set("baz", "yadda")
	changes, err := config.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []state.ItemChange{{
		Key:      "baz",
		Type:     state.ItemAdded,
		NewValue: "yadda",
	}, {
		Key:      "foo",
		Type:     state.ItemAdded,
		NewValue: "bar",
	}})
	time.Sleep(100 * time.Millisecond)
	config.Delete("foo")
	changes, err = config.Write()
	c.Assert(err, IsNil)
	c.Assert(changes, DeepEquals, []state.ItemChange{{
		Key:      "foo",
		Type:     state.ItemDeleted,
		OldValue: "bar",
	}})

	for _, want := range serviceWatchConfigData {
		select {
		case got, ok := <-configWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got.Map(), DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got, _ := <-configWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err = configWatcher.Stop()
	c.Assert(err, IsNil)
}

func (s *StateSuite) TestServiceWatchConfigIllegalData(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	configWatcher := wordpress.WatchConfig()

	// Receive empty change after service adding.
	select {
	case got, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, true)
		c.Assert(got.Map(), DeepEquals, map[string]interface{}{})
	case <-time.After(100 * time.Millisecond):
		c.Fatalf("unexpected timeout")
	}

	// Set config to illegal data.
	_, err = s.zkConn.Set("/services/service-0000000000/config", "---", -1)
	c.Assert(err, IsNil)

	select {
	case _, ok := <-configWatcher.Changes():
		c.Assert(ok, Equals, false)
	case <-time.After(100 * time.Millisecond):
	}

	err = configWatcher.Stop()
	c.Assert(err, ErrorMatches, "YAML error: .*")
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

func (s *StateSuite) TestUnitWatchNeedsUpgrade(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	needsUpgradeWatcher := unit.WatchNeedsUpgrade()

	for i, test := range unitWatchNeedsUpgradeTests {
		c.Logf("test %d", i)
		err := test.test(unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-needsUpgradeWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", test.want)
		}
	}

	select {
	case got, _ := <-needsUpgradeWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err = needsUpgradeWatcher.Stop()
	c.Assert(err, IsNil)
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

func (s *StateSuite) TestUnitWatchResolved(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	resolvedWatcher := unit.WatchResolved()

	for i, test := range unitWatchResolvedTests {
		c.Logf("test %d", i)
		err := test.test(unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-resolvedWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, Equals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", test.want)
		}
	}

	select {
	case got, _ := <-resolvedWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err = resolvedWatcher.Stop()
	c.Assert(err, IsNil)
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

func (s *StateSuite) TestUnitWatchPorts(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	unit, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	portsWatcher := unit.WatchPorts()

	for i, test := range unitWatchPortsTests {
		c.Logf("test %d", i)
		err := test.test(unit)
		c.Assert(err, IsNil)
		select {
		case got, ok := <-portsWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", test.want)
		}
	}

	select {
	case got, _ := <-portsWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	err = portsWatcher.Stop()
	c.Assert(err, IsNil)
}

type machinesWatchTest struct {
	test func(*state.State) error
	want func(*state.State) *state.MachinesChange
}

var machinesWatchTests = []machinesWatchTest{
	{
		func(s *state.State) error {
			_, err := s.AddMachine()
			return err
		},
		func(s *state.State) *state.MachinesChange {
			return &state.MachinesChange{Added: []*state.Machine{state.NewMachine(s, "machine-0000000000")}}
		},
	},
	{
		func(s *state.State) error {
			_, err := s.AddMachine()
			return err
		},
		func(s *state.State) *state.MachinesChange {
			return &state.MachinesChange{Added: []*state.Machine{state.NewMachine(s, "machine-0000000001")}}
		},
	},
	{
		func(s *state.State) error {
			return s.RemoveMachine(1)
		},
		func(s *state.State) *state.MachinesChange {
			return &state.MachinesChange{Deleted: []*state.Machine{state.NewMachine(s, "machine-0000000001")}}
		},
	},
}

func (s *StateSuite) TestWatchMachines(c *C) {
	w := s.st.WatchMachines()

	for i, test := range machinesWatchTests {
		c.Logf("test %d", i)
		err := test.test(s.st)
		c.Assert(err, IsNil)
		want := test.want(s.st)
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
		}
	}

	select {
	case got, _ := <-w.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	c.Assert(w.Stop(), IsNil)
}

var watchMachineUnitsTests = []struct {
	test func(m *state.Machine, units []*state.Unit) error
	want func(units []*state.Unit) *state.MachineUnitsChange
} {
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[0].AssignToMachine(m)
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Added: []*state.Unit{units[0], units[1]}}
		},
	},
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[2].AssignToMachine(m)
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Added: []*state.Unit{units[2]}}
		},
	},
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[0].UnassignFromMachine()
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Deleted: []*state.Unit{units[0], units[1]}}
		},
	},
	{
		func(m *state.Machine, units []*state.Unit) error {
			return units[2].UnassignFromMachine()
		},
		func(units []*state.Unit) *state.MachineUnitsChange {
			return &state.MachineUnitsChange{Deleted: []*state.Unit{units[2]}}
		},
	},
}

func (s *StateSuite) TestWatchMachineUnits(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	subCh := addLoggingCharm(c, s.st)
	logging, err := s.st.AddService("logging", subCh)
	c.Assert(err, IsNil)

	units := make([]*state.Unit, 3)
	units[0], err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	units[1], err = logging.AddUnitSubordinateTo(units[0])
	c.Assert(err, IsNil)
	units[2], err = wordpress.AddUnit()
	c.Assert(err, IsNil)

	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	w := m.WatchUnits()

	for i, test := range watchMachineUnitsTests {
		c.Logf("test %d", i)
		err := test.test(m, units)
		c.Assert(err, IsNil)
		want := test.want(units)
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(unitNames(got.Added), DeepEquals, unitNames(want.Added))
			c.Assert(unitNames(got.Deleted), DeepEquals, unitNames(want.Deleted))
		case <-time.After(500 * time.Millisecond):
			c.Fatalf("didn't get change: %v", want)
		}
	}

	select {
	case got, _ := <-w.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	c.Assert(w.Stop(), IsNil)
}

func unitNames(units []*state.Unit) (s []string) {
	for _, u := range units {
		s = append(s, u.Name())
	}
	return
}

type any map[string]interface{}

var environmentWatchTests = []struct {
	key   string
	value interface{}
	want  map[string]interface{}
}{
	{"provider", "dummy", any{"provider": "dummy"}},
	{"secret", "shhh", any{"provider": "dummy", "secret": "shhh"}},
	{"provider", "aws", any{"provider": "aws", "secret": "shhh"}},
}

func (s *StateSuite) TestWatchEnvironment(c *C) {
	// create a blank /environment key manually as it is 
	// not created by state.Init().
	path, err := s.zkConn.Create("/environment", "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
	c.Assert(path, Equals, "/environment")

	// fetch the /environment key as a *ConfigNode
	w := s.st.WatchEnvironConfig()
	config, ok := <-w.Changes()
	c.Assert(ok, Equals, true)

	for i, test := range environmentWatchTests {
		c.Logf("test %d", i)
		config.Set(test.key, test.value)
		_, err := config.Write()
		c.Assert(err, IsNil)
		select {
		case got, ok := <-w.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got.Map(), DeepEquals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", test.want)
		}
	}

	select {
	case got, _ := <-w.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}

	c.Assert(w.Stop(), IsNil)
}

var diffTests = []struct {
	A, B, want []string
}{
	{[]string{"A", "B", "C"}, []string{"A", "D", "C"}, []string{"B"}},
	{[]string{"A", "B", "C"}, []string{"C", "B", "A"}, nil},
	{[]string{"A", "B", "C"}, []string{"B"}, []string{"A", "C"}},
	{[]string{"B"}, []string{"A", "B", "C"}, nil},
	{[]string{"A", "D", "C"}, []string{}, []string{"A", "D", "C"}},
	{[]string{}, []string{"A", "D", "C"}, nil},
}

func (*StateSuite) TestDiff(c *C) {
	for _, test := range diffTests {
		c.Assert(test.want, DeepEquals, state.Diff(test.A, test.B))
	}
}
