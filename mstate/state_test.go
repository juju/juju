package mstate_test

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs/config"
	state "launchpad.net/juju-core/mstate"
	"launchpad.net/juju-core/testing"
	"net/url"
	"sort"
	"time"
)

type D []bson.DocElem

type StateSuite struct {
	ConnSuite
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) TestDialAgain(c *C) {
	// Ensure idempotent operations on Dial are working fine.
	for i := 0; i < 2; i++ {
		st, err := state.Dial(testing.MgoAddr)
		c.Assert(err, IsNil)
		c.Assert(st.Close(), IsNil)
	}
}

func (s *StateSuite) TestAddCharm(c *C) {
	// Check that adding charms from scratch works correctly.
	ch := testing.Charms.Dir("series", "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:series/%s-%d", ch.Meta().Name, ch.Revision()),
	)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	dummy, err := s.State.AddCharm(ch, curl, bundleURL, "dummy-1-sha256")
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, curl.String())

	doc := state.CharmDoc{}
	err = s.charms.FindId(curl).One(&doc)
	c.Assert(err, IsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, DeepEquals, curl)
}

func (s *StateSuite) AssertMachineCount(c *C, expect int) {
	ms, err := s.State.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

func (s *StateSuite) TestAddMachine(c *C) {
	machine0, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)
	machine1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine1.Id(), Equals, 1)

	machines := s.AllMachines(c)
	c.Assert(machines, DeepEquals, []int{0, 1})
}

func (s *StateSuite) TestRemoveMachine(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	err = machine.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveMachine(machine.Id())
	c.Assert(err, IsNil)

	machines := s.AllMachines(c)
	c.Assert(machines, DeepEquals, []int{1})

	// Removing a non-existing machine has to fail.
	// BUG(aram): use error strings from state.
	err = s.State.RemoveMachine(machine.Id())
	c.Assert(err, ErrorMatches, "cannot remove machine 0: .*")
}

func (s *StateSuite) TestReadMachine(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.State.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

func (s *StateSuite) TestAllMachines(c *C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		err := s.machines.Insert(D{{"_id", i}, {"life", state.Alive}})
		c.Assert(err, IsNil)
	}
	s.AssertMachineCount(c, numInserts)
	ms, _ := s.State.AllMachines()
	for k, v := range ms {
		c.Assert(v.Id(), Equals, k)
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
	url, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, charm.URL().String())
	mysql, err = s.State.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	url, err = mysql.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, charm.URL().String())
}

func (s *StateSuite) TestRemoveService(c *C) {
	charm := s.AddTestingCharm(c, "dummy")
	service, err := s.State.AddService("wordpress", charm)
	c.Assert(err, IsNil)

	// Remove of existing service.
	err = service.Die()
	c.Assert(err, IsNil)
	err = s.State.RemoveService(service)
	c.Assert(err, IsNil)
	_, err = s.State.Service("wordpress")
	c.Assert(err, ErrorMatches, `cannot get service "wordpress": .*`)

	// Remove of an invalid service, it has already been removed.
	// BUG(aram): use error strings from state.
	err = s.State.RemoveService(service)
	c.Assert(err, ErrorMatches, `cannot remove service "wordpress": .*`)
}

