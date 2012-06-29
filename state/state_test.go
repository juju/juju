package state_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/testing"
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
	testing.StateSuite
	zkConn *zookeeper.Conn
}

func (s *ConnSuite) SetUpTest(c *C) {
	s.StateSuite.SetUpTest(c)
	s.zkConn = state.ZkConn(s.St)
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
	path, err := s.zkConn.Create("/environment", "type: dummy\nname: foo\n", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
	c.Assert(path, Equals, "/environment")

	env, err := s.St.EnvironConfig()
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
	// create a blank /environment key manually as it is
	// not created by state.Initialize().
	path, err := s.zkConn.Create("/environment", "", 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	c.Assert(err, IsNil)
	c.Assert(path, Equals, "/environment")

	// fetch the /environment key as a *ConfigNode
	environConfigWatcher := s.St.WatchEnvironConfig()
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
			c.Fatalf("didn't get change: %#v", test.want)
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
	dummy, err := s.St.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, curl.String())
	children, _, err := s.zkConn.Children("/charms")
	c.Assert(err, IsNil)
	c.Assert(children, DeepEquals, []string{"local_3a_series_2f_dummy-1"})
}

func (s *StateSuite) TestMissingCharms(c *C) {
	// Check that getting a nonexistent charm fails.
	curl := charm.MustParseURL("local:series/random-99")
	_, err := s.St.Charm(curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:series/random-99": .*`)

	// Add a separate charm, test missing charm still missing.
	s.AddTestingCharm(c, "dummy")
	_, err = s.St.Charm(curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:series/random-99": .*`)
}

func (s *StateSuite) TestAddMachine(c *C) {
	machine0, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)
	machine1, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine1.Id(), Equals, 1)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000000", "machine-0000000001"})
}

func (s *StateSuite) TestRemoveMachine(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.St.AddMachine()
	c.Assert(err, IsNil)
	err = s.St.RemoveMachine(machine.Id())
	c.Assert(err, IsNil)

	children, _, err := s.zkConn.Children("/machines")
	c.Assert(err, IsNil)
	sort.Strings(children)
	c.Assert(children, DeepEquals, []string{"machine-0000000001"})

	// Removing a non-existing machine has to fail.
	err = s.St.RemoveMachine(machine.Id())
	c.Assert(err, ErrorMatches, "can't remove machine 0: machine not found")
}

func (s *StateSuite) TestReadMachine(c *C) {
	machine, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.St.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

func (s *StateSuite) TestReadNonExistentMachine(c *C) {
	_, err := s.St.Machine(0)
	c.Assert(err, ErrorMatches, "machine 0 not found")

	_, err = s.St.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.St.Machine(1)
	c.Assert(err, ErrorMatches, "machine 1 not found")
}

func (s *StateSuite) TestAllMachines(c *C) {
	s.AssertMachineCount(c, 0)
	_, err := s.St.AddMachine()
	c.Assert(err, IsNil)
	s.AssertMachineCount(c, 1)
	_, err = s.St.AddMachine()
	c.Assert(err, IsNil)
	s.AssertMachineCount(c, 2)
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
	machineWatcher := s.St.WatchMachines()
	defer func() {
		c.Assert(machineWatcher.Stop(), IsNil)
	}()

	for i, test := range machinesWatchTests {
		c.Logf("test %d", i)
		err := test.test(s.St)
		c.Assert(err, IsNil)
		want := test.want(s.St)
		select {
		case got, ok := <-machineWatcher.Changes():
			c.Assert(ok, Equals, true)
			c.Assert(got, DeepEquals, want)
		case <-time.After(200 * time.Millisecond):
			c.Fatalf("didn't get change: %#v", want)
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
	wordpress, err := s.St.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	mysql, err := s.St.AddService("mysql", charm)
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.St.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	url, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, charm.URL().String())
	mysql, err = s.St.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	url, err = mysql.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, charm.URL().String())
}

func (s *StateSuite) TestRemoveService(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	service, err := s.St.AddService("wordpress", charm)
	c.Assert(err, IsNil)

	// Remove of existing service.
	err = s.St.RemoveService(service)
	c.Assert(err, IsNil)
	_, err = s.St.Service("wordpress")
	c.Assert(err, ErrorMatches, `can't get service "wordpress": service with name "wordpress" not found`)

	// Remove of an illegal service, it has already been removed.
	err = s.St.RemoveService(service)
	c.Assert(err, ErrorMatches, `can't remove service "wordpress": can't get all units from service "wordpress": environment state has changed`)
}

func (s *StateSuite) TestReadNonExistentService(c *C) {
	_, err := s.St.Service("pressword")
	c.Assert(err, ErrorMatches, `can't get service "pressword": service with name "pressword" not found`)
}

func (s *StateSuite) TestAllServices(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	services, err := s.St.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 0)

	// Check that after adding services the result is ok.
	_, err = s.St.AddService("wordpress", charm)
	c.Assert(err, IsNil)
	services, err = s.St.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 1)

	_, err = s.St.AddService("mysql", charm)
	c.Assert(err, IsNil)
	services, err = s.St.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), Equals, "wordpress")
	c.Assert(services[1].Name(), Equals, "mysql")
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
