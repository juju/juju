// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/schema"
	coretesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher/watchertest"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	jujufactory "github.com/juju/juju/testing/factory"
)

type unitSuite struct {
	uniterSuite

	apiUnit *uniter.Unit
}

var _ = gc.Suite(&unitSuite{})

func (s *unitSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)

	var err error
	s.apiUnit, err = s.uniter.Unit(s.wordpressUnit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitSuite) TestRequestReboot(c *gc.C) {
	err := s.apiUnit.RequestReboot()
	c.Assert(err, jc.ErrorIsNil)
	rFlag, err := s.wordpressMachine.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsTrue)
}

func (s *unitSuite) TestUnitAndUnitTag(c *gc.C) {
	apiUnitFoo, err := s.uniter.Unit(names.NewUnitTag("foo/42"))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(apiUnitFoo, gc.IsNil)

	c.Assert(s.apiUnit.Tag(), gc.Equals, s.wordpressUnit.Tag().(names.UnitTag))
}

func (s *unitSuite) TestSetAgentStatus(c *gc.C) {
	statusInfo, err := s.wordpressUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Allocating)
	c.Assert(statusInfo.Message, gc.Equals, "")
	c.Assert(statusInfo.Data, gc.HasLen, 0)

	unitStatusInfo, err := s.wordpressUnit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitStatusInfo.Status, gc.Equals, status.Waiting)
	c.Assert(unitStatusInfo.Message, gc.Equals, "waiting for machine")
	c.Assert(unitStatusInfo.Data, gc.HasLen, 0)

	err = s.apiUnit.SetAgentStatus(status.Idle, "blah", nil)
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err = s.wordpressUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Idle)
	c.Assert(statusInfo.Message, gc.Equals, "blah")
	c.Assert(statusInfo.Data, gc.HasLen, 0)
	c.Assert(statusInfo.Since, gc.NotNil)

	// Ensure that unit has not changed.
	unitStatusInfo, err = s.wordpressUnit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitStatusInfo.Status, gc.Equals, status.Waiting)
	c.Assert(unitStatusInfo.Message, gc.Equals, "waiting for machine")
	c.Assert(unitStatusInfo.Data, gc.HasLen, 0)
}

func (s *unitSuite) TestSetUnitStatus(c *gc.C) {
	statusInfo, err := s.wordpressUnit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Waiting)
	c.Assert(statusInfo.Message, gc.Equals, "waiting for machine")
	c.Assert(statusInfo.Data, gc.HasLen, 0)

	agentStatusInfo, err := s.wordpressUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentStatusInfo.Status, gc.Equals, status.Allocating)
	c.Assert(agentStatusInfo.Message, gc.Equals, "")
	c.Assert(agentStatusInfo.Data, gc.HasLen, 0)

	err = s.apiUnit.SetUnitStatus(status.Active, "blah", nil)
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err = s.wordpressUnit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Active)
	c.Assert(statusInfo.Message, gc.Equals, "blah")
	c.Assert(statusInfo.Data, gc.HasLen, 0)
	c.Assert(statusInfo.Since, gc.NotNil)

	// Ensure unit's agent has not changed.
	agentStatusInfo, err = s.wordpressUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agentStatusInfo.Status, gc.Equals, status.Allocating)
	c.Assert(agentStatusInfo.Message, gc.Equals, "")
	c.Assert(agentStatusInfo.Data, gc.HasLen, 0)
}

func (s *unitSuite) TestUnitStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "blah",
		Since:   &now,
	}
	err := s.wordpressUnit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.apiUnit.UnitStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Since, gc.NotNil)
	result.Since = nil
	c.Assert(result, gc.DeepEquals, params.StatusResult{
		Status: status.Maintenance.String(),
		Info:   "blah",
		Data:   map[string]interface{}{},
	})
}

func (s *unitSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	err := s.apiUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)

	err = s.apiUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)

	err = s.wordpressUnit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.apiUnit.EnsureDead()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" not found`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
}

func (s *unitSuite) TestDestroy(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	err := s.apiUnit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" not found`)
}

