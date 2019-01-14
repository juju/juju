// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"errors"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type InterfaceSuite struct {
	HookContextSuite
	stub testing.Stub
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) TestUnitName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
}

func (s *InterfaceSuite) TestHookRelation(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	r, err := ctx.HookRelation()
	c.Assert(err, gc.ErrorMatches, ".*")
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRemoteUnitName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	name, err := ctx.RemoteUnitName()
	c.Assert(err, gc.ErrorMatches, ".*")
	c.Assert(name, gc.Equals, "")
}

func (s *InterfaceSuite) TestRelationIds(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	relIds, err := ctx.RelationIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relIds, gc.HasLen, 2)
	r, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")
	r, err = ctx.Relation(123)
	c.Assert(err, gc.ErrorMatches, ".*")
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRelationContext(c *gc.C) {
	ctx := s.GetContext(c, 1, "")
	r, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")
}

func (s *InterfaceSuite) TestRelationContextWithRemoteUnitName(c *gc.C) {
	ctx := s.GetContext(c, 1, "u/123")
	name, err := ctx.RemoteUnitName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "u/123")
}

func (s *InterfaceSuite) TestAddingMetricsInWrongContext(c *gc.C) {
	ctx := s.GetContext(c, 1, "u/123")
	err := ctx.AddMetric("key", "123", time.Now())
	c.Assert(err, gc.ErrorMatches, "metrics not allowed in this context")
	err = ctx.AddMetricLabels("key", "123", time.Now(), map[string]string{"foo": "bar"})
	c.Assert(err, gc.ErrorMatches, "metrics not allowed in this context")
}

func (s *InterfaceSuite) TestAvailabilityZone(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	zone, err := ctx.AvailabilityZone()
	c.Check(err, jc.ErrorIsNil)
	c.Check(zone, gc.Equals, "a-zone")
}

func (s *InterfaceSuite) TestUnitNetworkInfo(c *gc.C) {
	// Only the error case is tested to ensure end-to-end integration, the rest
	// of the cases are tested separately for network-get, api/uniter, and
	// apiserver/uniter, respectively.
	ctx := s.GetContext(c, -1, "")
	netInfo, err := ctx.NetworkInfo([]string{"unknown"}, -1)
	c.Check(err, jc.ErrorIsNil)
	c.Check(netInfo, gc.DeepEquals, map[string]params.NetworkInfoResult{
		"unknown": {
			Error: &params.Error{
				Message: "binding name \"unknown\" not defined by the unit's charm",
			},
		},
	},
	)
}

func (s *InterfaceSuite) TestUnitStatus(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	defer context.PatchCachedStatus(ctx.(runner.Context), "maintenance", "working", map[string]interface{}{"hello": "world"})()
	status, err := ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, "maintenance")
	c.Check(status.Info, gc.Equals, "working")
	c.Check(status.Data, gc.DeepEquals, map[string]interface{}{"hello": "world"})
}

func (s *InterfaceSuite) TestSetUnitStatus(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	err := ctx.SetUnitStatus(status)
	c.Check(err, jc.ErrorIsNil)
	unitStatus, err := ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(unitStatus.Status, gc.Equals, "maintenance")
	c.Check(unitStatus.Info, gc.Equals, "doing work")
	c.Check(unitStatus.Data, gc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestSetUnitStatusUpdatesFlag(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	c.Assert(ctx.(runner.Context).HasExecutionSetUnitStatus(), jc.IsFalse)
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	err := ctx.SetUnitStatus(status)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(ctx.(runner.Context).HasExecutionSetUnitStatus(), jc.IsTrue)
}

func (s *InterfaceSuite) TestGetSetWorkloadVersion(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	// No workload version set yet.
	result, err := ctx.UnitWorkloadVersion()
	c.Assert(result, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.SetUnitWorkloadVersion("Pipey")
	c.Assert(err, jc.ErrorIsNil)

	result, err = ctx.UnitWorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "Pipey")
}

func (s *InterfaceSuite) TestUnitStatusCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	unitStatus, err := ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(unitStatus.Status, gc.Equals, "waiting")
	c.Check(unitStatus.Info, gc.Equals, "waiting for machine")
	c.Check(unitStatus.Data, gc.DeepEquals, map[string]interface{}{})

	// Change remote state.
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "it works",
		Since:   &now,
	}
	err = s.unit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	// Local view is unchanged.
	unitStatus, err = ctx.UnitStatus()
	c.Check(err, jc.ErrorIsNil)
	c.Check(unitStatus.Status, gc.Equals, "waiting")
	c.Check(unitStatus.Info, gc.Equals, "waiting for machine")
	c.Check(unitStatus.Data, gc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestUnitCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	pr, err := ctx.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	pa, err := ctx.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	// Initially the public address is the same as the private address since
	// the "most public" address is chosen.
	c.Assert(pr, gc.Equals, pa)

	// Change remote state.
	err = s.machine.SetProviderAddresses(
		network.NewScopedAddress("blah.testing.invalid", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Local view is unchanged.
	pr, err = ctx.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	pa, err = ctx.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pr, gc.Equals, pa)
}

func (s *InterfaceSuite) TestConfigCaching(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	settings, err := ctx.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	// Change remote config.
	err = s.application.UpdateCharmConfig(charm.Settings{
		"blog-title": "Something Else",
	})
	c.Assert(err, jc.ErrorIsNil)

	// Local view is not changed.
	settings, err = ctx.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})
}