func (s *StateSuite) TestReadNonExistentService(c *C) {
	// BUG(aram): use error strings from state.
	_, err := s.State.Service("pressword")
	c.Assert(err, ErrorMatches, `cannot get service "pressword": .*`)
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

func (s *StateSuite) TestEnvironConfig(c *C) {
	initial := map[string]interface{}{
		"name":            "test",
		"type":            "test",
		"authorized-keys": "i-am-a-key",
		"default-series":  "precise",
		"development":     true,
	}
	env, err := config.New(initial)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(env)
	c.Assert(err, IsNil)
	env, err = s.State.EnvironConfig()
	c.Assert(err, IsNil)
	current := env.AllAttrs()
	c.Assert(current, DeepEquals, initial)

	current["authorized-keys"] = "i-am-a-new-key"
	env, err = config.New(current)
	c.Assert(err, IsNil)
	err = s.State.SetEnvironConfig(env)
	c.Assert(err, IsNil)
	env, err = s.State.EnvironConfig()
	c.Assert(err, IsNil)
	final := env.AllAttrs()
	c.Assert(final, DeepEquals, current)
}

var machinesWatchTests = []struct {
	test    func(*C, *state.State)
	added   []int
	removed []int
}{
	{
		test: func(c *C, s *state.State) {
			_, err := s.AddMachine()
			c.Assert(err, IsNil)
		},
		added: []int{0},
	},
	{
		test: func(c *C, s *state.State) {
			_, err := s.AddMachine()
			c.Assert(err, IsNil)
		},
		added: []int{1},
	},
	{
		test: func(c *C, s *state.State) {
			_, err := s.AddMachine()
			c.Assert(err, IsNil)
			_, err = s.AddMachine()
			c.Assert(err, IsNil)
		},
		added: []int{2, 3},
	},
	{
		test: func(c *C, s *state.State) {
			m3, err := s.Machine(3)
			c.Assert(err, IsNil)
			err = m3.Die()
			c.Assert(err, IsNil)
			err = s.RemoveMachine(3)
			c.Assert(err, IsNil)
		},
		removed: []int{3},
	},
	{
		test: func(c *C, s *state.State) {
			m0, err := s.Machine(0)
			c.Assert(err, IsNil)
			err = m0.Die()
			c.Assert(err, IsNil)
			err = s.RemoveMachine(0)
			c.Assert(err, IsNil)
			m2, err := s.Machine(2)
			c.Assert(err, IsNil)
			err = m2.Die()
			c.Assert(err, IsNil)
			err = s.RemoveMachine(2)
			c.Assert(err, IsNil)
		},
		removed: []int{0, 2},
	},
	{
		test: func(c *C, s *state.State) {
			_, err := s.AddMachine()
			c.Assert(err, IsNil)
			m1, err := s.Machine(1)
			c.Assert(err, IsNil)
			err = m1.Die()
			c.Assert(err, IsNil)
			err = s.RemoveMachine(1)
			c.Assert(err, IsNil)
		},
		added:   []int{4},
		removed: []int{1},
	},
	{
		test: func(c *C, s *state.State) {
			machines := [20]*state.Machine{}
			var err error
			for i := 0; i < len(machines); i++ {
				machines[i], err = s.AddMachine()
				c.Assert(err, IsNil)
			}
			for i := 0; i < len(machines); i++ {
				err = machines[i].SetInstanceId("spam" + fmt.Sprint(i))
				c.Assert(err, IsNil)
			}
			for i := 10; i < len(machines); i++ {
				err = machines[i].Die()
				c.Assert(err, IsNil)
				err = s.RemoveMachine(i + 5)
				c.Assert(err, IsNil)
			}
		},
		added: []int{5, 6, 7, 8, 9, 10, 11, 12, 13, 14},
	},
	{
		test: func(c *C, s *state.State) {
			_, err := s.AddMachine()
			c.Assert(err, IsNil)
			m9, err := s.Machine(9)
			c.Assert(err, IsNil)
			err = m9.Die()
			c.Assert(err, IsNil)
			err = s.RemoveMachine(9)
			c.Assert(err, IsNil)
		},
		added:   []int{25},
		removed: []int{9},
	},
}

func (s *StateSuite) TestWatchMachines(c *C) {
	machineWatcher := s.State.WatchMachines()
	defer func() {
		c.Assert(machineWatcher.Stop(), IsNil)
	}()
	for i, test := range machinesWatchTests {
		c.Logf("test %d", i)
		test.test(c, s.State)
		s.State.StartSync()
		got := &state.MachinesChange{}
		for {
			select {
			case new, ok := <-machineWatcher.Changes():
				c.Assert(ok, Equals, true)
				addMachineChanges(got, new)
				if moreMachinesRequired(got, test.added, test.removed) {
					continue
				}
				assertSameMachines(c, got, test.added, test.removed)
			case <-time.After(500 * time.Millisecond):
				c.Fatalf("did not get change, want: added: %#v, removed: %#v, got: %#v", test.added, test.removed, got)
			}
			break
		}
	}
	select {
	case got := <-machineWatcher.Changes():
		c.Fatalf("got unexpected change: %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func moreMachinesRequired(got *state.MachinesChange, added, removed []int) bool {
	return len(got.Added)+len(got.Removed) < len(added)+len(removed)
}

func addMachineChanges(changes *state.MachinesChange, more *state.MachinesChange) {
	changes.Added = append(changes.Added, more.Added...)
	changes.Removed = append(changes.Removed, more.Removed...)
}

type machineSlice []*state.Machine

func (m machineSlice) Len() int           { return len(m) }
func (m machineSlice) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m machineSlice) Less(i, j int) bool { return m[i].Id() < m[j].Id() }

func assertSameMachines(c *C, got *state.MachinesChange, added, removed []int) {
	c.Assert(got, NotNil)
	c.Assert(len(got.Added), Equals, len(added))
	c.Assert(len(got.Removed), Equals, len(removed))

	sort.Sort(machineSlice(got.Added))
	sort.Sort(machineSlice(got.Removed))
	for i, g := range got.Added {
		id := added[i]
		c.Assert(g.Id(), Equals, id)
	}
	for i, g := range got.Removed {
		id := removed[i]
		c.Assert(g.Id(), Equals, id)
	}
}