func (s *unitSuite) TestDestroyAllSubordinates(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	// Call without subordinates - no change.
	err := s.apiUnit.DestroyAllSubordinates()
	c.Assert(err, jc.ErrorIsNil)

	// Add a couple of subordinates and try again.
	_, _, loggingSub := s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)
	_, _, monitoringSub := s.addRelatedApplication(c, "wordpress", "monitoring", s.wordpressUnit)
	c.Assert(loggingSub.Life(), gc.Equals, state.Alive)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Alive)

	err = s.apiUnit.DestroyAllSubordinates()
	c.Assert(err, jc.ErrorIsNil)

	// Verify they got destroyed.
	err = loggingSub.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(loggingSub.Life(), gc.Equals, state.Dying)
	err = monitoringSub.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Dying)
}

func (s *unitSuite) TestRefreshLife(c *gc.C) {
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	err := s.apiUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	err = s.apiUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Dead)
}

func (s *unitSuite) TestRefreshResolve(c *gc.C) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)

	err = s.apiUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mode := s.apiUnit.Resolved()
	c.Assert(mode, gc.Equals, params.ResolvedRetryHooks)

	err = s.apiUnit.ClearResolved()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.apiUnit.Resolved()
	c.Assert(mode, gc.Equals, params.ResolvedRetryHooks)

	err = s.apiUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.apiUnit.Resolved()
	c.Assert(mode, gc.Equals, params.ResolvedNone)
}

func (s *unitSuite) TestRefreshSeries(c *gc.C) {
	c.Assert(s.apiUnit.Series(), gc.Equals, "quantal")
	err := s.wordpressMachine.UpdateMachineSeries("xenial", true)
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiUnit.Series(), gc.Equals, "quantal")

	err = s.apiUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.apiUnit.Series(), gc.Equals, "xenial")
}

func (s *unitSuite) TestWatch(c *gc.C) {
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	w, err := s.apiUnit.Watch()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = s.apiUnit.SetAgentStatus(status.Idle, "not really", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Make the unit dead and check it's detected.
	err = s.apiUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *unitSuite) TestWatchRelations(c *gc.C) {
	w, err := s.apiUnit.WatchRelations()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange()
	wc.AssertNoChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = s.wordpressApplication.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Add another application and relate it to wordpress,
	// check it's detected.
	s.addMachineAppCharmAndUnit(c, "mysql")
	rel := s.addRelation(c, "wordpress", "mysql")
	wc.AssertChange(rel.String())

	// Destroy the relation and check it's detected.
	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(rel.String())
	wc.AssertNoChange()
}

func (s *unitSuite) TestSubordinateWatchRelations(c *gc.C) {
	// A subordinate unit deployed with this wordpress unit shouldn't
	// be notified about changes to logging mysql.
	loggingRel, _, loggingUnit := s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = loggingUnit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	// Add another principal app that we can relate logging to.
	s.addMachineAppCharmAndUnit(c, "mysql")

	api := s.OpenAPIAs(c, loggingUnit.Tag(), password)
	uniter, err := api.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	apiUnit, err := uniter.Unit(loggingUnit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	w, err := apiUnit.WatchRelations()
	c.Assert(err, jc.ErrorIsNil)

	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	wc.AssertChange(loggingRel.Tag().Id())
	wc.AssertNoChange()

	// Adding a subordinate relation to another application doesn't notify this unit.
	s.addRelation(c, "mysql", "logging")
	wc.AssertNoChange()

	// Destroying a relevant relation does notify it.
	err = loggingRel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(loggingRel.Tag().Id())
	wc.AssertNoChange()
}

func (s *unitSuite) TestAssignedMachine(c *gc.C) {
	machineTag, err := s.apiUnit.AssignedMachine()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineTag, gc.Equals, s.wordpressMachine.Tag())
}

func (s *unitSuite) TestPrincipalName(c *gc.C) {
	unitName, ok, err := s.apiUnit.PrincipalName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, jc.IsFalse)
	c.Assert(unitName, gc.Equals, "")
}

