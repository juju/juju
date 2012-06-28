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
	stdtesting "testing"
)

func Test(t *stdtesting.T) { TestingT(t) }

var _ = Suite(&StateSuite{})

type StateSuite struct {
	MgoSuite
	session  *mgo.Session
	charms   *mgo.Collection
	machines *mgo.Collection
	services *mgo.Collection
	units    *mgo.Collection
	st       *state.State
	ch       charm.Charm
	curl     *charm.URL
}

func (s *StateSuite) SetUpTest(c *C) {
	s.MgoSuite.SetUpTest(c)
	session, err := mgo.Dial(mgoaddr)
	c.Assert(err, IsNil)
	s.session = session

	st, err := state.Dial(mgoaddr)
	c.Assert(err, IsNil)
	s.st = st

	s.charms = session.DB("juju").C("charms")
	s.machines = session.DB("juju").C("machines")
	s.services = session.DB("juju").C("services")
	s.units = session.DB("juju").C("units")

	s.ch = testing.Charms.Dir("dummy")
	url := fmt.Sprintf("local:series/%s-%d", s.ch.Meta().Name, s.ch.Revision())
	s.curl = charm.MustParseURL(url)
}

func (s *StateSuite) TearDownTest(c *C) {
	s.st.Close()
	s.session.Close()
	s.MgoSuite.TearDownTest(c)
}

func (s *StateSuite) TestAddCharm(c *C) {
	// Check that adding charms works correctly.
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.st.AddCharm(s.ch, s.curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())

	mdoc := &struct {
		URL *charm.URL `bson:"_id"`
	}{}
	err = s.charms.Find(bson.D{{"_id", s.curl}}).One(mdoc)
	c.Assert(err, IsNil)
	c.Assert(mdoc.URL, DeepEquals, s.curl)
}

