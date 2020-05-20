// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/state"
)

// commonRelationSuiteMixin contains fields used by both relationSuite
// and relationUnitSuite. We're not just embeddnig relationUnitSuite
// into relationSuite to avoid running the former's tests twice.
type commonRelationSuiteMixin struct {
	mysqlMachine     *state.Machine
	mysqlApplication *state.Application
	mysqlCharm       *state.Charm
	mysqlUnit        *state.Unit

	stateRelation *state.Relation
}

type relationUnitSuite struct {
	uniterSuite
	commonRelationSuiteMixin
}

var _ = gc.Suite(&relationUnitSuite{})

func (m *commonRelationSuiteMixin) SetUpTest(c *gc.C, s uniterSuite) {
	// Create another machine, application and unit, so we can
	// test relations and relation units.
	m.mysqlMachine, m.mysqlApplication, m.mysqlCharm, m.mysqlUnit = s.addMachineAppCharmAndUnit(c, "mysql")

	// Add a relation, used by both this suite and relationSuite.
	m.stateRelation = s.addRelation(c, "wordpress", "mysql")
	err := m.stateRelation.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	apiRelation, err := s.uniter.Relation(s.stateRelation.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	// TODO(dfc)
	apiUnit, err := s.uniter.Unit(s.wordpressUnit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRelation.Unit(apiUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)
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

func (s *relationUnitSuite) TestEnterScopeSuccessfully(c *gc.C) {
	// NOTE: This test is not as exhaustive as the ones in state.
	// Here, we just check the success case, while the two error
	// cases are tested separately.
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)

	err := apiRelUnit.EnterScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
}

func (s *relationUnitSuite) TestEnterScopeErrCannotEnterScope(c *gc.C) {
	// Test the ErrCannotEnterScope gets forwarded correctly.
	// We need to enter the scope wit the other unit first.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, true)

	// Now we destroy mysqlApplication, so the relation is be set to
	// dying.
	err = s.mysqlApplication.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.stateRelation.Refresh()
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)

	// Now we create a subordinate of wordpressUnit and enter scope.
	subRel, _, loggingSub := s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)
	wpRelUnit, err := subRel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)

	// Leave scope, destroy the subordinate and try entering again.
	err = wpRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, false)
	err = loggingSub.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	apiUnit, err := s.uniter.Unit(s.wordpressUnit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRel, err := s.uniter.Relation(subRel.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiRelUnit, err := apiRel.Unit(apiUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	err = apiRelUnit.EnterScope()
	c.Assert(err, gc.NotNil)
	c.Check(err, jc.Satisfies, params.IsCodeCannotEnterScopeYet)
	c.Check(err, gc.ErrorMatches, "cannot enter scope yet: non-alive subordinate unit has not been removed")
}

func (s *relationUnitSuite) TestLeaveScope(c *gc.C) {
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)

	err := wpRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)

	err = apiRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, false)
}

func (s *relationUnitSuite) TestSettings(c *gc.C) {
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	settings := map[string]interface{}{
		"some":  "settings",
		"other": "things",
	}
	err := wpRelUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)

	gotSettings, err := apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"some":  "settings",
		"other": "things",
	})
}

func (s *relationUnitSuite) claimLeadership(c *gc.C, appName, unitName string) lease.Token {
	claimer, err := s.LeaseManager.Claimer(lease.ApplicationLeadershipNamespace, s.State.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(claimer.Claim(appName, unitName, time.Minute), jc.ErrorIsNil)
	checker, err := s.LeaseManager.Checker(lease.ApplicationLeadershipNamespace, s.State.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	return checker.Token(appName, unitName)
}

func (s *relationUnitSuite) claimLeadershipFor(c *gc.C, unit *state.Unit) lease.Token {
	return s.claimLeadership(c, unit.ApplicationName(), unit.Name())
}

func (s *relationUnitSuite) TestApplicationSettings(c *gc.C) {
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	settings := map[string]interface{}{
		"some":  "settings",
		"other": "things",
	}
	err := wpRelUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
	token := s.claimLeadershipFor(c, s.wordpressUnit)

	err = s.stateRelation.UpdateApplicationSettings("wordpress", token, map[string]interface{}{
		"foo": "bar",
		"baz": "1",
	})
	c.Assert(err, jc.ErrorIsNil)
	gotSettings, err := apiRelUnit.ApplicationSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"foo": "bar",
		"baz": "1",
	})

}

func (s *relationUnitSuite) TestReadSettings(c *gc.C) {
	// First try to read the settings which are not set.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, true)

	// Try reading - should be ok.
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)
	gotSettings, err := apiRelUnit.ReadSettings("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings, gc.HasLen, 0)

	// Now leave and re-enter scope with some settings.
	settings := map[string]interface{}{
		"some":  "settings",
		"other": "things",
	}
	err = myRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, false)
	err = myRelUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, true)
	gotSettings, err = apiRelUnit.ReadSettings("mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings, gc.DeepEquals, params.Settings{
		"some":  "settings",
		"other": "things",
	})
}

func (s *relationUnitSuite) TestReadApplicationSettings(c *gc.C) {
	// First try to read the settings which are not set.
	myRelUnit, err := s.stateRelation.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	// Set an application setting for mysql, notice that wordpress can read it

	// Add Wordpress Application Settings, and see that MySQL can read those App settings.
	token := s.claimLeadershipFor(c, s.mysqlUnit)
	settings := map[string]interface{}{
		"app": "settings",
	}
	err = s.stateRelation.UpdateApplicationSettings("mysql", token, settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, true)
	_, apiRelUnit := s.getRelationUnits(c)
	gotSettings, err := apiRelUnit.ReadSettings("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings, gc.DeepEquals, params.Settings{
		"app": "settings",
	})
}