func (s *unitSuite) TestHasSubordinates(c *gc.C) {
	found, err := s.apiUnit.HasSubordinates()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.IsFalse)

	// Add a couple of subordinates and try again.
	s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)
	s.addRelatedApplication(c, "wordpress", "monitoring", s.wordpressUnit)

	found, err = s.apiUnit.HasSubordinates()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, jc.IsTrue)
}

func (s *unitSuite) TestPublicAddress(c *gc.C) {
	address, err := s.apiUnit.PublicAddress()
	c.Assert(err, gc.ErrorMatches, `"unit-wordpress-0" has no public address set`)

	err = s.wordpressMachine.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

	address, err = s.apiUnit.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "1.2.3.4")
}

func (s *unitSuite) TestPrivateAddress(c *gc.C) {
	address, err := s.apiUnit.PrivateAddress()
	c.Assert(err, gc.ErrorMatches, `"unit-wordpress-0" has no private address set`)

	err = s.wordpressMachine.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)

	address, err = s.apiUnit.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "1.2.3.4")
}

func (s *unitSuite) TestAvailabilityZone(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "AvailabilityZone",
		func(result interface{}) error {
			if results, ok := result.(*params.StringResults); ok {
				results.Results = []params.StringResult{{
					Result: "a-zone",
				}}
			}
			return nil
		},
	)

	zone, err := s.apiUnit.AvailabilityZone()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zone, gc.Equals, "a-zone")
}

func (s *unitSuite) TestOpenClosePortRanges(c *gc.C) {
	ports, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)

	err = s.apiUnit.OpenPorts("tcp", 1234, 1400)
	c.Assert(err, jc.ErrorIsNil)
	err = s.apiUnit.OpenPorts("udp", 4321, 5000)
	c.Assert(err, jc.ErrorIsNil)

	ports, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	// OpenedPorts returns a sorted slice.
	c.Assert(ports, gc.DeepEquals, []network.PortRange{
		{Protocol: "tcp", FromPort: 1234, ToPort: 1400},
		{Protocol: "udp", FromPort: 4321, ToPort: 5000},
	})

	err = s.apiUnit.ClosePorts("udp", 4321, 5000)
	c.Assert(err, jc.ErrorIsNil)

	ports, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	// OpenedPorts returns a sorted slice.
	c.Assert(ports, gc.DeepEquals, []network.PortRange{
		{Protocol: "tcp", FromPort: 1234, ToPort: 1400},
	})

	err = s.apiUnit.ClosePorts("tcp", 1234, 1400)
	c.Assert(err, jc.ErrorIsNil)

	ports, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)
}

func (s *unitSuite) TestGetSetCharmURL(c *gc.C) {
	// No charm URL set yet.
	curl, ok := s.wordpressUnit.CharmURL()
	c.Assert(curl, gc.IsNil)
	c.Assert(ok, jc.IsFalse)

	// Now check the same through the API.
	_, err := s.apiUnit.CharmURL()
	c.Assert(err, gc.Equals, uniter.ErrNoCharmURLSet)

	err = s.apiUnit.SetCharmURL(s.wordpressCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	curl, err = s.apiUnit.CharmURL()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(curl, gc.NotNil)
	c.Assert(curl.String(), gc.Equals, s.wordpressCharm.String())
}

func (s *unitSuite) TestNetworkInfo(c *gc.C) {
	var called int
	relId := 2
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		called++
		if called == 1 {
			*(result.(*params.UnitRefreshResults)) = params.UnitRefreshResults{
				Results: []params.UnitRefreshResult{{Life: params.Alive, Resolved: params.ResolvedNone, Series: "quantal"}}}
			return nil
		}
		c.Check(objType, gc.Equals, "Uniter")
		c.Check(version, gc.Equals, expectedVersion)
		c.Check(id, gc.Equals, "")
		c.Check(request, gc.Equals, "NetworkInfo")
		c.Check(arg, gc.DeepEquals, params.NetworkInfoParams{
			Unit:       "unit-mysql-0",
			Bindings:   []string{"server"},
			RelationId: &relId,
		})
		c.Assert(result, gc.FitsTypeOf, &params.NetworkInfoResults{})
		*(result.(*params.NetworkInfoResults)) = params.NetworkInfoResults{
			Results: map[string]params.NetworkInfoResult{
				"db": {
					Error: &params.Error{Message: "FAIL"},
				}},
		}
		return nil
	})

	ut := names.NewUnitTag("mysql/0")
	st := uniter.NewState(apiCaller, ut)
	unit, err := st.Unit(ut)
	c.Assert(err, jc.ErrorIsNil)
	result, err := unit.NetworkInfo([]string{"server"}, &relId)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result["db"].Error, gc.ErrorMatches, "FAIL")
	c.Assert(called, gc.Equals, 2)
}

