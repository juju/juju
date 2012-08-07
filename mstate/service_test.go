package mstate_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	state "launchpad.net/juju-core/mstate"
)

type ServiceSuite struct {
	ConnSuite
	charm   *state.Charm
	service *state.Service
}

var _ = Suite(&ServiceSuite{})

func (s *ServiceSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	s.charm = s.AddTestingCharm(c, "dummy")
	var err error
	s.service, err = s.State.AddService("mysql", s.charm)
	c.Assert(err, IsNil)
}

func (s *ServiceSuite) TestServiceCharm(c *C) {
	// Check that getting and setting the service charm URL works correctly.
	testcurl, err := s.service.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.charm.URL().String())

	// TODO BUG https://bugs.launchpad.net/juju-core/+bug/1020318
	testcurl = charm.MustParseURL("local:myseries/mydummy-1")
	err = s.service.SetCharmURL(testcurl)
	c.Assert(err, IsNil)
	testcurl, err = s.service.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s *ServiceSuite) TestServiceRefesh(c *C) {
	s1, err := s.State.Service(s.service.Name())
	c.Assert(err, IsNil)

	newcurl := charm.MustParseURL("local:myseries/mydummy-1")
	err = s.service.SetCharmURL(newcurl)
	c.Assert(err, IsNil)

	testcurl, err := s1.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, s.charm.URL().String())

	err = s1.Refresh()
	testcurl, err = s1.CharmURL()
	c.Assert(err, IsNil)
	c.Assert(testcurl.String(), Equals, "local:myseries/mydummy-1")
}

func (s *ServiceSuite) TestAddUnit(c *C) {
	// Check that principal units can be added on their own.
	unitZero, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitZero.Name(), Equals, "mysql/0")
	principal := unitZero.IsPrincipal()
	c.Assert(principal, Equals, true)
	unitOne, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	c.Assert(unitOne.Name(), Equals, "mysql/1")
	principal = unitOne.IsPrincipal()
	c.Assert(principal, Equals, true)

	// Check that principal units cannot be added to principal units.
	_, err = s.service.AddUnitSubordinateTo(unitZero)
	c.Assert(err, ErrorMatches, `cannot add unit of principal service "mysql" as a subordinate of "mysql/0"`)

	// Assign the principal unit to a machine.
	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = unitZero.AssignToMachine(m)
	c.Assert(err, IsNil)

	// Add a subordinate service.
	subCharm := s.AddTestingCharm(c, "logging")
	logging, err := s.State.AddService("logging", subCharm)
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

func (s *ServiceSuite) TestReadUnit(c *C) {
	_, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	_, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	// Check that retrieving a unit works correctly.
	unit, err := s.service.Unit("mysql/0")
	c.Assert(err, IsNil)
	c.Assert(unit.Name(), Equals, "mysql/0")

	// Check that retrieving a non-existent or an invalidly
	// named unit fail nicely.
	unit, err = s.service.Unit("mysql")
	c.Assert(err, ErrorMatches, `cannot get unit "mysql" from service "mysql":.*`)
	unit, err = s.service.Unit("mysql/0/0")
	c.Assert(err, ErrorMatches, `cannot get unit "mysql/0/0" from service "mysql": .*`)
	unit, err = s.service.Unit("pressword/0")
	c.Assert(err, ErrorMatches, `cannot get unit "pressword/0" from service "mysql": .*`)

	// Add another service to check units are not misattributed.
	mysql, err := s.State.AddService("wordpress", s.charm)
	c.Assert(err, IsNil)
	_, err = mysql.AddUnit()
	c.Assert(err, IsNil)

	// BUG(aram): use error strings from state.
	unit, err = s.service.Unit("wordpress/0")
	c.Assert(err, ErrorMatches, `cannot get unit "wordpress/0" from service "mysql": .*`)

	// Check that retrieving all units works.
	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(len(units), Equals, 2)
	c.Assert(units[0].Name(), Equals, "mysql/0")
	c.Assert(units[1].Name(), Equals, "mysql/1")
}

func (s *ServiceSuite) TestUnitRefresh(c *C) {
	u0, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	u1, err := s.service.Unit(u0.Name())
	c.Assert(err, IsNil)

	m, err := s.State.AddMachine()
	c.Assert(err, IsNil)
	err = u0.AssignToMachine(m)
	c.Assert(err, IsNil)

	c.Assert(u1.MachineId(), IsNil)
	err = u1.Refresh()
	c.Assert(u1.MachineId(), DeepEquals, u0.MachineId())
}

func (s *ServiceSuite) TestRemoveUnit(c *C) {
	_, err := s.service.AddUnit()
	c.Assert(err, IsNil)
	_, err = s.service.AddUnit()
	c.Assert(err, IsNil)

	// Check that removing a unit works.
	unit, err := s.service.Unit("mysql/0")
	c.Assert(err, IsNil)
	err = s.service.RemoveUnit(unit)
	c.Assert(err, IsNil)

	units, err := s.service.AllUnits()
	c.Assert(err, IsNil)
	c.Assert(units, HasLen, 1)
	c.Assert(units[0].Name(), Equals, "mysql/1")

	// Check that removing a non-existent unit fails nicely.
	// TODO(aram): improve error message.
	// BUG(aram): use error strings from state.
	err = s.service.RemoveUnit(unit)
	c.Assert(err, ErrorMatches, `cannot remove unit "mysql/0": .*`)
}