func (s *InterfaceSuite) TestGoalState(c *gc.C) {

	timestamp := time.Date(2200, time.November, 5, 0, 0, 0, 0, time.UTC)
	mockUnitSince := func(inUnits application.UnitsGoalState) application.UnitsGoalState {
		outUnits := application.UnitsGoalState{}
		for name, gsStatus := range inUnits {
			c.Assert(gsStatus.Since, gc.NotNil)
			outUnits[name] = application.GoalStateStatus{
				Status: gsStatus.Status,
				Since:  &timestamp,
			}
		}
		return outUnits
	}
	goalStateCheck := application.GoalState{
		Units: application.UnitsGoalState{
			"u/0": application.GoalStateStatus{
				Status: "waiting",
				Since:  &timestamp,
			},
		},
		Relations: map[string]application.UnitsGoalState{
			"server": {
				"db0": application.GoalStateStatus{
					Status: "joining",
					Since:  &timestamp,
				},
				"db1": application.GoalStateStatus{
					Status: "joining",
					Since:  &timestamp,
				},
			},
		},
	}

	ctx := s.GetContext(c, -1, "")
	goalState, err := ctx.GoalState()

	// Mock status Since string
	goalState.Units = mockUnitSince(goalState.Units)
	for relationsNames, relationUnits := range goalState.Relations {
		goalState.Relations[relationsNames] = mockUnitSince(relationUnits)
	}

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(goalState, jc.DeepEquals, &goalStateCheck)
}

// TestNonActionCallsToActionMethodsFail does exactly what its name says:
// it simply makes sure that Action-related calls to HookContexts with a nil
// actionData member error out correctly.
func (s *InterfaceSuite) TestNonActionCallsToActionMethodsFail(c *gc.C) {
	ctx := context.HookContext{}
	_, err := ctx.ActionParams()
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.SetActionFailed()
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.SetActionMessage("foo")
	c.Check(err, gc.ErrorMatches, "not running an action")
	err = ctx.UpdateActionResults([]string{"1", "2", "3"}, "value")
	c.Check(err, gc.ErrorMatches, "not running an action")
}

// TestUpdateActionResults demonstrates that UpdateActionResults functions
// as expected.
func (s *InterfaceSuite) TestUpdateActionResults(c *gc.C) {
	tests := []struct {
		initial  map[string]interface{}
		keys     []string
		value    string
		expected map[string]interface{}
	}{{
		initial: map[string]interface{}{},
		keys:    []string{"foo"},
		value:   "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}, {
		initial: map[string]interface{}{
			"foo": "bar",
		},
		keys:  []string{"foo", "bar"},
		value: "baz",
		expected: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
	}, {
		initial: map[string]interface{}{
			"foo": map[string]interface{}{
				"bar": "baz",
			},
		},
		keys:  []string{"foo"},
		value: "bar",
		expected: map[string]interface{}{
			"foo": "bar",
		},
	}}

	for i, t := range tests {
		c.Logf("UpdateActionResults test %d: %#v: %#v", i, t.keys, t.value)
		hctx := context.GetStubActionContext(t.initial)
		err := hctx.UpdateActionResults(t.keys, t.value)
		c.Assert(err, jc.ErrorIsNil)
		actionData, err := hctx.ActionData()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actionData.ResultsMap, jc.DeepEquals, t.expected)
	}
}

// TestSetActionFailed ensures SetActionFailed works properly.
func (s *InterfaceSuite) TestSetActionFailed(c *gc.C) {
	hctx := context.GetStubActionContext(nil)
	err := hctx.SetActionFailed()
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionData.Failed, jc.IsTrue)
}

