package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	"sort"
	stdtesting "testing"
	"time"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

// ConnSuite is a testing.StateSuite with direct access to the
// State's underlying zookeeper.Conn.
// TODO: Separate test methods that use zkConn into a single ZooKeeper-
// specific suite, and use plain StateSuites elsewhere, so we can maybe
// eventually have a single set of tests that work against both state and
// mstate.
type ConnSuite struct {
	testing.JujuConnSuite
	zkConn *zookeeper.Conn
}

func (s *ConnSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.zkConn = state.ZkConn(s.State)
}

type StateSuite struct {
	ConnSuite
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) TestInitialize(c *C) {
	// Check that initialization of an already-initialized state succeeds.
	st, err := state.Initialize(s.StateInfo(c))
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	st.Close()

	// Check that Open blocks until Initialize has succeeded.
	coretesting.ZkRemoveTree(s.zkConn, "/")

	errc := make(chan error)
	go func() {
		st, err := state.Open(s.StateInfo(c))
		errc <- err
		if st != nil {
			st.Close()
		}
	}()

	// Wait a little while to verify that it's actually blocking.
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-errc:
		c.Fatalf("state.Open did not block (returned error %v)", err)
	default:
	}

	st, err = state.Initialize(s.StateInfo(c))
	c.Assert(err, IsNil)
	c.Assert(st, NotNil)
	defer st.Close()

	select {
	case err := <-errc:
		c.Assert(err, IsNil)
	case <-time.After(1e9):
		c.Fatalf("state.Open blocked forever")
	}
}

func (s *StateSuite) TestEnvironConfig(c *C) {
	_, err := s.zkConn.Set("/environment", "type: dummy\nname: foo\n", -1)
	c.Assert(err, IsNil)

	env, err := s.State.EnvironConfig()
	env.Read()
	c.Assert(err, IsNil)
	c.Assert(env.Map(), DeepEquals, map[string]interface{}{"type": "dummy", "name": "foo"})
}

type environConfig map[string]interface{}

var environmentWatchTests = []struct {
	key   string
	value interface{}
	want  map[string]interface{}
}{
	{"provider", "dummy", environConfig{"provider": "dummy"}},
	{"secret", "shhh", environConfig{"provider": "dummy", "secret": "shhh"}},
	{"provider", "aws", environConfig{"provider": "aws", "secret": "shhh"}},
}

func (s *StateSuite) TestWatchEnvironment(c *C) {
	// Blank out the environment created by JujuConnSuite,
	// so that we know what we have.
	_, err := s.zkConn.Set("/environment", "", -1)
	c.Assert(err, IsNil)

	// fetch the /environment key as a *ConfigNode
	environConfigWatcher := s.State.WatchEnvironConfig()
	defer func() {
		c.Assert(environConfigWatcher.Stop(), IsNil)
	}()

	config, ok := <-environConfigWatcher.Changes()
	c.Assert(ok, Equals, true)

	for i, test := range environmentWatchTests {
		c.Logf("test %d", i)
		config.Set(test.key, test.value)
		_, err := config.Write()
		c.Assert(err, IsNil)
		select {
		case got, ok := <-environConfigWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got.Map(), DeepEquals, test.want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", test.want)
		}
	}

	select {
	case got := <-environConfigWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *StateSuite) TestAddCharm(c *C) {
	// Check that adding charms from scratch works correctly.
	ch := coretesting.Charms.Dir("dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, curl.String())
	children, _, err := s.zkConn.Children("/charms")
	c.Assert(err, IsNil)
	c.Assert(children, DeepEquals, []string{"local_3a_series_2f_dummy-1"})
}

func (s *StateSuite) TestMissingCharms(c *C) {
	// Check that getting a nonexistent charm fails.
	curl := charm.MustParseURL("local:series/random-99")
	_, err := s.State.Charm(curl)
	c.Assert(err, ErrorMatches, `cannot get charm "local:series/random-99": .*`)

	// Add a separate charm, test missing charm still missing.
	s.AddTestingCharm(c, "dummy")
	_, err = s.State.Charm(curl)
	c.Assert(err, ErrorMatches, `cannot get charm "local:series/random-99": .*`)
}

func (s *StateSuite) TestAddMachine(c *C) {
	machine0, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)
	machine1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine1.Id(), Equals, 1)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000000", "machine-0000000001"})
}

func (s *StateSuite) TestRemoveMachine(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.State.RemoveMachine(machine.Id())
	c.Assert(err, IsNil)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000001"})

	// Removing a non-existing machine has to fail.
	err = s.State.RemoveMachine(machine.Id())
	c.Assert(err, ErrorMatches, "cannot remove machine 0: machine not found")
}

