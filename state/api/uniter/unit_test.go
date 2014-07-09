// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"sort"

	"github.com/juju/charm"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/uniter"
	statetesting "github.com/juju/juju/state/testing"
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
	c.Assert(err, gc.IsNil)
}

func (s *unitSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *unitSuite) TestUnitAndUnitTag(c *gc.C) {
	apiUnitFoo, err := s.uniter.Unit(names.NewUnitTag("foo/42"))
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(err, jc.Satisfies, params.IsCodeUnauthorized)
	c.Assert(apiUnitFoo, gc.IsNil)

	c.Assert(s.apiUnit.Tag(), gc.Equals, "unit-wordpress-0")
}

func (s *unitSuite) TestSetStatus(c *gc.C) {
	status, info, data, err := s.wordpressUnit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusPending)
	c.Assert(info, gc.Equals, "")
	c.Assert(data, gc.HasLen, 0)

	err = s.apiUnit.SetStatus(params.StatusStarted, "blah", nil)
	c.Assert(err, gc.IsNil)

	status, info, data, err = s.wordpressUnit.Status()
	c.Assert(err, gc.IsNil)
	c.Assert(status, gc.Equals, params.StatusStarted)
	c.Assert(info, gc.Equals, "blah")
	c.Assert(data, gc.HasLen, 0)
}

func (s *unitSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	err := s.apiUnit.EnsureDead()
	c.Assert(err, gc.IsNil)

	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)

	err = s.apiUnit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)

	err = s.wordpressUnit.Remove()
	c.Assert(err, gc.IsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	err = s.apiUnit.EnsureDead()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" not found`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)
}

func (s *unitSuite) TestDestroy(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	err := s.apiUnit.Destroy()
	c.Assert(err, gc.IsNil)

	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.ErrorMatches, `unit "wordpress/0" not found`)
}

func (s *unitSuite) TestDestroyAllSubordinates(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	// Call without subordinates - no change.
	err := s.apiUnit.DestroyAllSubordinates()
	c.Assert(err, gc.IsNil)

	// Add a couple of subordinates and try again.
	_, _, loggingSub := s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)
	_, _, monitoringSub := s.addRelatedService(c, "wordpress", "monitoring", s.wordpressUnit)
	c.Assert(loggingSub.Life(), gc.Equals, state.Alive)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Alive)

	err = s.apiUnit.DestroyAllSubordinates()
	c.Assert(err, gc.IsNil)

	// Verify they got destroyed.
	err = loggingSub.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(loggingSub.Life(), gc.Equals, state.Dying)
	err = monitoringSub.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Dying)
}

func (s *unitSuite) TestRefresh(c *gc.C) {
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	err := s.apiUnit.EnsureDead()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	err = s.apiUnit.Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Dead)
}

func (s *unitSuite) TestWatch(c *gc.C) {
	c.Assert(s.apiUnit.Life(), gc.Equals, params.Alive)

	w, err := s.apiUnit.Watch()
	c.Assert(err, gc.IsNil)
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Change something other than the lifecycle and make sure it's
	// not detected.
	err = s.apiUnit.SetStatus(params.StatusStarted, "not really", nil)
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// Make the unit dead and check it's detected.
	err = s.apiUnit.EnsureDead()
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitSuite) TestResolve(c *gc.C) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, gc.IsNil)

	mode, err := s.apiUnit.Resolved()
	c.Assert(err, gc.IsNil)
	c.Assert(mode, gc.Equals, params.ResolvedRetryHooks)

	err = s.apiUnit.ClearResolved()
	c.Assert(err, gc.IsNil)

	mode, err = s.apiUnit.Resolved()
	c.Assert(err, gc.IsNil)
	c.Assert(mode, gc.Equals, params.ResolvedNone)
}

func (s *unitSuite) TestIsPrincipal(c *gc.C) {
	ok, err := s.apiUnit.IsPrincipal()
	c.Assert(err, gc.IsNil)
	c.Assert(ok, jc.IsTrue)
}

func (s *unitSuite) TestHasSubordinates(c *gc.C) {
	found, err := s.apiUnit.HasSubordinates()
	c.Assert(err, gc.IsNil)
	c.Assert(found, jc.IsFalse)

	// Add a couple of subordinates and try again.
	s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)
	s.addRelatedService(c, "wordpress", "monitoring", s.wordpressUnit)

	found, err = s.apiUnit.HasSubordinates()
	c.Assert(err, gc.IsNil)
	c.Assert(found, jc.IsTrue)
}