func (s *unitSuite) TestConfigSettings(c *gc.C) {
	// Make sure ConfigSettings returns an error when
	// no charm URL is set, as its state counterpart does.
	settings, err := s.apiUnit.ConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")

	// Now set the charm and try again.
	err = s.apiUnit.SetCharmURL(s.wordpressCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	settings, err = s.apiUnit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"blog-title": "My Title",
	})

	// Update the config and check we get the changes on the next call.
	err = s.wordpressApplication.UpdateCharmConfig(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, jc.ErrorIsNil)

	settings, err = s.apiUnit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"blog-title": "superhero paparazzi",
	})
}

func (s *unitSuite) TestWatchConfigSettings(c *gc.C) {
	// Make sure WatchConfigSettings returns an error when
	// no charm URL is set, as its state counterpart does.
	w, err := s.apiUnit.WatchConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")

	// Now set the charm and try again.
	err = s.apiUnit.SetCharmURL(s.wordpressCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	w, err = s.apiUnit.WatchConfigSettings()
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	err = s.wordpressApplication.UpdateCharmConfig(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.wordpressApplication.UpdateCharmConfig(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *unitSuite) TestWatchTrustConfigSettings(c *gc.C) {
	watcher, err := s.apiUnit.WatchTrustConfigSettings()
	c.Assert(err, jc.ErrorIsNil)

	notifyWatcher := watchertest.NewNotifyWatcherC(c, watcher, s.BackingState.StartSync)
	defer notifyWatcher.AssertStops()

	// Initial event.
	notifyWatcher.AssertOneChange()

	// Update application config and see if it is reported
	trustFieldKey := "trust"
	s.wordpressApplication.UpdateApplicationConfig(application.ConfigAttributes{
		trustFieldKey: true,
	},
		[]string{},
		environschema.Fields{trustFieldKey: {
			Description: "Does this application have access to trusted credentials",
			Type:        environschema.Tbool,
			Group:       environschema.JujuGroup,
		}},
		schema.Defaults{
			trustFieldKey: false,
		},
	)
	notifyWatcher.AssertOneChange()
}

func (s *unitSuite) TestWatchActionNotifications(c *gc.C) {
	w, err := s.apiUnit.WatchActionNotifications()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertChange()

	// Add a couple of actions and make sure the changes are detected.
	action, err := s.wordpressUnit.AddAction("fakeaction", map[string]interface{}{
		"outfile": "foo.txt",
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(action.Id())

	action, err = s.wordpressUnit.AddAction("fakeaction", map[string]interface{}{
		"outfile": "foo.bz2",
		"compression": map[string]interface{}{
			"kind":    "bzip",
			"quality": float64(5.0),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(action.Id())
}

func (s *unitSuite) TestWatchActionNotificationsError(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "WatchActionNotifications",
		func(result interface{}) error {
			return fmt.Errorf("Test error")
		},
	)

	_, err := s.apiUnit.WatchActionNotifications()
	c.Assert(err.Error(), gc.Equals, "Test error")
}

func (s *unitSuite) TestWatchActionNotificationsErrorResults(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "WatchActionNotifications",
		func(results interface{}) error {
			if results, ok := results.(*params.StringsWatchResults); ok {
				results.Results = make([]params.StringsWatchResult, 1)
				results.Results[0] = params.StringsWatchResult{
					Error: &params.Error{
						Message: "An error in the watch result.",
						Code:    params.CodeNotAssigned,
					},
				}
			}
			return nil
		},
	)

	_, err := s.apiUnit.WatchActionNotifications()
	c.Assert(err.Error(), gc.Equals, "An error in the watch result.")
}

func (s *unitSuite) TestWatchActionNotificationsNoResults(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "WatchActionNotifications",
		func(results interface{}) error {
			return nil
		},
	)

	_, err := s.apiUnit.WatchActionNotifications()
	c.Assert(err.Error(), gc.Equals, "expected 1 result, got 0")
}

func (s *unitSuite) TestWatchActionNotificationsMoreResults(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "WatchActionNotifications",
		func(results interface{}) error {
			if results, ok := results.(*params.StringsWatchResults); ok {
				results.Results = make([]params.StringsWatchResult, 2)
			}
			return nil
		},
	)

	_, err := s.apiUnit.WatchActionNotifications()
	c.Assert(err.Error(), gc.Equals, "expected 1 result, got 2")
}

func (s *unitSuite) TestWatchUpgradeSeriesNotifications(c *gc.C) {
	watcher, err := s.apiUnit.WatchUpgradeSeriesNotifications()
	c.Assert(err, jc.ErrorIsNil)

	notifyWatcher := watchertest.NewNotifyWatcherC(c, watcher, s.BackingState.StartSync)
	defer notifyWatcher.AssertStops()

	notifyWatcher.AssertOneChange()

	s.CreateUpgradeSeriesLock(c)

	// Expect a notification that the document was created (i.e. a lock was placed)
	notifyWatcher.AssertOneChange()

	err = s.wordpressMachine.RemoveUpgradeSeriesLock()
	c.Assert(err, jc.ErrorIsNil)

	// A notification that the document was removed (i.e. the lock was released)
	notifyWatcher.AssertOneChange()
}

func (s *unitSuite) TestUpgradeSeriesStatusIsInitializedToUnitStarted(c *gc.C) {
	// First we create the prepare lock
	s.CreateUpgradeSeriesLock(c)

	// Then we check to see the status of our upgrade. We note that creating
	// the lock essentially kicks off an upgrade from the perspective of
	// assigned units.
	status, err := s.apiUnit.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, model.UpgradeSeriesPrepareStarted)
}

func (s *unitSuite) TestSetUpgradeSeriesStatusFailsIfNoLockExists(c *gc.C) {
	arbitraryStatus := model.UpgradeSeriesNotStarted
	arbitraryReason := ""

	err := s.apiUnit.SetUpgradeSeriesStatus(arbitraryStatus, arbitraryReason)
	c.Assert(err, gc.ErrorMatches, "upgrade lock for machine \"[0-9]*\" not found")
}

func (s *unitSuite) TestSetUpgradeSeriesStatusUpdatesStatus(c *gc.C) {
	arbitraryNonDefaultStatus := model.UpgradeSeriesPrepareRunning
	arbitraryReason := ""

	// First we create the prepare lock or the required state will not exists
	s.CreateUpgradeSeriesLock(c)

	// Change the state to something other than the default remote state of UpgradeSeriesPrepareStarted
	err := s.apiUnit.SetUpgradeSeriesStatus(arbitraryNonDefaultStatus, arbitraryReason)
	c.Assert(err, jc.ErrorIsNil)

	// Check to see that the upgrade status has been set appropriately
	status, err := s.apiUnit.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, arbitraryNonDefaultStatus)
}

func (s *unitSuite) TestSetUpgradeSeriesStatusShouldOnlySetSpecifiedUnit(c *gc.C) {
	// add another unit
	unit2, err := s.wordpressApplication.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	err = unit2.AssignToMachine(s.wordpressMachine)
	c.Assert(err, jc.ErrorIsNil)

	// Creating a lock for the machine transitions all units to started state
	s.CreateUpgradeSeriesLock(c, unit2.Name())

	// Complete one unit
	err = unit2.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareCompleted, "")
	c.Assert(err, jc.ErrorIsNil)

	// The other unit should still be in the started state
	status, err := s.wordpressUnit.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.Equals, model.UpgradeSeriesPrepareStarted)
}

