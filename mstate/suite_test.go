package mstate_test

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/mstate"
	"launchpad.net/juju-core/testing"
	"net/url"
	"sort"
)

// ConnSuite facilitates access to the underlying MongoDB.
// It is embedded in UtilSuite.
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

func (s *ConnSuite) AllMachines(c *C) []int {
	docs := []state.MachineDoc{}
	err := s.machines.Find(bson.D{{"life", state.Alive}}).All(&docs)
	c.Assert(err, IsNil)
	ids := []int{}
	for _, v := range docs {
		ids = append(ids, v.Id)
	}
	sort.Ints(ids)
	return ids
}

// UtilSuite provides the infrastructure for all other 
// test suites (StateSuite, CharmSuite, MachineSuite, etc).
type UtilSuite struct {
	MgoSuite
	ConnSuite
	State *state.State
}

func (s *UtilSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	st, err := state.Dial(mgoaddr)
	c.Assert(err, IsNil)
	s.State = st
}

func (s *UtilSuite) TearDownTest(c *C) {
	s.State.Close()
	s.ConnSuite.TearDownTest(c)
}

func (s *UtilSuite) AddTestingCharm(c *C, name string) *state.Charm {
	ch := testing.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := s.State.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}
