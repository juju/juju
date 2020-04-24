// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/quota"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/context/mocks"
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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRemoteUnitName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	name, err := ctx.RemoteUnitName()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(name, gc.Equals, "")
}

func (s *InterfaceSuite) TestRemoteApplicationName(c *gc.C) {
	ctx := s.GetContext(c, -1, "")
	name, err := ctx.RemoteApplicationName()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
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
		network.NewScopedSpaceAddress("blah.testing.invalid", network.ScopePublic),
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
	err = s.application.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "Something Else"})
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
			"db": {
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
	err = ctx.LogActionMessage("foo")
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
		hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "")
		context.WithActionContext(hctx, t.initial, nil)
		err := hctx.UpdateActionResults(t.keys, t.value)
		c.Assert(err, jc.ErrorIsNil)
		actionData, err := hctx.ActionData()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actionData.ResultsMap, jc.DeepEquals, t.expected)
	}
}

// TestSetActionFailed ensures SetActionFailed works properly.
func (s *InterfaceSuite) TestSetActionFailed(c *gc.C) {
	hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "")
	context.WithActionContext(hctx, nil, nil)
	err := hctx.SetActionFailed()
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionData.Failed, jc.IsTrue)
}

// TestSetActionMessage ensures SetActionMessage works properly.
func (s *InterfaceSuite) TestSetActionMessage(c *gc.C) {
	hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "")
	context.WithActionContext(hctx, nil, nil)
	err := hctx.SetActionMessage("because reasons")
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Check(err, jc.ErrorIsNil)
	c.Check(actionData.ResultsMessage, gc.Equals, "because reasons")
}

func (s *InterfaceSuite) toSupportNewActionID(c *gc.C) {
	ver, err := s.Model.AgentVersion()
	c.Assert(err, jc.ErrorIsNil)

	if !state.IsNewActionIDSupported(ver) {
		err := s.State.SetModelAgentVersion(state.MinVersionSupportNewActionID, true)
		c.Assert(err, jc.ErrorIsNil)
	}
}

// TestLogActionMessage ensures LogActionMessage works properly.
func (s *InterfaceSuite) TestLogActionMessage(c *gc.C) {
	s.toSupportNewActionID(c)
	operationID, err := s.Model.EnqueueOperation("a test")
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.unit.AddAction(operationID, "fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = action.Begin()
	c.Assert(err, jc.ErrorIsNil)

	hctx := s.getHookContext(c, s.State.ModelUUID(), -1, "")
	context.WithActionContext(hctx, nil, nil)
	err = hctx.LogActionMessage("hello world")
	c.Assert(err, jc.ErrorIsNil)
	a, err := s.Model.Action(action.Id())
	c.Assert(err, jc.ErrorIsNil)
	messages := a.Messages()
	c.Assert(messages, gc.HasLen, 1)
	c.Assert(messages[0].Message(), gc.Equals, "hello world")
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
	ctx := &context.HookContext{}
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

	ctx := &context.HookContext{}
	addStorageToContext(ctx, "data", params.StorageConstraints{})
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

	ctx := &context.HookContext{}
	addStorageToContext(ctx, "data", params.StorageConstraints{})
	addStorageToContext(ctx, "data", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func (s *InterfaceSuite) TestStorageAddConstraintsDifferentStorage(c *gc.C) {
	expected := map[string][]params.StorageConstraints{
		"data": {params.StorageConstraints{}},
		"diff": {
			params.StorageConstraints{Count: &two}},
	}

	ctx := &context.HookContext{}
	addStorageToContext(ctx, "data", params.StorageConstraints{})
	addStorageToContext(ctx, "diff", params.StorageConstraints{Count: &two})
	assertStorageAddInContext(c, ctx, expected)
}

func addStorageToContext(ctx *context.HookContext,
	name string,
	cons params.StorageConstraints,
) {
	addOne := map[string]params.StorageConstraints{name: cons}
	_ = ctx.AddUnitStorage(addOne)
}

func assertStorageAddInContext(c *gc.C,
	ctx *context.HookContext, expected map[string][]params.StorageConstraints,
) {
	obtained := context.StorageAddConstraints(ctx)
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

var _ = gc.Suite(&mockHookContextSuite{})

type mockHookContextSuite struct {
	mockUnit  *mocks.MockHookUnit
	mockCache params.UnitStateResult
}

func (s *mockHookContextSuite) TestDeleteCharmStateValue(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	err := hookContext.DeleteCharmStateValue("one")
	c.Assert(err, jc.ErrorIsNil)

	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCache, gc.DeepEquals, s.mockCache.CharmState)
}

func (s *mockHookContextSuite) TestDeleteCacheStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	err := hookContext.DeleteCharmStateValue("five")
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestGetCharmState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCache, gc.DeepEquals, s.mockCache.CharmState)
}

func (s *mockHookContextSuite) TestGetCharmStateStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	_, err := hookContext.GetCharmState()
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestGetCharmStateValue(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	obtainedVale, err := hookContext.GetCharmStateValue("one")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVale, gc.Equals, "two")
}

func (s *mockHookContextSuite) TestGetCharmStateValueEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	obtainedVale, err := hookContext.GetCharmStateValue("seven")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVale, gc.Equals, "")
}

func (s *mockHookContextSuite) TestGetCharmStateValueNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	obtainedCache, err := hookContext.GetCharmStateValue("five")
	c.Assert(err, gc.ErrorMatches, "\"five\" not found")
	c.Assert(obtainedCache, gc.Equals, "")
}