func (s *unitSuite) CreateUpgradeSeriesLock(c *gc.C, additionalUnits ...string) {
	unitNames := additionalUnits
	unitNames = append(unitNames, s.wordpressUnit.Name())
	series := "trust"

	err := s.wordpressMachine.CreateUpgradeSeriesLock(unitNames, series)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitSuite) TestApplicationNameAndTag(c *gc.C) {
	c.Assert(s.apiUnit.ApplicationName(), gc.Equals, s.wordpressApplication.Name())
	c.Assert(s.apiUnit.ApplicationTag(), gc.Equals, s.wordpressApplication.Tag())
}

func (s *unitSuite) TestRelationSuspended(c *gc.C) {
	relationStatus, err := s.apiUnit.RelationsStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationStatus, gc.HasLen, 0)

	rel1, _, _ := s.addRelatedApplication(c, "wordpress", "monitoring", s.wordpressUnit)
	relationStatus, err = s.apiUnit.RelationsStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationStatus, gc.DeepEquals, []uniter.RelationStatus{{
		Tag:       rel1.Tag().(names.RelationTag),
		InScope:   true,
		Suspended: false,
	}})

	rel2 := s.addRelationSuspended(c, "wordpress", "logging", s.wordpressUnit)
	relationStatus, err = s.apiUnit.RelationsStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relationStatus, jc.SameContents, []uniter.RelationStatus{{
		Tag:       rel1.Tag().(names.RelationTag),
		InScope:   true,
		Suspended: false,
	}, {
		Tag:       rel2.Tag().(names.RelationTag),
		InScope:   false,
		Suspended: true,
	}})
}