func (s *StateSuite) TestReadMachine(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.State.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

func (s *StateSuite) TestReadNonExistentMachine(c *C) {
	_, err := s.State.Machine(0)
	c.Assert(err, ErrorMatches, "machine 0 not found")

	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.State.Machine(1)
	c.Assert(err, ErrorMatches, "machine 1 not found")
}

func (s *StateSuite) TestAllMachines(c *C) {
	assertMachineCount(c, s.State, 0)
	_, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	assertMachineCount(c, s.State, 1)
	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	assertMachineCount(c, s.State, 2)
}

type machinesWatchTest struct {
	test func(*state.State) error
	want func(*state.State) *state.MachinesChange
}

var machinesWatchTests = []machinesWatchTest{
	{
		func(_ *state.State) error {
			return nil
		},
		func(_ *state.State) *state.MachinesChange {
			return &state.MachinesChange{}
		},
	},
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
			return &state.MachinesChange{Removed: []*state.Machine{state.NewMachine(s, "machine-0000000001")}}
		},
	},
}

func (s *StateSuite) TestWatchMachines(c *C) {
	machineWatcher := s.State.WatchMachines()
	defer func() {
		c.Assert(machineWatcher.Stop(), IsNil)
	}()

	for i, test := range machinesWatchTests {
		c.Logf("test %d", i)
		err := test.test(s.State)
		c.Assert(err, IsNil)
		want := test.want(s.State)
		select {
		case got, ok := <-machineWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", want)
		}
	}

	select {
	case got := <-machineWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *StateSuite) TestAddService(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	wordpress, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	mysql, err := s.State.AddService("mysql", charm)
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.State.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	wch, force, err := wordpress.Charm()
	c.Assert(err, IsNil)
	c.Assert(wch.URL(), DeepEquals, charm.URL())
	c.Assert(force, Equals, false)

	mysql, err = s.State.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	mch, force, err := mysql.Charm()
	c.Assert(err, IsNil)
	c.Assert(mch.URL(), DeepEquals, charm.URL())
	c.Assert(force, Equals, false)
}

func (s *StateSuite) TestRemoveService(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	service, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)

	// Remove of existing service.
	err = s.State.RemoveService(service)
	c.Assert(err, IsNil)
	_, err = s.State.Service("wordpress")
	c.Assert(err, ErrorMatches, `cannot get service "wordpress": service with name "wordpress" not found`)

	// Remove of an illegal service, it has already been removed.
	err = s.State.RemoveService(service)
	c.Assert(err, ErrorMatches, `cannot remove service "wordpress": cannot get relations for service "wordpress": environment state has changed`)
}

func (s *StateSuite) TestReadNonExistentService(c *C) {
	_, err := s.State.Service("pressword")
	c.Assert(err, ErrorMatches, `cannot get service "pressword": service with name "pressword" not found`)
}

func (s *StateSuite) TestAllServices(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	services, err := s.State.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 0)

	// Check that after adding services the result is ok.
	_, err = s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	services, err = s.State.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 1)

	_, err = s.State.AddService("mysql", charm)
	c.Assert(err, IsNil)
	services, err = s.State.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), Equals, "wordpress")
	c.Assert(services[1].Name(), Equals, "mysql")
}

var serviceWatchTests = []struct {
	testOp string
	name   string
	idx    int
}{
	{"none", "", 0},
	{"add", "wordpress", 0},
	{"add", "mysql", 1},
	{"remove", "wordpress", 0},
}

func (s *StateSuite) TestWatchServices(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	servicesWatcher := s.State.WatchServices()
	defer func() {
		c.Assert(servicesWatcher.Stop(), IsNil)
	}()
	services := make([]*state.Service, 2)

	for i, test := range serviceWatchTests {
		c.Logf("test %d", i)
		var want *state.ServicesChange
		switch test.testOp {
		case "none":
			want = &state.ServicesChange{}
		case "add":
			var err error
			services[test.idx], err = s.State.AddService(test.name, charm)
			c.Assert(err, IsNil)
			want = &state.ServicesChange{[]*state.Service{services[test.idx]}, nil}
		case "remove":
			service, err := s.State.Service(test.name)
			c.Assert(err, IsNil)
			err = s.State.RemoveService(service)
			c.Assert(err, IsNil)
			want = &state.ServicesChange{nil, []*state.Service{services[test.idx]}}
			services[test.idx] = nil
		}
		select {
		case got, ok := <-servicesWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("did not get change: %#v", want)
		}
	}

	select {
	case got := <-servicesWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
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

var sortPortsTests = []struct {
	have, want []state.Port
}{
	{nil, []state.Port{}},
	{[]state.Port{{"b", 1}, {"a", 99}, {"a", 1}}, []state.Port{{"a", 1}, {"a", 99}, {"b", 1}}},
}

func (*StateSuite) TestSortPorts(c *C) {
	for _, t := range sortPortsTests {
		p := make([]state.Port, len(t.have))
		copy(p, t.have)
		state.SortPorts(p)
		c.Check(p, DeepEquals, t.want)
		state.SortPorts(p)
		c.Check(p, DeepEquals, t.want)
	}
}