func (s *relationUnitSuite) TestReadSettingsInvalidUnitTag(c *gc.C) {
	// First try to read the settings which are not set.
	myRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, true)

	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	s.assertInScope(c, wpRelUnit, false)
	// Not a valid unit or application name
	_, err = apiRelUnit.ReadSettings("0mysql")
	c.Assert(err, gc.ErrorMatches, "\"0mysql\" is not a valid unit or application")
}

func (s *relationUnitSuite) setupMysqlRelatedToWordpress(c *gc.C) (*state.RelationUnit, *uniter.Unit) {
	// Enter scope with mysqlUnit.
	mysqlRelUnit, err := s.stateRelation.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	apiRel, err := s.uniter.Relation(s.stateRelation.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	apiUnit, err := s.uniter.Unit(names.NewUnitTag("wordpress/0"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = apiRel.Unit(apiUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)

	// We just created the wordpress unit, make sure its event isn't still in the queue
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	return mysqlRelUnit, apiUnit
}

func (s *relationUnitSuite) TestWatchRelationUnits(c *gc.C) {
	mysqlRelUnit, wpAPIUnit := s.setupMysqlRelatedToWordpress(c)

	w, err := s.uniter.WatchRelationUnits(s.stateRelation.Tag().(names.RelationTag), wpAPIUnit.Tag())
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewRelationUnitsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange([]string{"mysql/0"}, []string{"mysql"}, nil)
	wc.AssertNoChange()

	// Leave scope with mysqlUnit, check it's detected.
	err = mysqlRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, false)
	wc.AssertChange(nil, nil, []string{"mysql/0"})

	// Non-change is not reported.
	err = mysqlRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
	// TODO(jam): make an application settings change and see that it is detected
}

func (s *relationUnitSuite) TestUpdateRelationSettingsForUnit(c *gc.C) {
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	err := wpRelUnit.EnterScope(map[string]interface{}{
		"some":  "settings",
		"other": "things",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
	gotSettings, err := apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"some":  "settings",
		"other": "things",
	})

	c.Assert(apiRelUnit.UpdateRelationSettings(params.Settings{
		"some": "thing else",
	}, nil), jc.ErrorIsNil)
	gotSettings, err = apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"some":  "thing else",
		"other": "things",
	})
}

func (s *relationUnitSuite) TestUpdateRelationSettingsForUnitWithDelete(c *gc.C) {
	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	err := wpRelUnit.EnterScope(map[string]interface{}{
		"some":  "settings",
		"other": "things",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
	gotSettings, err := apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"some":  "settings",
		"other": "things",
	})

	c.Assert(apiRelUnit.UpdateRelationSettings(params.Settings{
		"some": "",
	}, nil), jc.ErrorIsNil)
	gotSettings, err = apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"other": "things",
	})
}

func (s *relationUnitSuite) TestUpdateRelationSettingsForApplication(c *gc.C) {
	// Claim the leadership, but we don't need the token right now, we just need
	// to be the leader to call UpdateRelationSettings
	_ = s.claimLeadershipFor(c, s.wordpressUnit)

	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	c.Assert(wpRelUnit.EnterScope(nil), jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
	gotSettings, err := apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{})
	gotSettings, err = apiRelUnit.ApplicationSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{})

	c.Assert(apiRelUnit.UpdateRelationSettings(nil, params.Settings{"some": "value"}), jc.ErrorIsNil)
	gotSettings, err = apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{})
	gotSettings, err = apiRelUnit.ApplicationSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{
		"some": "value",
	})
}

func (s *relationUnitSuite) TestUpdateRelationSettingsForApplicationNotLeader(c *gc.C) {
	// s.wordpressUnit is wordpress/0, claim leadership by another unit
	_ = s.claimLeadership(c, "wordpress", "wordpress/2")

	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	c.Assert(wpRelUnit.EnterScope(nil), jc.ErrorIsNil)
	_, err := apiRelUnit.ApplicationSettings()
	c.Assert(err, gc.ErrorMatches, "permission denied.*")

	err = apiRelUnit.UpdateRelationSettings(nil, params.Settings{"some": "value"})
	c.Assert(err, gc.ErrorMatches, "permission denied.*")
}

func (s *relationUnitSuite) TestUpdateRelationSettingsForUnitAndApplication(c *gc.C) {
	_ = s.claimLeadershipFor(c, s.wordpressUnit)

	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	c.Assert(wpRelUnit.EnterScope(map[string]interface{}{
		"foo": "bar",
	}), jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
	c.Assert(apiRelUnit.UpdateRelationSettings(params.Settings{
		"foo": "quux",
	}, params.Settings{
		"app": "bar",
	}), jc.ErrorIsNil)
	gotSettings, err := apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{"foo": "quux"})
	gotSettings, err = apiRelUnit.ApplicationSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{"app": "bar"})
}

func (s *relationUnitSuite) TestUpdateRelationSettingsForUnitAndApplicationNotLeader(c *gc.C) {
	_ = s.claimLeadership(c, "wordpress", "wordpress/2")

	wpRelUnit, apiRelUnit := s.getRelationUnits(c)
	c.Assert(wpRelUnit.EnterScope(map[string]interface{}{
		"foo": "bar",
	}), jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
	err := apiRelUnit.UpdateRelationSettings(params.Settings{
		"foo": "quux",
	}, params.Settings{
		"app": "bar",
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	// Since we refused the change to the application settings, the change for the unit is also
	// rejected.

	gotSettings, err := apiRelUnit.Settings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSettings.Map(), gc.DeepEquals, params.Settings{"foo": "bar"})
	gotSettings, err = apiRelUnit.ApplicationSettings()
	c.Assert(err, gc.ErrorMatches, "permission denied.*")
}