func (s *unitSuite) TestWatchAddresses(c *gc.C) {
	w, err := s.apiUnit.WatchAddresses()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	// Update config get an event.
	err = s.wordpressMachine.SetProviderAddresses(network.NewAddress("0.1.2.4"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.wordpressMachine.SetProviderAddresses(network.NewAddress("0.1.2.4"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	// Change is reported for machine addresses.
	err = s.wordpressMachine.SetMachineAddresses(network.NewAddress("0.1.2.5"))
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Set machine addresses to empty is reported.
	err = s.wordpressMachine.SetMachineAddresses()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *unitSuite) TestWatchAddressesErrors(c *gc.C) {
	err := s.wordpressUnit.UnassignFromMachine()
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.apiUnit.WatchAddresses()
	c.Assert(err, jc.Satisfies, params.IsCodeNotAssigned)
}

func (s *unitSuite) TestAddMetrics(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "AddMetrics",
		func(results interface{}) error {
			result := results.(*params.ErrorResults)
			result.Results = make([]params.ErrorResult, 1)
			return nil
		},
	)
	metrics := []params.Metric{{
		Key: "A", Value: "23", Time: time.Now(),
	}, {
		Key: "B", Value: "27.0", Time: time.Now(), Labels: map[string]string{"foo": "bar"},
	}}
	err := s.apiUnit.AddMetrics(metrics)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitSuite) TestAddMetricsError(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "AddMetrics",
		func(results interface{}) error {
			result := results.(*params.ErrorResults)
			result.Results = make([]params.ErrorResult, 1)
			return fmt.Errorf("test error")
		},
	)
	metrics := []params.Metric{{
		Key: "A", Value: "23", Time: time.Now(),
	}, {
		Key: "B", Value: "27.0", Time: time.Now(), Labels: map[string]string{"foo": "bar"},
	}}
	err := s.apiUnit.AddMetrics(metrics)
	c.Assert(err, gc.ErrorMatches, "unable to add metric: test error")
}

func (s *unitSuite) TestAddMetricsResultError(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "AddMetrics",
		func(results interface{}) error {
			result := results.(*params.ErrorResults)
			result.Results = make([]params.ErrorResult, 1)
			result.Results[0].Error = &params.Error{
				Message: "error adding metrics",
				Code:    params.CodeNotAssigned,
			}
			return nil
		},
	)
	metrics := []params.Metric{{
		Key: "A", Value: "23", Time: time.Now(),
	}, {
		Key: "B", Value: "27.0", Time: time.Now(), Labels: map[string]string{"foo": "bar"},
	}}
	err := s.apiUnit.AddMetrics(metrics)
	c.Assert(err, gc.ErrorMatches, "error adding metrics")
}

func (s *unitSuite) TestMeterStatus(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "GetMeterStatus",
		func(results interface{}) error {
			result := results.(*params.MeterStatusResults)
			result.Results = make([]params.MeterStatusResult, 1)
			result.Results[0].Code = "GREEN"
			result.Results[0].Info = "All ok."
			return nil
		},
	)
	statusCode, statusInfo, err := s.apiUnit.MeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusCode, gc.Equals, "GREEN")
	c.Assert(statusInfo, gc.Equals, "All ok.")
}