// addDummyCharm adds the 'dummy' charm state to st.
func (s *StateSuite) addDummyCharm(c *C) *state.Charm {
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	dummy, err := s.st.AddCharm(s.ch, s.curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	return dummy
}

func (s *StateSuite) TestCharmAttributes(c *C) {
	// Check that the basic (invariant) fields of the charm
	// are correctly in place.
	s.addDummyCharm(c)

	dummy, err := s.st.Charm(s.curl)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	c.Assert(dummy.Revision(), Equals, 1)
	bundleURL, err := url.Parse("http://bundle.url")
	c.Assert(err, IsNil)
	c.Assert(dummy.BundleURL(), DeepEquals, bundleURL)
	c.Assert(dummy.BundleSha256(), Equals, "dummy-sha256")
	meta := dummy.Meta()
	c.Assert(meta.Name, Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}

func (s *StateSuite) TestNonExistentCharmPriorToInitialization(c *C) {
	// Check that getting a charm before any other charm has been added fails nicely.
	_, err := s.st.Charm(s.curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:series/dummy-1": .*`)
}

func (s *StateSuite) TestGetNonExistentCharm(c *C) {
	// Check that getting a non-existent charm fails nicely.
	s.addDummyCharm(c)

	curl := charm.MustParseURL("local:anotherseries/dummy-1")
	_, err := s.st.Charm(curl)
	c.Assert(err, ErrorMatches, `can't get charm "local:anotherseries/dummy-1": .*`)
}

func (s *StateSuite) assertMachineCount(c *C, expect int) {
	ms, err := s.st.AllMachines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, expect)
}

func (s *StateSuite) TestAllMachines(c *C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		err := s.machines.Insert(bson.D{{"_id", i}})
		c.Assert(err, IsNil)
	}
	s.assertMachineCount(c, numInserts)
	ms, _ := s.st.AllMachines()
	for k, v := range ms {
		c.Assert(v.Id(), Equals, k)
	}
}

func (s *StateSuite) TestAddMachine(c *C) {
	numInserts := 42
	for i := 0; i < numInserts; i++ {
		m, err := s.st.AddMachine()
		c.Assert(err, IsNil)
		c.Assert(m.Id(), Equals, i)
	}
	s.assertMachineCount(c, numInserts)
}

func (s *StateSuite) TestRemoveMachine(c *C) {
	m0, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	m1, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = s.st.RemoveMachine(m0.Id())
	c.Assert(err, IsNil)
	s.assertMachineCount(c, 1)
	ms, err := s.st.AllMachines()
	c.Assert(ms[0].Id(), Equals, m1.Id())

	// Removing a non-existing machine has to fail.
	err = s.st.RemoveMachine(m0.Id())
	c.Assert(err, ErrorMatches, "can't remove machine 0")
}

func (s *StateSuite) TestMachineInstanceId(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = s.machines.Update(bson.D{{"_id", machine.Id()}}, bson.D{{"instanceid", "spaceship/0"}})
	c.Assert(err, IsNil)

	iid, err := machine.InstanceId()
	c.Assert(err, IsNil)
	c.Assert(iid, Equals, "spaceship/0")
}

func (s *StateSuite) TestMachineSetInstanceId(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = machine.SetInstanceId("umbrella/0")
	c.Assert(err, IsNil)

	n, err := s.machines.Find(bson.D{{"instanceid", "umbrella/0"}}).Count()
	c.Assert(err, IsNil)
	c.Assert(n, Equals, 1)
}

func (s *StateSuite) TestReadMachine(c *C) {
	machine, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	expectedId := machine.Id()
	machine, err = s.st.Machine(expectedId)
	c.Assert(err, IsNil)
	c.Assert(machine.Id(), Equals, expectedId)
}

func (s *StateSuite) TestMachineUnits(c *C) {
	// Check that Machine.Units works correctly.

	// Make three machines, three services and three units for each service;
	// variously assign units to machines and check that Machine.Units
	// tells us the right thing.

	m0, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	m1, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	m2, err := s.st.AddMachine()
	c.Assert(err, IsNil)

	dummy := s.addDummyCharm(c)
	logging := addLoggingCharm(c, s.st)
	s0, err := s.st.AddService("s0", dummy)
	c.Assert(err, IsNil)
	s1, err := s.st.AddService("s1", dummy)
	c.Assert(err, IsNil)
	s2, err := s.st.AddService("s2", dummy)
	c.Assert(err, IsNil)
	s3, err := s.st.AddService("s3", logging)
	c.Assert(err, IsNil)

	units := make([][]*state.Unit, 4)
	for i, svc := range []*state.Service{s0, s1, s2} {
		units[i] = make([]*state.Unit, 3)
		for j := range units[i] {
			units[i][j], err = svc.AddUnit()
			c.Assert(err, IsNil)
		}
	}
	// Add the logging units subordinate to the s2 units.
	units[3] = make([]*state.Unit, 3)
	for i := range units[3] {
		units[3][i], err = s3.AddUnitSubordinateTo(units[2][i])
	}

	assignments := []struct {
		machine      *state.Machine
		units        []*state.Unit
		subordinates []*state.Unit
	}{
		{m0, []*state.Unit{units[0][0]}, nil},
		{m1, []*state.Unit{units[0][1], units[1][0], units[1][1], units[2][0]}, []*state.Unit{units[3][0]}},
		{m2, []*state.Unit{units[2][2]}, []*state.Unit{units[3][2]}},
	}

	for _, a := range assignments {
		for _, u := range a.units {
			err := u.AssignToMachine(a.machine)
			c.Assert(err, IsNil)
		}
	}

	mdocs := []struct {
		Id         int `bson:"_id"`
		InstanceId string
		UnitSet    string
	}{}
	udocs := []struct {
		Name    string `bson:"_id"`
		Service string
		UnitSet string
	}{}
	s.session.DB("juju").C("units").Find(nil).All(&udocs)
	s.session.DB("juju").C("machines").Find(nil).All(&mdocs)
	c.Logf("\n\n%+v\n\n%+v\n\n", udocs, mdocs)

	for i, a := range assignments {
		c.Logf("test %d", i)
		got, err := a.machine.Units()
		c.Assert(err, IsNil)
		expect := sortedUnitNames(append(a.units, a.subordinates...))
		c.Assert(sortedUnitNames(got), DeepEquals, expect)
	}
}

func sortedUnitNames(units []*state.Unit) []string {
	names := make([]string, len(units))
	for i, u := range units {
		names[i] = u.Name()
	}
	sort.Strings(names)
	return names
}

func (s *StateSuite) TestAddService(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	mysql, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")

	// Check that retrieving the new created services works correctly.
	wordpress, err = s.st.Service("wordpress")
	c.Assert(err, IsNil)
	c.Assert(wordpress.Name(), Equals, "wordpress")
	url, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, s.curl.String())
	mysql, err = s.st.Service("mysql")
	c.Assert(err, IsNil)
	c.Assert(mysql.Name(), Equals, "mysql")
	url, err = mysql.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(url.String(), Equals, s.curl.String())
}

func (s *StateSuite) TestReadNonExistentService(c *C) {
	_, err := s.st.Service("pressword")
	c.Assert(err, ErrorMatches, `can't get service "pressword": .*`)
}

func (s *StateSuite) TestRemoveService(c *C) {
	dummy := s.addDummyCharm(c)
	service, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Remove of existing service.
	err = s.st.RemoveService(service)
	c.Assert(err, IsNil)
	_, err = s.st.Service("wordpress")
	c.Assert(err, ErrorMatches, `can't get service "wordpress": .*`)
}

func (s *StateSuite) TestAllServices(c *C) {
	services, err := s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 0)

	// Check that after adding services the result is ok.
	dummy := s.addDummyCharm(c)
	_, err = s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	services, err = s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 1)

	_, err = s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	services, err = s.st.AllServices()
	c.Assert(err, IsNil)
	c.Assert(len(services), Equals, 2)

	// Check the returned service, order is defined by sorted keys.
	c.Assert(services[0].Name(), Equals, "wordpress")
	c.Assert(services[1].Name(), Equals, "mysql")
}

