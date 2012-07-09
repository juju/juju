package mstate_test

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/mstate"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	"sort"
	stdtesting "testing"
)

func Test(t *stdtesting.T) { TestingT(t) }

// ConnSuite facilitates access to the underlying MongoDB. It is embedded
// in other suites, like StateSuite.
type ConnSuite struct {
	MgoSuite
	session  *mgo.Session
	charms   *mgo.Collection
	machines *mgo.Collection
	services *mgo.Collection
	units    *mgo.Collection
}

func (cs *ConnSuite) SetUpTest(c *C) {
	cs.MgoSuite.SetUpTest(c)
	session, err := mgo.Dial(mgoaddr)
	c.Assert(err, IsNil)
	cs.session = session
	cs.charms = session.DB("juju").C("charms")
	cs.machines = session.DB("juju").C("machines")
	cs.services = session.DB("juju").C("services")
	cs.units = session.DB("juju").C("units")
}

func (cs *ConnSuite) TearDownTest(c *C) {
	cs.session.Close()
	cs.MgoSuite.TearDownTest(c)
}

func (s *ConnSuite) AllMachines(c *C) []string {
	docs := []state.MachineDoc{}
	err := s.machines.Find(bson.D{{"life", state.Alive}}).All(&docs)
	c.Assert(err, IsNil)
	names := []string{}
	for _, v := range docs {
		names = append(names, v.String())
	}
	sort.Strings(names)
	return names
}

type StateSuite struct {
	ConnSuite
	State *state.State
}

var _ = Suite(&StateSuite{})

func (s *StateSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	st, err := state.Dial(mgoaddr)
	c.Assert(err, IsNil)
	s.State = st
}

func (s *StateSuite) TearDownTest(c *C) {
	s.State.Close()
	s.ConnSuite.TearDownTest(c)
}

func (s *StateSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := coretesting.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.State.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
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

	doc := state.CharmDoc{}
	err = s.charms.FindId(curl).One(&doc)
	c.Assert(err, IsNil)
	c.Logf("%#v", doc)
	c.Assert(doc.URL, DeepEquals, curl)
}

func (s *StateSuite) TestAddMachine(c *C) {
	machine0, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine0.Id(), Equals, 0)
	machine1, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	c.Assert(machine1.Id(), Equals, 1)

	machines := s.AllMachines(c)
	c.Assert(machines, DeepEquals, []string{"machine-0000000000", "machine-0000000001"})
}

func (s *StateSuite) TestRemoveMachine(c *C) {
	machine, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	_, err = s.State.AddMachine()
	c.Assert(err, IsNil)
	err = s.State.RemoveMachine(machine.Id())
	c.Assert(err, IsNil)

	machines := s.AllMachines(c)
	c.Assert(machines, DeepEquals, []string{"machine-0000000001"})

	// Removing a non-existing machine has to fail.
	err = s.State.RemoveMachine(machine.Id())
	c.Assert(err, ErrorMatches, "can't remove machine 0: .*")
}