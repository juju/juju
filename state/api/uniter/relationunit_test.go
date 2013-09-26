// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/uniter"
	statetesting "launchpad.net/juju-core/state/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

// commonRelationSuiteMixin contains fields used by both relationSuite
// and relationUnitSuite. We're not just embeddnig relationUnitSuite
// into relationSuite to avoid running the former's tests twice.
type commonRelationSuiteMixin struct {
	mysqlMachine *state.Machine
	mysqlService *state.Service
	mysqlCharm   *state.Charm
	mysqlUnit    *state.Unit

	stateRelation *state.Relation
}

type relationUnitSuite struct {
	uniterSuite
	commonRelationSuiteMixin
}

var _ = gc.Suite(&relationUnitSuite{})

func (m *commonRelationSuiteMixin) SetUpTest(c *gc.C, s uniterSuite) {
	// Create another machine, service and unit, so we can
	// test relations and relation units.
	m.mysqlMachine, m.mysqlService, m.mysqlCharm, m.mysqlUnit = s.addMachineServiceCharmAndUnit(c, "mysql")

	// Add a relation, used by both this suite and relationSuite.
	m.stateRelation = s.addRelation(c, "wordpress", "mysql")
}

func (s *relationUnitSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	s.commonRelationSuiteMixin.SetUpTest(c, s.uniterSuite)
}

func (s *relationUnitSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *relationUnitSuite) getRelationUnits(c *gc.C) (*state.RelationUnit, *uniter.RelationUnit) {
	wpRelUnit, err := s.stateRelation.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	apiRelation, err := s.uniter.Relation(s.stateRelation.Tag())
	c.Assert(err, gc.IsNil)
	apiUnit, err := s.uniter.Unit(s.wordpressUnit.Tag())
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRelation.Unit(apiUnit)
	c.Assert(err, gc.IsNil)
	return wpRelUnit, apiRelUnit
}

func (s *relationUnitSuite) TestRelation(c *gc.C) {
	_, apiRelUnit := s.getRelationUnits(c)

	apiRel := apiRelUnit.Relation()
	c.Assert(apiRel, gc.NotNil)
	c.Assert(apiRel.String(), gc.Equals, "wordpress:db mysql:server")
}

func (s *relationUnitSuite) TestEndpoint(c *gc.C) {
	_, apiRelUnit := s.getRelationUnits(c)

	apiEndpoint := apiRelUnit.Endpoint()
	c.Assert(apiEndpoint, gc.DeepEquals, uniter.Endpoint{
		charm.Relation{
			Name:      "db",
			Role:      "requirer",
			Interface: "mysql",
			Optional:  false,
			Limit:     1,
			Scope:     "global",
		},
	})
}

func (s *relationUnitSuite) TestPrivateAddress(c *gc.C) {
	_, apiRelUnit := s.getRelationUnits(c)

	// Try getting it first without an address set.
	address, err := apiRelUnit.PrivateAddress()
	c.Assert(err, gc.ErrorMatches, `"unit-wordpress-0" has no private address set`)

	// Set an address and try again.
	err = s.wordpressUnit.SetPrivateAddress("1.2.3.4")
	c.Assert(err, gc.IsNil)
	address, err = apiRelUnit.PrivateAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(address, gc.Equals, "1.2.3.4")
}

func (s *relationUnitSuite) TestEnterScopeSuccessfully(c *gc.C) {
	// NOTE: This test is not as exhaustive as the ones in state.
	// Here, we just check the success case, while the two error
	// cases are tested separately.
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)

	err := apiRelUnit.EnterScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, wpRelUnit, true)
}

func (s *relationUnitSuite) TestEnterScopeErrCannotEnterScope(c *gc.C) {
	// Test the ErrCannotEnterScope gets forwarded correctly.
	// We need to enter the scope wit the other unit first.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, true)

	// Now we destroy mysqlService, so the relation is be set to
	// dying.
	err = s.mysqlService.Destroy()
	c.Assert(err, gc.IsNil)
	err = s.stateRelation.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.stateRelation.Life(), gc.Equals, state.Dying)

	// Enter the scope with wordpressUnit.
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)
	err = apiRelUnit.EnterScope()
	c.Assert(err, gc.NotNil)
	c.Check(err, jc.Satisfies, params.IsCodeCannotEnterScope)
	c.Check(err, gc.ErrorMatches, "cannot enter scope: unit or relation is not alive")
}