func (s *unitSuite) TestMeterStatusError(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "GetMeterStatus",
		func(results interface{}) error {
			result := results.(*params.MeterStatusResults)
			result.Results = make([]params.MeterStatusResult, 1)
			return fmt.Errorf("boo")
		},
	)
	statusCode, statusInfo, err := s.apiUnit.MeterStatus()
	c.Assert(err, gc.ErrorMatches, "boo")
	c.Assert(statusCode, gc.Equals, "")
	c.Assert(statusInfo, gc.Equals, "")
}

func (s *unitSuite) TestMeterStatusResultError(c *gc.C) {
	uniter.PatchUnitResponse(s, s.apiUnit, "GetMeterStatus",
		func(results interface{}) error {
			result := results.(*params.MeterStatusResults)
			result.Results = make([]params.MeterStatusResult, 1)
			result.Results[0].Error = &params.Error{
				Message: "error getting meter status",
				Code:    params.CodeNotAssigned,
			}
			return nil
		},
	)
	statusCode, statusInfo, err := s.apiUnit.MeterStatus()
	c.Assert(err, gc.ErrorMatches, "error getting meter status")
	c.Assert(statusCode, gc.Equals, "")
	c.Assert(statusInfo, gc.Equals, "")
}

func (s *unitSuite) TestUpgradeSeriesStatusMultipleReturnsError(c *gc.C) {
	facadeCaller := testing.StubFacadeCaller{Stub: &coretesting.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{
				{Status: "prepare started"},
				{Status: "completed"},
			},
		}
		return nil
	}
	uniter.PatchUnitUpgradeSeriesFacade(s.apiUnit, &facadeCaller)

	_, err := s.apiUnit.UpgradeSeriesStatus()
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *unitSuite) TestUpgradeSeriesStatusSingleResult(c *gc.C) {
	facadeCaller := testing.StubFacadeCaller{Stub: &coretesting.Stub{}}
	facadeCaller.FacadeCallFn = func(name string, args, response interface{}) error {
		*(response.(*params.UpgradeSeriesStatusResults)) = params.UpgradeSeriesStatusResults{
			Results: []params.UpgradeSeriesStatusResult{{Status: "completed"}},
		}
		return nil
	}
	uniter.PatchUnitUpgradeSeriesFacade(s.apiUnit, &facadeCaller)

	sts, err := s.apiUnit.UpgradeSeriesStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(sts, gc.Equals, model.UpgradeSeriesCompleted)
}

type unitMetricBatchesSuite struct {
	jujutesting.JujuConnSuite

	st      api.Connection
	uniter  *uniter.State
	apiUnit *uniter.Unit
	charm   *state.Charm
}

var _ = gc.Suite(&unitMetricBatchesSuite{})