// TestSetActionMessage ensures SetActionMessage works properly.
func (s *InterfaceSuite) TestSetActionMessage(c *gc.C) {
	hctx := context.GetStubActionContext(nil)
	err := hctx.SetActionMessage("because reasons")
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Check(err, jc.ErrorIsNil)
	c.Check(actionData.ResultsMessage, gc.Equals, "because reasons")
}

func (s *InterfaceSuite) TestRequestRebootAfterHook(c *gc.C) {
	var killed bool
	p := &mockProcess{func() error {
		killed = true
		return nil
	}}
	ctx := s.GetContext(c, -1, "").(*context.HookContext)
	ctx.SetProcess(p)
	err := ctx.RequestReboot(jujuc.RebootAfterHook)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(killed, jc.IsFalse)
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootAfterHook)
}

func (s *InterfaceSuite) TestRequestRebootNow(c *gc.C) {
	ctx := s.GetContext(c, -1, "").(*context.HookContext)

	var stub testing.Stub
	var p *mockProcess
	p = &mockProcess{func() error {
		// Reboot priority should be set before the process
		// is killed, or else the client waiting for the
		// process to exit will race with the setting of
		// the priority.
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootNow)
		return stub.NextErr()
	}}
	stub.SetErrors(errors.New("process is already dead"))
	ctx.SetProcess(p)

	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, jc.ErrorIsNil)

	// Everything went well, so priority should still be RebootNow.
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootNow)
}

func (s *InterfaceSuite) TestRequestRebootNowTimeout(c *gc.C) {
	ctx := s.GetContext(c, -1, "").(*context.HookContext)

	var advanced bool
	var p *mockProcess
	p = &mockProcess{func() error {
		// Reboot priority should be set before the process
		// is killed, or else the client waiting for the
		// process to exit will race with the setting of
		// the priority.
		priority := ctx.GetRebootPriority()
		c.Assert(priority, gc.Equals, jujuc.RebootNow)
		if !advanced {
			advanced = true
			s.clock.Advance(time.Hour) // force timeout
		}
		return nil
	}}
	ctx.SetProcess(p)

	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, gc.ErrorMatches, "failed to kill context process 123")

	// RequestReboot failed, so priority should revert to RebootSkip.
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootSkip)
}

func (s *InterfaceSuite) TestRequestRebootNowNoProcess(c *gc.C) {
	// A normal hook run or a juju-run command will record the *os.Process
	// object of the running command, in HookContext. When requesting a
	// reboot with the --now flag, the process is killed and only
	// then will we set the reboot priority. This test basically simulates
	// the case when the process calling juju-reboot is not recorded.
	ctx := context.HookContext{}
	err := ctx.RequestReboot(jujuc.RebootNow)
	c.Assert(err, gc.ErrorMatches, "no process to kill")
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootNow)
}

func (s *InterfaceSuite) TestStorageAddConstraints(c *gc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": {
			params.StorageConstraints{},
		},
	}

	ctx := context.HookContext{}
	addStorageToContext(&ctx, "data", params.StorageConstraints{})
	assertStorageAddInContext(c, ctx, expected)
}

var two = uint64(2)

func (s *InterfaceSuite) TestStorageAddConstraintsSameStorage(c *gc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": {
			params.StorageConstraints{},
			params.StorageConstraints{Count: &two},
		},
	}

	ctx := context.HookContext{}
	addStorageToContext(&ctx, "data", params.StorageConstraints{})
	addStorageToContext(&ctx, "data", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func (s *InterfaceSuite) TestStorageAddConstraintsDifferentStorage(c *gc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": {params.StorageConstraints{}},
		"diff": {
			params.StorageConstraints{Count: &two}},
	}

	ctx := context.HookContext{}
	addStorageToContext(&ctx, "data", params.StorageConstraints{})
	addStorageToContext(&ctx, "diff", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func addStorageToContext(ctx *context.HookContext,
	name string,
	cons params.StorageConstraints,
) {
	addOne := map[string]params.StorageConstraints{name: cons}
	ctx.AddUnitStorage(addOne)
}

func assertStorageAddInContext(c *gc.C,
	ctx context.HookContext, expected map[string][]params.StorageConstraints,
) {
	obtained := context.StorageAddConstraints(&ctx)
	c.Assert(len(obtained), gc.Equals, len(expected))
	for k, v := range obtained {
		c.Assert(v, jc.SameContents, expected[k])
	}
}

type mockProcess struct {
	kill func() error
}

func (p *mockProcess) Kill() error {
	return p.kill()
}

func (p *mockProcess) Pid() int {
	return 123
}
