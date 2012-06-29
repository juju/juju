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
			return &state.MachinesChange{Deleted: []*state.Machine{state.NewMachine(s, "machine-0000000001")}}
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