func (s *relationUnitSuite) TestEnterScopeErrCannotEnterScopeYet(c *gc.C) {
	// Test the ErrCannotEnterScopeYet gets forwarded correctly.
	// First we need to destroy the stateRelation.
	err := s.stateRelation.Destroy()
	c.Assert(err, gc.IsNil)

	// Now we create a subordinate of wordpressUnit and enter scope.
	subRel, _, loggingSub := s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)
	wpRelUnit, err := subRel.Unit(s.wordpressUnit)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, wpRelUnit, true)

	// Leave scope, destroy the subordinate and try entering again.
	err = wpRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, wpRelUnit, false)
	err = loggingSub.Destroy()
	c.Assert(err, gc.IsNil)

	apiUnit, err := s.uniter.Unit(s.wordpressUnit.Tag())
	c.Assert(err, gc.IsNil)
	apiRel, err := s.uniter.Relation(subRel.Tag())
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit)
	c.Assert(err, gc.IsNil)
	err = apiRelUnit.EnterScope()
	c.Assert(err, gc.NotNil)
	c.Check(err, jc.Satisfies, params.IsCodeCannotEnterScopeYet)
	c.Check(err, gc.ErrorMatches, "cannot enter scope yet: non-alive subordinate unit has not been removed")
}

func (s *relationUnitSuite) TestLeaveScope(c *gc.C) {
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)

	err := wpRelUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, wpRelUnit, true)

	err = apiRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, wpRelUnit, false)
}

func (s *relationUnitSuite) TestSettings(c *gc.C) {
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	settings := map[string]interface{}{
		"some":  "settings",
		"other": "things",
	}
	err := wpRelUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, wpRelUnit, true)

	gotSettings, err := apiRelUnit.Settings()
	c.Assert(err, gc.IsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.RelationSettings{
		"some":  "settings",
		"other": "things",
	})
}

func (s *relationUnitSuite) TestReadSettings(c *gc.C) {
	// First try to read the settings which are not set.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, true)

	// Try reading - should be ok.
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)
	gotSettings, err := apiRelUnit.ReadSettings("mysql/0")
	c.Assert(err, gc.IsNil)
	c.Assert(gotSettings, gc.HasLen, 0)

	// Now leave and re-enter scope with some settings.
	settings := map[string]interface{}{
		"some":  "settings",
		"other": "things",
	}
	err = myRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, false)
	err = myRelUnit.EnterScope(settings)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, true)
	gotSettings, err = apiRelUnit.ReadSettings("mysql/0")
	c.Assert(err, gc.IsNil)
	c.Assert(gotSettings, gc.DeepEquals, params.RelationSettings{
		"some":  "settings",
		"other": "things",
	})
}

func (s *relationUnitSuite) TestWatchRelationUnits(c *gc.C) {
	// Enter scope with mysqlUnit.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, gc.IsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, true)

	apiRel, err := s.uniter.Relation(s.stateRelation.Tag())
	c.Assert(err, gc.IsNil)
	apiUnit, err := s.uniter.Unit("unit-wordpress-0")
	c.Assert(err, gc.IsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit)
	c.Assert(err, gc.IsNil)

	w, err := apiRelUnit.Watch()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewRelationUnitsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange([]string{"mysql/0"}, nil)

	// Leave scope with mysqlUnit, check it's detected.
	err = myRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	s.assertInScope(c, myRelUnit, false)
	wc.AssertChange(nil, []string{"mysql/0"})

	// Non-change is not reported.
	err = myRelUnit.LeaveScope()
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// NOTE: This test is not as exhaustive as the one in state,
	// because the watcher is already tested there. Here we just
	// ensure we get the events when we expect them and don't get
	// them when they're not expected.

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}