func (s *StateSuite) TestAddUnit(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that principal units can be added on their own.
	unitZero, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitZero.Name(), Equals, "wordpress/0")
	principal := unitZero.IsPrincipal()
	c.Assert(principal, Equals, true)
	unitOne, err := wordpress.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitOne.Name(), Equals, "wordpress/1")
	principal = unitOne.IsPrincipal()
	c.Assert(principal, Equals, true)

	// Check that principal units cannot be added to principal units.
	_, err = wordpress.AddUnitSubordinateTo(unitZero)
	c.Assert(err, ErrorMatches, `can't add unit of principal service "wordpress" as a subordinate of "wordpress/0"`)

	// Assign the principal unit to a machine.
	m, err := s.st.AddMachine()
	c.Assert(err, IsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, IsNil)

	// Add a subordinate service.
	subCh := addLoggingCharm(c, s.st)
	logging, err := s.st.AddService("logging", subCh)
	c.Assert(err, IsNil)

	// Check that subordinate units can be added to principal units
	subZero, err := logging.AddUnitSubordinateTo(unitZero)
	c.Assert(err, IsNil)
	c.Assert(subZero.Name(), Equals, "logging/0")
	principal = subZero.IsPrincipal()
	c.Assert(principal, Equals, false)

	// Check the subordinate unit has been assigned its principal's machine.
	id, err := subZero.AssignedMachineId()
	c.Assert(err, IsNil)
	c.Assert(id, Equals, m.Id())

	// Check that subordinate units must be added to other units.
	_, err = logging.AddUnit()
	c.Assert(err, ErrorMatches, `cannot directly add units to subordinate service "logging"`)

	// Check that subordinate units cannnot be added to subordinate units.
	_, err = logging.AddUnitSubordinateTo(subZero)
	c.Assert(err, ErrorMatches, "a subordinate unit must be added to a principal unit")
}

func (s *StateSuite) TestReadUnit(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	mysql, err := s.st.AddService("mysql", dummy)
	c.Assert(err, IsNil)
	_, err = mysql.AddUnit()
	c.Assert(err, IsNil)

	// Check that retrieving a unit works correctly.
	unit, err := wordpress.Unit("wordpress/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "wordpress/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = wordpress.Unit("wordpress")
	c.Assert(err, ErrorMatches, `can't get unit "wordpress" from service "wordpress": .*`)
	unit, err = wordpress.Unit("wordpress/0/0")
	c.Assert(err, ErrorMatches, `can't get unit "wordpress/0/0" from service "wordpress": .*`)
	unit, err = wordpress.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `can't get unit "pressword/0" from service "wordpress": .*`)
	unit, err = wordpress.Unit("mysql/0")
	c.Assert(err, ErrorMatches, `can't get unit "mysql/0" from service "wordpress": .*`)

	// Check that retrieving all units works.
	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 2)
	c.Assert(units[0].Name(), Equals, "wordpress/0")
	c.Assert(units[1].Name(), Equals, "wordpress/1")
}

func (s *StateSuite) TestServiceCharm(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)

	// Check that getting and setting the service charm URL works correctly.
	testcurl, err := wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.curl.String())
	testcurl, err = charm.ParseURL("local:myseries/mydummy-1")
	c.Assert(err, IsNil)
	err = wordpress.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = wordpress.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s *StateSuite) TestRemoveUnit(c *C) {
	dummy := s.addDummyCharm(c)
	wordpress, err := s.st.AddService("wordpress", dummy)
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)
	_, err = wordpress.AddUnit()
	c.Assert(err, IsNil)

	// Check that removing a unit works.
	unit, err := wordpress.Unit("wordpress/0")
	c.Assert(err, IsNil)
	err = wordpress.RemoveUnit(unit)
	c.Assert(err, IsNil)

	units, err := wordpress.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 1)
	c.Assert(units[0].Name(), Equals, "wordpress/1")

	// Check that removing a non-existent unit fails nicely.
	err = wordpress.RemoveUnit(unit)
	// TODO use error string from state_test.TestRemoveUnit()
	c.Assert(err, ErrorMatches, `can't remove unit "wordpress/0": .*`)
}

// addLoggingCharm adds a "logging" (subordinate) charm
// to the state.
func addLoggingCharm(c *C, st *state.State) *state.Charm {
	bundle := testing.Charms.Bundle(c.MkDir(), "logging")
	curl := charm.MustParseURL("cs:series/logging-99")
	bundleURL, err := url.Parse("http://subordinate.url")
	c.Assert(err, IsNil)
	ch, err := st.AddCharm(bundle, curl, bundleURL, "dummy-sha256")
	c.Assert(err, IsNil)
	return ch
}
