// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/deployer"
	"launchpad.net/juju-core/state/api/params"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type deployerSuite struct {
	testing.JujuConnSuite

	stateAPI *api.State

	// These are raw State objects. Use them for setup and assertions, but
	// should never be touched by the API calls themselves
	machine     *state.Machine
	service0    *state.Service
	service1    *state.Service
	principal   *state.Unit
	subordinate *state.Unit

	st *deployer.State
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.stateAPI, s.machine = s.OpenAPIAsNewMachine(c, state.JobManageEnviron, state.JobHostUnits)

	var err error
	// Create the needed services and relate them.
	s.service0 = s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	s.service1 = s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, gc.IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, gc.IsNil)

	// Create principal and subordinate units and assign them.
	s.principal, err = s.service0.AddUnit()
	c.Assert(err, gc.IsNil)
	err = s.principal.AssignToMachine(s.machine)
	c.Assert(err, gc.IsNil)
	relUnit, err := rel.Unit(s.principal)
	c.Assert(err, gc.IsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.subordinate, err = s.service1.Unit("logging/0")
	c.Assert(err, gc.IsNil)

	// Create the deployer facade.
	s.st = s.stateAPI.Deployer()
	c.Assert(s.st, gc.NotNil)
}

// Note: This is really meant as a unit-test, this isn't a test that
// should need all of the setup we have for this test suite
func (s *deployerSuite) TestNew(c *gc.C) {
	deployer := deployer.NewState(s.stateAPI)
	c.Assert(deployer, gc.NotNil)
}

func (s *deployerSuite) assertUnauthorized(c *gc.C, err error) {
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *deployerSuite) TestWatchUnitsWrongMachine(c *gc.C) {
	// Try with a non-existent machine tag.
	machine, err := s.st.Machine("machine-42")
	c.Assert(err, gc.IsNil)
	w, err := machine.WatchUnits()
	s.assertUnauthorized(c, err)
	c.Assert(w, gc.IsNil)

	// Try it with an invalid tag format.
	machine, err = s.st.Machine("foo")
	c.Assert(err, gc.IsNil)
	w, err = machine.WatchUnits()
	s.assertUnauthorized(c, err)
	c.Assert(w, gc.IsNil)
}

func (s *deployerSuite) TestWatchUnits(c *gc.C) {
	machine, err := s.st.Machine(s.machine.Tag())
	c.Assert(err, gc.IsNil)
	w, err := machine.WatchUnits()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange("mysql/0", "logging/0")
	wc.AssertNoChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = s.subordinate.SetPassword("foo")
	c.Assert(err, gc.ErrorMatches, "password is only 3 bytes long, and is not a valid Agent password")
	wc.AssertNoChange()

	err = s.subordinate.SetPassword("foo-12345678901234567890")
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the subordinate dead and check it's detected.
	err = s.subordinate.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertChange("logging/0")
	wc.AssertNoChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *deployerSuite) TestUnit(c *gc.C) {
	// Try getting a missing unit and an invalid tag.
	unit, err := s.st.Unit("unit-foo-42")
	s.assertUnauthorized(c, err)
	c.Assert(unit, gc.IsNil)
	unit, err = s.st.Unit("42")
	s.assertUnauthorized(c, err)
	c.Assert(unit, gc.IsNil)

	// Try getting a unit we're not responsible for.
	// First create a new machine and deploy another unit there.
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	principal1, err := s.service0.AddUnit()
	c.Assert(err, gc.IsNil)
	err = principal1.AssignToMachine(machine)
	c.Assert(err, gc.IsNil)
	unit, err = s.st.Unit(principal1.Tag())
	s.assertUnauthorized(c, err)
	c.Assert(unit, gc.IsNil)

	// Get the principal and subordinate we're responsible for.
	unit, err = s.st.Unit(s.principal.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Name(), gc.Equals, "mysql/0")
	unit, err = s.st.Unit(s.subordinate.Tag())
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Name(), gc.Equals, "logging/0")
}

func (s *deployerSuite) TestUnitLifeRefresh(c *gc.C) {
	unit, err := s.st.Unit(s.subordinate.Tag())
	c.Assert(err, gc.IsNil)

	c.Assert(unit.Life(), gc.Equals, params.Alive)

	// Now make it dead and check again, then refresh and check.
	err = s.subordinate.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.subordinate.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.subordinate.Life(), gc.Equals, state.Dead)
	c.Assert(unit.Life(), gc.Equals, params.Alive)
	err = unit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(unit.Life(), gc.Equals, params.Dead)
}

func (s *deployerSuite) TestUnitRemove(c *gc.C) {
	unit, err := s.st.Unit(s.principal.Tag())
	c.Assert(err, gc.IsNil)

	// It fails because the entity is still alive.
	// And EnsureDead will fail because there is a subordinate.
	err = unit.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove entity "unit-mysql-0": still alive`)
	c.Assert(params.ErrCode(err), gc.Equals, "")

	// With the subordinate it also fails due to it being alive.
	unit, err = s.st.Unit(s.subordinate.Tag())
	c.Assert(err, gc.IsNil)
	err = unit.Remove()
	c.Assert(err, gc.ErrorMatches, `cannot remove entity "unit-logging-0": still alive`)
	c.Assert(params.ErrCode(err), gc.Equals, "")

	// Make it dead first and try again.
	err = s.subordinate.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = unit.Remove()
	c.Assert(err, gc.IsNil)

	// Verify it's gone.
	err = unit.Refresh()
	s.assertUnauthorized(c, err)
	unit, err = s.st.Unit(s.subordinate.Tag())
	s.assertUnauthorized(c, err)
	c.Assert(unit, gc.IsNil)
}

func (s *deployerSuite) TestUnitSetPassword(c *gc.C) {
	unit, err := s.st.Unit(s.principal.Tag())
	c.Assert(err, gc.IsNil)

	// Change the principal's password and verify.
	err = unit.SetPassword("foobar-12345678901234567890")
	c.Assert(err, gc.IsNil)
	err = s.principal.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.principal.PasswordValid("foobar-12345678901234567890"), gc.Equals, true)

	// Then the subordinate.
	unit, err = s.st.Unit(s.subordinate.Tag())
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword("phony-12345678901234567890")
	c.Assert(err, gc.IsNil)
	err = s.subordinate.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.subordinate.PasswordValid("phony-12345678901234567890"), gc.Equals, true)
}

func (s *deployerSuite) TestStateAddresses(c *gc.C) {
	addrs := []instance.Address{
		instance.NewAddress("0.1.2.3"),
	}
	err := s.machine.SetAddresses(addrs)
	c.Assert(err, gc.IsNil)

	stateAddresses, err := s.State.Addresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(stateAddresses), gc.Equals, 1)

	addresses, err := s.st.StateAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, stateAddresses)
}

func (s *deployerSuite) TestAPIAddresses(c *gc.C) {
	addrs := []instance.Address{
		instance.NewAddress("0.1.2.3"),
	}
	err := s.machine.SetAddresses(addrs)
	c.Assert(err, gc.IsNil)

	stateAddresses, err := s.State.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(len(stateAddresses), gc.Equals, 1)

	addresses, err := s.st.APIAddresses()
	c.Assert(err, gc.IsNil)
	c.Assert(addresses, gc.DeepEquals, stateAddresses)
}

func (s *deployerSuite) TestCACert(c *gc.C) {
	caCert, err := s.st.CACert()
	c.Assert(err, gc.IsNil)
	c.Assert(caCert, gc.DeepEquals, s.State.CACert())
}