func (s *unitSuite) TestPublicAddress(c *gc.C) {
	address, err := s.apiUnit.PublicAddress()
	c.Assert(err, gc.ErrorMatches, `"unit-wordpress-0" has no public address set`)

	err = s.wordpressMachine.SetAddresses(network.NewAddress("1.2.3.4", network.ScopePublic))
	c.Assert(err, gc.IsNil)

	address, err = s.apiUnit.PublicAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(address, gc.Equals, "1.2.3.4")
}

func (s *unitSuite) TestPrivateAddress(c *gc.C) {
	address, err := s.apiUnit.PrivateAddress()
	c.Assert(err, gc.ErrorMatches, `"unit-wordpress-0" has no private address set`)

	err = s.wordpressMachine.SetAddresses(network.NewAddress("1.2.3.4", network.ScopeCloudLocal))
	c.Assert(err, gc.IsNil)

	address, err = s.apiUnit.PrivateAddress()
	c.Assert(err, gc.IsNil)
	c.Assert(address, gc.Equals, "1.2.3.4")
}

func (s *unitSuite) TestOpenClosePort(c *gc.C) {
	ports := s.wordpressUnit.OpenedPorts()
	c.Assert(ports, gc.HasLen, 0)

	err := s.apiUnit.OpenPort("foo", 1234)
	c.Assert(err, gc.IsNil)
	err = s.apiUnit.OpenPort("bar", 4321)
	c.Assert(err, gc.IsNil)

	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	ports = s.wordpressUnit.OpenedPorts()
	// OpenedPorts returns a sorted slice.
	c.Assert(ports, gc.DeepEquals, []network.Port{
		{Protocol: "bar", Number: 4321},
		{Protocol: "foo", Number: 1234},
	})

	err = s.apiUnit.ClosePort("bar", 4321)
	c.Assert(err, gc.IsNil)

	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	ports = s.wordpressUnit.OpenedPorts()
	// OpenedPorts returns a sorted slice.
	c.Assert(ports, gc.DeepEquals, []network.Port{
		{Protocol: "foo", Number: 1234},
	})

	err = s.apiUnit.ClosePort("foo", 1234)
	c.Assert(err, gc.IsNil)

	err = s.wordpressUnit.Refresh()
	c.Assert(err, gc.IsNil)
	ports = s.wordpressUnit.OpenedPorts()
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
	c.Assert(err, gc.IsNil)

	curl, err = s.apiUnit.CharmURL()
	c.Assert(err, gc.IsNil)
	c.Assert(curl, gc.NotNil)
	c.Assert(curl.String(), gc.Equals, s.wordpressCharm.String())
}

func (s *unitSuite) TestConfigSettings(c *gc.C) {
	// Make sure ConfigSettings returns an error when
	// no charm URL is set, as its state counterpart does.
	settings, err := s.apiUnit.ConfigSettings()
	c.Assert(err, gc.ErrorMatches, "unit charm not set")

	// Now set the charm and try again.
	err = s.apiUnit.SetCharmURL(s.wordpressCharm.URL())
	c.Assert(err, gc.IsNil)

	settings, err = s.apiUnit.ConfigSettings()
	c.Assert(err, gc.IsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"blog-title": "My Title",
	})

	// Update the config and check we get the changes on the next call.
	err = s.wordpressService.UpdateConfigSettings(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, gc.IsNil)

	settings, err = s.apiUnit.ConfigSettings()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)

	w, err = s.apiUnit.WatchConfigSettings()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = s.wordpressService.UpdateConfigSettings(charm.Settings{
		"blog-title": "superhero paparazzi",
	})
	c.Assert(err, gc.IsNil)
	err = s.wordpressService.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.wordpressService.UpdateConfigSettings(charm.Settings{
		"blog-title": "sauceror central",
	})
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// NOTE: This test is not as exhaustive as the one in state,
	// because the watcher is already tested there. Here we just
	// ensure we get the events when we expect them and don't get
	// them when they're not expected.

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitSuite) TestWatchActions(c *gc.C) {
	w, err := s.apiUnit.WatchActions()
	c.Assert(err, gc.IsNil)

	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertChange()

	// Add a couple of actions and make sure the changes are detected.
	action, err := s.wordpressUnit.AddAction("snapshot", map[string]interface{}{
		"outfile": "foo.txt",
	})
	c.Assert(err, gc.IsNil)
	wc.AssertChange(action.Id())

	action, err = s.wordpressUnit.AddAction("backup", map[string]interface{}{
		"outfile": "foo.bz2",
		"compression": map[string]interface{}{
			"kind":    "bzip",
			"quality": float64(5.0),
		},
	})
	c.Assert(err, gc.IsNil)
	wc.AssertChange(action.Id())

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitSuite) TestWatchActionsError(c *gc.C) {
	restore := testing.PatchValue(uniter.Call, func(st *uniter.State, method string, params, results interface{}) error {
		return fmt.Errorf("Test error")
	})

	defer restore()

	_, err := s.apiUnit.WatchActions()
	c.Assert(err.Error(), gc.Equals, "Test error")
}