func (s *mockHookContextSuite) TestGetCharmStateValueStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	_, err := hookContext.GetCharmStateValue("key")
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestSetCacheQuotaLimits(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	s.testSetCache(c)
}

func (s *mockHookContextSuite) TestSetCache(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit)

	// Test key len limit
	err := hookContext.SetCharmStateValue(
		strings.Repeat("a", quota.MaxCharmStateKeySize+1),
		"lol",
	)
	c.Assert(err, jc.Satisfies, errors.IsQuotaLimitExceeded)
	c.Assert(err, gc.ErrorMatches, ".*max allowed key.*")

	// Test value len limit
	err = hookContext.SetCharmStateValue(
		"lol",
		strings.Repeat("a", quota.MaxCharmStateValueSize+1),
	)
	c.Assert(err, jc.Satisfies, errors.IsQuotaLimitExceeded)
	c.Assert(err, gc.ErrorMatches, ".*max allowed value.*")
}

func (s *mockHookContextSuite) TestSetCacheEmptyStartState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, nil)

	s.testSetCache(c)
}

func (s *mockHookContextSuite) testSetCache(c *gc.C) {
	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	err := hookContext.SetCharmStateValue("five", "six")
	c.Assert(err, jc.ErrorIsNil)
	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, jc.ErrorIsNil)
	value, ok := obtainedCache["five"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "six")
}

func (s *mockHookContextSuite) TestSetCacheStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	err := hookContext.SetCharmStateValue("five", "six")
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *mockHookContextSuite) TestFlushWithNonDirtyCache(c *gc.C) {
	defer s.setupMocks(c).Finish()
	hookContext := context.NewMockUnitHookContext(s.mockUnit)
	s.expectStateValues()
	s.mockUnit.EXPECT().Tag().Return(names.NewUnitTag("wordpress/0"))

	// The following commands are no-ops as they don't mutate the cache.
	err := hookContext.SetCharmStateValue("one", "two") // no-op: KV already present
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.DeleteCharmStateValue("not-there") // no-op: key not present
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with a success. As the cache is not dirty we do
	// not expect a SetState call.
	err = hookContext.Flush("success", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mockHookContextSuite) TestSequentialFlushOfCacheValues(c *gc.C) {
	defer s.setupMocks(c).Finish()
	hookContext := context.NewMockUnitHookContext(s.mockUnit)

	// We expect a single call for the following API endpoints
	s.expectStateValues()
	s.mockUnit.EXPECT().Tag().Return(names.NewUnitTag("wordpress/0")).Times(2)
	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				SetUnitState: &params.SetUnitStateArg{
					Tag: "unit-wordpress-0",
					CharmState: &map[string]string{
						"one":   "two",
						"three": "four",
						"lorem": "ipsum",
						"seven": "",
					},
				},
			},
		},
	}).Return(nil)

	// Mutate cache and flush; this should call out to SetState and reset
	// the dirty flag
	err := hookContext.SetCharmStateValue("lorem", "ipsum")
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.Flush("success", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Flush again; as the cache is not dirty, the SetState call is skipped.
	err = hookContext.Flush("success", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *mockHookContextSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockUnit = mocks.NewMockHookUnit(ctrl)
	return ctrl
}

func (s *mockHookContextSuite) expectStateValues() {
	s.mockCache = params.UnitStateResult{
		CharmState: map[string]string{
			"one":   "two",
			"three": "four",
			"seven": "",
		}}
	s.mockUnit.EXPECT().State().Return(s.mockCache, nil)
}

func (s *mockHookContextSuite) TestActionAbort(c *gc.C) {
	tests := []struct {
		Status string
		Failed bool
		Cancel bool
	}{
		{Status: "aborted", Failed: true, Cancel: true},
		{Status: "failed", Failed: true, Cancel: false},
		{Status: "aborted", Failed: false, Cancel: true},
		{Status: "failed", Failed: false, Cancel: false},
	}
	for _, test := range tests {
		mocks := s.setupMocks(c)
		apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
			c.Assert(objType, gc.Equals, "Uniter")
			c.Assert(version, gc.Equals, 0)
			c.Assert(id, gc.Equals, "")
			c.Assert(request, gc.Equals, "FinishActions")
			c.Assert(arg, gc.DeepEquals, params.ActionExecutionResults{
				Results: []params.ActionExecutionResult{{
					ActionTag: "action-2",
					Status:    test.Status,
					Message:   "failed yo",
				}}})
			c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
			*(result.(*params.ErrorResults)) = params.ErrorResults{
				Results: []params.ErrorResult{{}},
			}
			return nil
		})
		state := uniter.NewState(apiCaller, names.NewUnitTag("mysql/0"))
		hookContext := context.NewMockUnitHookContextWithState(s.mockUnit, state)
		cancel := make(chan struct{})
		if test.Cancel {
			close(cancel)
		}
		context.WithActionContext(hookContext, nil, cancel)
		if test.Failed {
			err := hookContext.SetActionFailed()
			c.Assert(err, jc.ErrorIsNil)
		}
		actionData, err := hookContext.ActionData()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(actionData.Failed, gc.Equals, test.Failed)
		err = hookContext.Flush("", errors.Errorf("failed yo"))
		c.Assert(err, jc.ErrorIsNil)
		mocks.Finish()
	}
}
