package mstate_test

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/mstate"
	coretesting "launchpad.net/juju-core/testing"
	"net/url"
	stdtesting "testing"
)

func Test(t *stdtesting.T) { TestingT(t) }

type StateSuite struct {
	ConnSuite
}

var _ = Suite(&StateSuite{})

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
		err := s.machines.Insert(bson.D{{"_id", i}, {"life", state.Alive}})
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
