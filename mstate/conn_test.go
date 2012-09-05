package mstate_test

import (
	"fmt"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/mstate"
	"launchpad.net/juju-core/testing"
	"net/url"
	"sort"
	stdtesting "testing"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// ConnSuite provides the infrastructure for all other 
// test suites (StateSuite, CharmSuite, MachineSuite, etc).
type ConnSuite struct {
	testing.CharmSuite
	testing.MgoSuite
	charms    *mgo.Collection
	machines  *mgo.Collection
	relations *mgo.Collection
	services  *mgo.Collection
	units     *mgo.Collection
	State     *state.State
}

func (cs *ConnSuite) SetUpSuite(c *C) {
	cs.CharmSuite.SetUpSuite(c)
	cs.MgoSuite.SetUpSuite(c)
}

func (cs *ConnSuite) TearDownSuite(c *C) {
	cs.MgoSuite.TearDownSuite(c)
	cs.CharmSuite.TearDownSuite(c)
}

func (cs *ConnSuite) SetUpTest(c *C) {
	cs.CharmSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
	cs.charms = cs.MgoSuite.Session.DB("juju").C("charms")
	cs.machines = cs.MgoSuite.Session.DB("juju").C("machines")
	cs.relations = cs.MgoSuite.Session.DB("juju").C("relations")
	cs.services = cs.MgoSuite.Session.DB("juju").C("services")
	cs.units = cs.MgoSuite.Session.DB("juju").C("units")
	var err error
	cs.State, err = state.Dial(testing.MgoAddr)
	c.Assert(err, IsNil)
}

func (cs *ConnSuite) TearDownTest(c *C) {
	cs.State.Close()
	cs.MgoSuite.TearDownTest(c)
	cs.CharmSuite.TearDownTest(c)
}

func (s *ConnSuite) AllMachines(c *C) []int {
	docs := []state.MachineDoc{}
	err := s.machines.Find(D{{"life", state.Alive}}).All(&docs)
	c.Assert(err, IsNil)
	ids := []int{}
	for _, v := range docs {
		ids = append(ids, v.Id)
	}
	sort.Ints(ids)
	return ids
}

func (s *ConnSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := s.CharmDir("series", name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.State.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}