func (s *unitSuite) TestWatchActionsErrorResults(c *gc.C) {
	restore := testing.PatchValue(uniter.Call, func(st *uniter.State, method string, args, results interface{}) error {
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
	})

	defer restore()

	_, err := s.apiUnit.WatchActions()
	c.Assert(err.Error(), gc.Equals, "An error in the watch result.")
}

func (s *unitSuite) TestWatchActionsNoResults(c *gc.C) {
	restore := testing.PatchValue(uniter.Call, func(st *uniter.State, method string, params, results interface{}) error {
		return nil
	})

	defer restore()

	_, err := s.apiUnit.WatchActions()
	c.Assert(err.Error(), gc.Equals, "expected 1 result, got 0")
}

func (s *unitSuite) TestWatchActionsMoreResults(c *gc.C) {
	restore := testing.PatchValue(uniter.Call, func(st *uniter.State, method string, args, results interface{}) error {
		if results, ok := results.(*params.StringsWatchResults); ok {
			results.Results = make([]params.StringsWatchResult, 2)
		}
		return nil
	})

	defer restore()

	_, err := s.apiUnit.WatchActions()
	c.Assert(err.Error(), gc.Equals, "expected 1 result, got 2")
}

func (s *unitSuite) TestServiceNameAndTag(c *gc.C) {
	c.Assert(s.apiUnit.ServiceName(), gc.Equals, "wordpress")
	c.Assert(s.apiUnit.ServiceTag(), gc.Equals, "service-wordpress")
}

func (s *unitSuite) TestJoinedRelations(c *gc.C) {
	joinedRelations, err := s.apiUnit.JoinedRelations()
	c.Assert(err, gc.IsNil)
	c.Assert(joinedRelations, gc.HasLen, 0)

	rel1, _, _ := s.addRelatedService(c, "wordpress", "monitoring", s.wordpressUnit)
	joinedRelations, err = s.apiUnit.JoinedRelations()
	c.Assert(err, gc.IsNil)
	c.Assert(joinedRelations, gc.DeepEquals, []string{rel1.Tag().String()})

	rel2, _, _ := s.addRelatedService(c, "wordpress", "logging", s.wordpressUnit)
	joinedRelations, err = s.apiUnit.JoinedRelations()
	c.Assert(err, gc.IsNil)
	sort.Strings(joinedRelations)
	c.Assert(joinedRelations, gc.DeepEquals, []string{rel2.Tag().String(), rel1.Tag().String()})
}

func (s *unitSuite) TestWatchAddresses(c *gc.C) {
	w, err := s.apiUnit.WatchAddresses()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewNotifyWatcherC(c, s.BackingState, w)

	// Initial event.
	wc.AssertOneChange()

	// Update config a couple of times, check a single event.
	err = s.wordpressMachine.SetAddresses(network.NewAddress("0.1.2.3", network.ScopeUnknown))
	c.Assert(err, gc.IsNil)
	err = s.wordpressMachine.SetAddresses(network.NewAddress("0.1.2.4", network.ScopeUnknown))
	c.Assert(err, gc.IsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.wordpressMachine.SetAddresses(network.NewAddress("0.1.2.4", network.ScopeUnknown))
	c.Assert(err, gc.IsNil)
	wc.AssertNoChange()

	// NOTE: This test is not as exhaustive as the one in state,
	// because the watcher is already tested there. Here we just
	// ensure we get the events when we expect them and don't get
	// them when they're not expected.

	statetesting.AssertStop(c, w)
	wc.AssertClosed()
}

func (s *unitSuite) TestWatchAddressesErrors(c *gc.C) {
	err := s.wordpressUnit.UnassignFromMachine()
	c.Assert(err, gc.IsNil)
	_, err = s.apiUnit.WatchAddresses()
	c.Assert(err, jc.Satisfies, params.IsCodeNotAssigned)
}