func (s *unitMetricBatchesSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.charm = s.Factory.MakeCharm(c, &jujufactory.CharmParams{
		Name: "metered",
		URL:  "cs:quantal/metered",
	})
	application := s.Factory.MakeApplication(c, &jujufactory.ApplicationParams{
		Charm: s.charm,
	})
	unit := s.Factory.MakeUnit(c, &jujufactory.UnitParams{
		Application: application,
		SetCharmURL: true,
	})

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.st = s.OpenAPIAs(c, unit.Tag(), password)

	// Create the uniter API facade.
	s.uniter, err = s.st.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.uniter, gc.NotNil)

	s.apiUnit, err = s.uniter.Unit(unit.Tag().(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *unitMetricBatchesSuite) TestSendMetricBatchPatch(c *gc.C) {
	metrics := []params.Metric{{
		Key: "pings", Value: "5", Time: time.Now().UTC(),
	}, {
		Key: "pongs", Value: "6", Time: time.Now().UTC(), Labels: map[string]string{"foo": "bar"},
	}}
	uuid := utils.MustNewUUID().String()
	batch := params.MetricBatch{
		UUID:     uuid,
		CharmURL: s.charm.URL().String(),
		Created:  time.Now(),
		Metrics:  metrics,
	}

	var called bool
	uniter.PatchUnitResponse(s, s.apiUnit, "AddMetricBatches",
		func(response interface{}) error {
			called = true
			result := response.(*params.ErrorResults)
			result.Results = make([]params.ErrorResult, 1)
			return nil
		})

	results, err := s.apiUnit.AddMetricBatches([]params.MetricBatch{batch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[batch.UUID], gc.IsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *unitMetricBatchesSuite) TestSendMetricBatchFail(c *gc.C) {
	var called bool
	uniter.PatchUnitResponse(s, s.apiUnit, "AddMetricBatches",
		func(response interface{}) error {
			called = true
			result := response.(*params.ErrorResults)
			result.Results = make([]params.ErrorResult, 1)
			result.Results[0].Error = common.ServerError(common.ErrPerm)
			return nil
		})
	metrics := []params.Metric{{
		Key: "pings", Value: "5", Time: time.Now().UTC(),
	}, {
		Key: "pongs", Value: "6", Time: time.Now().UTC(), Labels: map[string]string{"foo": "bar"},
	}}
	uuid := utils.MustNewUUID().String()
	batch := params.MetricBatch{
		UUID:     uuid,
		CharmURL: s.charm.URL().String(),
		Created:  time.Now(),
		Metrics:  metrics,
	}

	results, err := s.apiUnit.AddMetricBatches([]params.MetricBatch{batch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[batch.UUID], gc.ErrorMatches, "permission denied")
	c.Assert(called, jc.IsTrue)
}

func (s *unitMetricBatchesSuite) TestSendMetricBatch(c *gc.C) {
	uuid := utils.MustNewUUID().String()
	now := time.Now().Round(time.Second).UTC()
	metrics := []params.Metric{{
		Key: "pings", Value: "5", Time: now,
	}, {
		Key: "pongs", Value: "6", Time: time.Now().UTC(), Labels: map[string]string{"foo": "bar"},
	}}
	batch := params.MetricBatch{
		UUID:     uuid,
		CharmURL: s.charm.URL().String(),
		Created:  now,
		Metrics:  metrics,
	}

	results, err := s.apiUnit.AddMetricBatches([]params.MetricBatch{batch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[batch.UUID], gc.IsNil)

	batches, err := s.State.AllMetricBatches()
	c.Assert(err, gc.IsNil)
	c.Assert(batches, gc.HasLen, 1)
	c.Assert(batches[0].UUID(), gc.Equals, uuid)
	c.Assert(batches[0].Sent(), jc.IsFalse)
	c.Assert(batches[0].CharmURL(), gc.Equals, s.charm.URL().String())
	c.Assert(batches[0].Metrics(), gc.HasLen, 2)
	c.Assert(batches[0].Metrics()[0].Key, gc.Equals, "pings")
	c.Assert(batches[0].Metrics()[0].Value, gc.Equals, "5")
	c.Assert(batches[0].Metrics()[0].Labels, gc.HasLen, 0)
	c.Assert(batches[0].Metrics()[1].Key, gc.Equals, "pongs")
	c.Assert(batches[0].Metrics()[1].Value, gc.Equals, "6")
	c.Assert(batches[0].Metrics()[1].Labels, gc.DeepEquals, map[string]string{"foo": "bar"})
}
