// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	stdcontext "context"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/charm"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/agent/uniter"
	apiuniter "github.com/juju/juju/api/agent/uniter"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/quota"
	coresecrets "github.com/juju/juju/core/secrets"
	status2 "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/vault"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/context/mocks"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type InterfaceSuite struct {
	BaseHookContextSuite
}

var _ = gc.Suite(&InterfaceSuite{})

func (s *InterfaceSuite) TestUnitName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	c.Assert(ctx.UnitName(), gc.Equals, "u/0")
}

func (s *InterfaceSuite) TestHookRelation(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	r, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRemoteUnitName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	name, err := ctx.RemoteUnitName()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(name, gc.Equals, "")
}

func (s *InterfaceSuite) TestRemoteApplicationName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	name, err := ctx.RemoteApplicationName()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(name, gc.Equals, "")
}

func (s *InterfaceSuite) TestWorkloadName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	name, err := ctx.WorkloadName()
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(name, gc.Equals, "")
}

func (s *InterfaceSuite) TestRelationIds(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.AddContextRelation(c, ctrl, "db")
	s.AddContextRelation(c, ctrl, "db1")
	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	relIds, err := ctx.RelationIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relIds, gc.HasLen, 2)
	r, err := ctx.Relation(0)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:0")
	r, err = ctx.Relation(123)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	c.Assert(r, gc.IsNil)
}

func (s *InterfaceSuite) TestRelationContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.AddContextRelation(c, ctrl, "db")
	s.AddContextRelation(c, ctrl, "db1")
	ctx := s.GetContext(c, ctrl, 1, "", names.StorageTag{})

	r, err := ctx.HookRelation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(r.Name(), gc.Equals, "db")
	c.Assert(r.FakeId(), gc.Equals, "db:1")
}

func (s *InterfaceSuite) TestRelationContextWithRemoteUnitName(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	s.AddContextRelation(c, ctrl, "db")
	s.AddContextRelation(c, ctrl, "db1")
	ctx := s.GetContext(c, ctrl, 1, "u/123", names.StorageTag{})
	name, err := ctx.RemoteUnitName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "u/123")
}

func (s *InterfaceSuite) TestAvailabilityZone(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	zone, err := ctx.AvailabilityZone()
	c.Check(err, jc.ErrorIsNil)
	c.Check(zone, gc.Equals, "a-zone")
}

func (s *InterfaceSuite) TestUnitNetworkInfo(c *gc.C) {
	// Only the error case is tested to ensure end-to-end integration, the rest
	// of the cases are tested separately for network-get, api/uniter, and
	// apiserver/uniter, respectively.
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})

	result := map[string]params.NetworkInfoResult{
		"unknown": {
			Error: &params.Error{
				Message: `undefined for unit charm: endpoint "unknown" not valid`,
				Code:    params.CodeNotValid,
			},
		},
	}
	s.unit.EXPECT().NetworkInfo([]string{"unknown"}, nil).Return(result, nil)

	netInfo, err := ctx.NetworkInfo([]string{"unknown"}, -1)
	c.Check(err, jc.ErrorIsNil)
	c.Check(netInfo, gc.DeepEquals, result)
}

func (s *InterfaceSuite) TestUnitStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	defer context.PatchCachedStatus(ctx.(context.Context), "maintenance", "working", map[string]interface{}{"hello": "world"})()
	status, err := ctx.UnitStatus(stdcontext.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(status.Status, gc.Equals, "maintenance")
	c.Check(status.Info, gc.Equals, "working")
	c.Check(status.Data, gc.DeepEquals, map[string]interface{}{"hello": "world"})
}

func (s *InterfaceSuite) TestSetUnitStatus(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	s.unit.EXPECT().SetUnitStatus(gomock.Any(), status2.Maintenance, "doing work", nil).Return(nil)
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	err := ctx.SetUnitStatus(stdcontext.Background(), status)
	c.Check(err, jc.ErrorIsNil)

	s.unit.EXPECT().UnitStatus(stdcontext.Background()).Return(params.StatusResult{
		Status: "maintenance",
		Info:   "doing work",
		Data:   map[string]interface{}{},
	}, nil)
	unitStatus, err := ctx.UnitStatus(stdcontext.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(unitStatus.Status, gc.Equals, "maintenance")
	c.Check(unitStatus.Info, gc.Equals, "doing work")
	c.Check(unitStatus.Data, gc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestSetUnitStatusUpdatesFlag(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	c.Assert(ctx.(context.Context).HasExecutionSetUnitStatus(), jc.IsFalse)
	status := jujuc.StatusInfo{
		Status: "maintenance",
		Info:   "doing work",
	}
	s.unit.EXPECT().SetUnitStatus(gomock.Any(), status2.Maintenance, "doing work", nil).Return(nil)
	err := ctx.SetUnitStatus(stdcontext.Background(), status)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(ctx.(context.Context).HasExecutionSetUnitStatus(), jc.IsTrue)
}

func (s *InterfaceSuite) TestGetSetWorkloadVersion(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	s.uniter.EXPECT().UnitWorkloadVersion(gomock.Any(), s.unit.Tag()).Return("", nil)

	// No workload version set yet.
	result, err := ctx.UnitWorkloadVersion(stdcontext.Background())
	c.Assert(result, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)

	s.uniter.EXPECT().SetUnitWorkloadVersion(gomock.Any(), s.unit.Tag(), "Pipey").Return(nil)
	err = ctx.SetUnitWorkloadVersion(stdcontext.Background(), "Pipey")
	c.Assert(err, jc.ErrorIsNil)

	// Second call does not hit backend.
	s.uniter.EXPECT().UnitWorkloadVersion(gomock.Any(), s.unit.Tag()).Return("Pipey", nil)
	result, err = ctx.UnitWorkloadVersion(stdcontext.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, "Pipey")
}

func (s *InterfaceSuite) TestUnitStatusCaching(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	s.unit.EXPECT().UnitStatus(gomock.Any()).Return(params.StatusResult{
		Status: "waiting",
		Info:   "waiting for machine",
		Data:   map[string]interface{}{},
	}, nil)
	unitStatus, err := ctx.UnitStatus(stdcontext.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(unitStatus.Status, gc.Equals, "waiting")
	c.Check(unitStatus.Info, gc.Equals, "waiting for machine")
	c.Check(unitStatus.Data, gc.DeepEquals, map[string]interface{}{})

	// Second call does not hit backend.
	unitStatus, err = ctx.UnitStatus(stdcontext.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(unitStatus.Status, gc.Equals, "waiting")
	c.Check(unitStatus.Info, gc.Equals, "waiting for machine")
	c.Check(unitStatus.Data, gc.DeepEquals, map[string]interface{}{})
}

func (s *InterfaceSuite) TestUnitCaching(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	pr, err := ctx.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
	pa, err := ctx.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)
	// Initially the public address is the same as the private address since
	// the "most public" address is chosen.
	c.Assert(pr, gc.Equals, pa)

	// Second call does not hit backend.
	pr, err = ctx.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pr, gc.Equals, "u-0.testing.invalid")
}

func (s *InterfaceSuite) TestConfigCaching(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	cfg := charm.Settings{"blog-title": "My Title"}
	s.unit.EXPECT().ConfigSettings().Return(cfg, nil)

	settings, err := ctx.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, cfg)

	// Second call does not hit backend.
	settings, err = ctx.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, cfg)
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

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	s.uniter.EXPECT().GoalState(gomock.Any()).Return(goalStateCheck, nil)
	goalState, err := ctx.GoalState(stdcontext.Background())

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
		ctrl := gomock.NewController(c)
		c.Logf("UpdateActionResults test %d: %#v: %#v", i, t.keys, t.value)
		hctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
		context.WithActionContext(hctx, t.initial, nil)
		err := hctx.UpdateActionResults(t.keys, t.value)
		c.Assert(err, jc.ErrorIsNil)
		actionData, err := hctx.ActionData()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(actionData.ResultsMap, jc.DeepEquals, t.expected)
		ctrl.Finish()
	}
}

// TestSetActionFailed ensures SetActionFailed works properly.
func (s *InterfaceSuite) TestSetActionFailed(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
	context.WithActionContext(hctx, nil, nil)
	err := hctx.SetActionFailed()
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(actionData.Failed, jc.IsTrue)
}

// TestSetActionMessage ensures SetActionMessage works properly.
func (s *InterfaceSuite) TestSetActionMessage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
	context.WithActionContext(hctx, nil, nil)
	err := hctx.SetActionMessage("because reasons")
	c.Assert(err, jc.ErrorIsNil)
	actionData, err := hctx.ActionData()
	c.Check(err, jc.ErrorIsNil)
	c.Check(actionData.ResultsMessage, gc.Equals, "because reasons")
}

// TestLogActionMessage ensures LogActionMessage works properly.
func (s *InterfaceSuite) TestLogActionMessage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := s.getHookContext(c, ctrl, coretesting.ModelTag.Id(), -1, "", names.StorageTag{})
	s.unit.EXPECT().LogActionMessage(names.NewActionTag("2"), "hello world").Return(nil)
	context.WithActionContext(hctx, nil, nil)
	err := hctx.LogActionMessage("hello world")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *InterfaceSuite) TestRequestRebootAfterHook(c *gc.C) {
	var killed bool
	p := &mockProcess{func() error {
		killed = true
		return nil
	}}

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{}).(*context.HookContext)
	ctx.SetProcess(p)
	err := ctx.RequestReboot(jujuc.RebootAfterHook)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(killed, jc.IsFalse)
	priority := ctx.GetRebootPriority()
	c.Assert(priority, gc.Equals, jujuc.RebootAfterHook)
}

func (s *InterfaceSuite) TestRequestRebootNow(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{}).(*context.HookContext)

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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{}).(*context.HookContext)

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
	// A normal hook run or a juju-exec command will record the *os.Process
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

func (s *InterfaceSuite) TestSecretMetadata(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	uri, _ := coresecrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	uri2 := coresecrets.NewURI()
	s.secretMetadata = map[string]jujuc.SecretMetadata{
		uri.ID: {
			Label:        "label",
			Owner:        names.NewApplicationTag("mariadb"),
			Description:  "description",
			RotatePolicy: coresecrets.RotateHourly,
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Scope:  "relation-mariadb.db#gitlab.db",
					Role:   coresecrets.RoleView,
				},
			},
		},
		uri2.ID: {
			Owner:       names.NewApplicationTag("mariadb"),
			Description: "will be removed",
		},
	}
	ctx := s.GetContext(c, ctrl, -1, "", names.StorageTag{})
	md, err := ctx.SecretMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, jc.DeepEquals, map[string]jujuc.SecretMetadata{
		uri.ID: {
			Label:        "label",
			Owner:        names.NewApplicationTag("mariadb"),
			Description:  "description",
			RotatePolicy: coresecrets.RotateHourly,
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Scope:  "relation-mariadb.db#gitlab.db",
					Role:   coresecrets.RoleView,
				},
			},
		},
		uri2.ID: {
			Owner:       names.NewApplicationTag("mariadb"),
			Description: "will be removed",
		},
	})
	uri3, err := ctx.CreateSecret(&jujuc.SecretCreateArgs{
		OwnerTag: names.NewApplicationTag("foo"),
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Description: ptr("a new one"),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.UpdateSecret(uri, &jujuc.SecretUpdateArgs{
		Description: ptr("another"),
	})
	c.Assert(err, jc.ErrorIsNil)
	ctx.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    ptr("gitlab/1"),
		RelationKey: ptr("mariadb:db gitlab:db"),
		Role:        ptr(coresecrets.RoleView),
	})

	err = ctx.RemoveSecret(uri2, nil)
	c.Assert(err, jc.ErrorIsNil)
	md, err = ctx.SecretMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md, jc.DeepEquals, map[string]jujuc.SecretMetadata{
		uri.ID: {
			Label:        "label",
			Owner:        names.NewApplicationTag("mariadb"),
			Description:  "another",
			RotatePolicy: coresecrets.RotateHourly,
			Access: []coresecrets.AccessInfo{
				{Target: "unit-gitlab-0", Scope: "relation-mariadb.db#gitlab.db", Role: "view"},
			},
		},
		uri3.ID: {
			Owner:          names.NewApplicationTag("foo"),
			Description:    "a new one",
			LatestRevision: 1,
		},
	})
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

var _ = gc.Suite(&HookContextSuite{})

type HookContextSuite struct {
	testing.IsolationSuite
	mockUnit       *api.MockUnit
	mockLeadership *mocks.MockLeadershipContext
	mockCache      params.UnitStateResult
}

func (s *HookContextSuite) TestDeleteCharmStateValue(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.DeleteCharmStateValue("one")
	c.Assert(err, jc.ErrorIsNil)

	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCache, gc.DeepEquals, s.mockCache.CharmState)
}

func (s *HookContextSuite) TestDeleteCacheStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.DeleteCharmStateValue("five")
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *HookContextSuite) TestGetCharmState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedCache, gc.DeepEquals, s.mockCache.CharmState)
}

func (s *HookContextSuite) TestGetCharmStateStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	_, err := hookContext.GetCharmState()
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *HookContextSuite) TestGetCharmStateValue(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedVale, err := hookContext.GetCharmStateValue("one")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVale, gc.Equals, "two")
}

func (s *HookContextSuite) TestGetCharmStateValueEmpty(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedVale, err := hookContext.GetCharmStateValue("seven")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedVale, gc.Equals, "")
}

func (s *HookContextSuite) TestGetCharmStateValueNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	obtainedCache, err := hookContext.GetCharmStateValue("five")
	c.Assert(err, gc.ErrorMatches, "\"five\" not found")
	c.Assert(obtainedCache, gc.Equals, "")
}

func (s *HookContextSuite) TestGetCharmStateValueStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	_, err := hookContext.GetCharmStateValue("key")
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *HookContextSuite) TestSetCacheQuotaLimits(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	s.testSetCache(c)
}

func (s *HookContextSuite) TestSetCache(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectStateValues()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	// Test key len limit
	err := hookContext.SetCharmStateValue(
		strings.Repeat("a", quota.MaxCharmStateKeySize+1),
		"lol",
	)
	c.Assert(err, jc.ErrorIs, errors.QuotaLimitExceeded)
	c.Assert(err, gc.ErrorMatches, ".*max allowed key.*")

	// Test value len limit
	err = hookContext.SetCharmStateValue(
		"lol",
		strings.Repeat("a", quota.MaxCharmStateValueSize+1),
	)
	c.Assert(err, jc.ErrorIs, errors.QuotaLimitExceeded)
	c.Assert(err, gc.ErrorMatches, ".*max allowed value.*")
}

func (s *HookContextSuite) TestSetCacheEmptyStartState(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, nil)

	s.testSetCache(c)
}

func (s *HookContextSuite) testSetCache(c *gc.C) {
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.SetCharmStateValue("five", "six")
	c.Assert(err, jc.ErrorIsNil)
	obtainedCache, err := hookContext.GetCharmState()
	c.Assert(err, jc.ErrorIsNil)
	value, ok := obtainedCache["five"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(value, gc.Equals, "six")
}

func (s *HookContextSuite) TestSetCacheStateErr(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockUnit.EXPECT().State().Return(params.UnitStateResult{}, errors.Errorf("testing an error"))

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	err := hookContext.SetCharmStateValue("five", "six")
	c.Assert(err, gc.ErrorMatches, "loading unit state from database: testing an error")
}

func (s *HookContextSuite) TestFlushWithNonDirtyCache(c *gc.C) {
	defer s.setupMocks(c).Finish()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	s.expectStateValues()

	// The following commands are no-ops as they don't mutate the cache.
	err := hookContext.SetCharmStateValue("one", "two") // no-op: KV already present
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.DeleteCharmStateValue("not-there") // no-op: key not present
	c.Assert(err, jc.ErrorIsNil)

	// Flush the context with a success. As the cache is not dirty we do
	// not expect a SetState call.
	err = hookContext.Flush(stdcontext.Background(), "success", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) TestSequentialFlushOfCacheValues(c *gc.C) {
	defer s.setupMocks(c).Finish()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	// We expect a single call for the following API endpoints
	s.expectStateValues()
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
	err = hookContext.Flush(stdcontext.Background(), "success", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Flush again; as the cache is not dirty, the SetState call is skipped.
	err = hookContext.Flush(stdcontext.Background(), "success", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) TestOpenPortRange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.CAAS, s.mockLeadership)

	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				OpenPorts: []params.EntityPortRange{
					{
						Tag:      "unit-wordpress-0",
						Endpoint: "",
						Protocol: "tcp",
						FromPort: 8080,
						ToPort:   8080,
					},
				},
			},
		},
	}).Return(nil)

	err := hookContext.OpenPortRange("", network.MustParsePortRange("8080/tcp"))
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.Flush(stdcontext.Background(), "success", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) TestOpenedPortRanges(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				OpenPorts: []params.EntityPortRange{
					{
						Tag:      "unit-wordpress-0",
						Endpoint: "",
						Protocol: "tcp",
						FromPort: 8080,
						ToPort:   8080,
					},
				},
			},
		},
	}).Return(nil)

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.CAAS, s.mockLeadership)

	err := hookContext.OpenPortRange("", network.MustParsePortRange("8080/tcp"))
	c.Assert(err, jc.ErrorIsNil)

	// OpenedPortRanges() should return the pending requests, see
	// https://bugs.launchpad.net/juju/+bug/2008035
	openedPorts := hookContext.OpenedPortRanges()
	expectedOpenPorts := []network.PortRange{
		// Already present range from NewMockUnitHookContext()
		{
			FromPort: 666,
			ToPort:   888,
			Protocol: "tcp",
		},
		// Newly added but not yet flushed range
		{
			FromPort: 8080,
			ToPort:   8080,
			Protocol: "tcp",
		},
	}
	c.Assert(openedPorts.UniquePortRanges(), gc.DeepEquals, expectedOpenPorts)

	err = hookContext.Flush(stdcontext.Background(), "success", nil)
	c.Assert(err, jc.ErrorIsNil)

	// After Flush() opened ports should remain the same.
	openedPorts = hookContext.OpenedPortRanges()
	c.Assert(openedPorts.UniquePortRanges(), gc.DeepEquals, expectedOpenPorts)
}

func (s *HookContextSuite) TestClosePortRange(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.CAAS, s.mockLeadership)

	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{
			{
				Tag: "unit-wordpress-0",
				ClosePorts: []params.EntityPortRange{
					{
						Tag:      "unit-wordpress-0",
						Endpoint: "",
						Protocol: "tcp",
						FromPort: 8080,
						ToPort:   8080,
					},
				},
			},
		},
	}).Return(nil)

	err := hookContext.ClosePortRange("", network.MustParsePortRange("8080/tcp"))
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.Flush(stdcontext.Background(), "success", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockUnit = api.NewMockUnit(ctrl)
	s.mockUnit.EXPECT().Tag().Return(names.NewUnitTag("wordpress/0")).AnyTimes()
	s.mockUnit.EXPECT().ApplicationName().Return("wordpress").AnyTimes()
	s.mockLeadership = mocks.NewMockLeadershipContext(ctrl)
	return ctrl
}

func (s *HookContextSuite) expectStateValues() {
	s.mockCache = params.UnitStateResult{
		CharmState: map[string]string{
			"one":   "two",
			"three": "four",
			"seven": "",
		}}
	s.mockUnit.EXPECT().State().Return(s.mockCache, nil)
}

func (s *HookContextSuite) TestActionAbort(c *gc.C) {
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
		ctrl := s.setupMocks(c)
		client := api.NewMockUniterClient(ctrl)
		hookContext := context.NewMockUnitHookContextWithUniter(s.mockUnit, client)
		client.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("2"), test.Status, map[string]any(nil), "failed yo").Return(nil)

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
		err = hookContext.Flush(stdcontext.Background(), "", errors.Errorf("failed yo"))
		c.Assert(err, jc.ErrorIsNil)
		ctrl.Finish()
	}
}

func (s *HookContextSuite) TestActionFlushError(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockUnit.EXPECT().CommitHookChanges(params.CommitHookChangesArgs{
		Args: []params.CommitHookChangesArg{{
			Tag: "unit-wordpress-0",
			OpenPorts: []params.EntityPortRange{{
				Tag:      "unit-wordpress-0",
				Protocol: "tcp",
				FromPort: 666,
				ToPort:   666,
				Endpoint: "ep",
			}},
		}},
	}).Return(errors.New("flush failed"))

	client := api.NewMockUniterClient(ctrl)
	hookContext := context.NewMockUnitHookContextWithUniter(s.mockUnit, client)
	resultData := map[string]interface{}{
		"stderr":      "flush failed",
		"return-code": "1",
	}
	client.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("2"), "failed", resultData, "committing requested changes failed").Return(nil)
	context.SetEnvironmentHookContextSecret(hookContext, coresecrets.NewURI().String(), nil, nil, nil)

	err := hookContext.OpenPortRange("ep", network.PortRange{Protocol: "tcp", FromPort: 666, ToPort: 666})
	c.Assert(err, jc.ErrorIsNil)
	cancel := make(chan struct{})
	context.WithActionContext(hookContext, nil, cancel)
	err = hookContext.Flush(stdcontext.Background(), "", nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) TestMissingAction(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	client := api.NewMockUniterClient(ctrl)
	hookContext := context.NewMockUnitHookContextWithUniter(s.mockUnit, client)
	client.EXPECT().ActionFinish(gomock.Any(), names.NewActionTag("2"), "failed", map[string]any(nil),
		`action not implemented on unit "wordpress/0"`).Return(nil)

	context.WithActionContext(hookContext, nil, nil)
	err := hookContext.Flush(stdcontext.Background(), "action", charmrunner.NewMissingHookError("noaction"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *HookContextSuite) assertSecretGetFromPendingChanges(c *gc.C,
	refresh, peek bool,
	setPendingSecretChanges func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string),
) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	uri := coresecrets.NewURI()
	label := "label"
	data := map[string]string{"foo": "bar"}
	if !refresh && !peek {
		data["foo"] = "existing"
	}
	setPendingSecretChanges(hookContext, uri, label, data)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, mockBackendClient{})

	value, err := hookContext.GetSecret(nil, label, refresh, peek)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, data)
}

func (s *HookContextSuite) TestSecretGetFromPendingCreateChanges(c *gc.C) {
	s.assertSecretGetFromPendingChanges(c, false, false,
		func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string) {
			arg := apiuniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(value)
			hc.SetPendingSecretCreates(
				map[string]uniter.SecretCreateArg{uri.ID: arg})
		},
	)
}

func (s *HookContextSuite) TestAppSecretGetFromPendingCreateChanges(c *gc.C) {
	s.assertSecretGetFromPendingChanges(c, false, true,
		func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string) {
			arg := uniter.SecretCreateArg{OwnerTag: names.NewApplicationTag(s.mockUnit.ApplicationName())}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(value)
			hc.SetPendingSecretCreates(
				map[string]uniter.SecretCreateArg{uri.ID: arg})
		},
	)
}

func (s *HookContextSuite) TestSecretGetFromPendingUpdateChanges(c *gc.C) {
	s.assertSecretGetFromPendingChanges(c, false, true,
		func(hc *context.HookContext, uri *coresecrets.URI, label string, value map[string]string) {
			arg := apiuniter.SecretUpdateArg{}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(value)
			hc.SetPendingSecretUpdates(
				map[string]uniter.SecretUpdateArg{uri.ID: arg})
		},
	)
}

type mockBackend struct {
	provider.SecretsBackend
}

func (mockBackend) GetContent(_ stdcontext.Context, revisionId string) (coresecrets.SecretValue, error) {
	if revisionId != "rev-id" {
		return nil, errors.NotFoundf("revision %q", revisionId)
	}
	return coresecrets.NewSecretValue(map[string]string{"foo": "bar"}), nil
}

type mockBackendClient struct {
	secrets.BackendsClient
}

func (mockBackendClient) GetContent(uri *coresecrets.URI, label string, refresh, peek bool) (coresecrets.SecretValue, error) {
	return coresecrets.NewSecretValue(map[string]string{"foo": "existing"}), nil
}

func (s *HookContextSuite) TestSecretGet(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.PatchValue(&secrets.GetBackend, func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		c.Assert(cfg.BackendConfig.BackendType, gc.Equals, "vault")
		return mockBackend{}, nil
	})

	uri := coresecrets.NewURI()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "SecretsManager")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetSecretContentInfo")
		c.Assert(arg, jc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Label:   "label",
				Refresh: true,
				Peek:    true,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			Results: []params.SecretContentResult{{
				Content: params.SecretContentParams{
					ValueRef: &params.SecretValueRef{
						BackendID:  "backend-id",
						RevisionID: "rev-id",
					},
				},
				BackendConfig: &params.SecretBackendConfigResult{
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      coretesting.ModelTag.Id(),
					ModelName:      "fred",
					Config: params.SecretBackendConfig{
						BackendType: vault.BackendType,
					},
				},
			}},
		}
		return nil
	})

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	secretsBackend, err := secrets.NewClient(jujuSecretsAPI)
	c.Assert(err, jc.ErrorIsNil)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, jujuSecretsAPI, secretsBackend)

	value, err := hookContext.GetSecret(uri, "label", true, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *HookContextSuite) assertSecretGetOwnedSecretURILookup(
	c *gc.C, patchContext func(*context.HookContext, *coresecrets.URI, string, api.SecretsAccessor, secrets.BackendsClient),
) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "SecretsManager")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "GetSecretContentInfo")
		c.Assert(arg, gc.DeepEquals, params.GetSecretContentArgs{
			Args: []params.GetSecretContentArg{{
				URI:     uri.String(),
				Refresh: false,
				Peek:    false,
			}},
		})
		c.Assert(result, gc.FitsTypeOf, &params.SecretContentResults{})
		*(result.(*params.SecretContentResults)) = params.SecretContentResults{
			[]params.SecretContentResult{{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			}},
		}
		return nil
	})

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	secretsBackend, err := secrets.NewClient(jujuSecretsAPI)
	c.Assert(err, jc.ErrorIsNil)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, jujuSecretsAPI, secretsBackend)

	patchContext(hookContext, uri, "label", jujuSecretsAPI, secretsBackend)

	value, err := hookContext.GetSecret(nil, "label", false, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *HookContextSuite) TestSecretGetOwnedSecretURILookupFromAppliedCache(c *gc.C) {
	s.assertSecretGetOwnedSecretURILookup(c,
		func(ctx *context.HookContext, uri *coresecrets.URI, label string, client api.SecretsAccessor, backend secrets.BackendsClient) {
			context.SetEnvironmentHookContextSecret(
				ctx, uri.String(),
				map[string]jujuc.SecretMetadata{
					uri.ID: {Label: "label", Owner: s.mockUnit.Tag()},
				},
				client, backend)
		},
	)
}

func (s *HookContextSuite) TestSecretGetOwnedSecretURILookupFromPendingCreate(c *gc.C) {
	s.assertSecretGetOwnedSecretURILookup(c,
		func(ctx *context.HookContext, uri *coresecrets.URI, label string, client api.SecretsAccessor, backend secrets.BackendsClient) {
			arg := apiuniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
			ctx.SetPendingSecretCreates(
				map[string]uniter.SecretCreateArg{uri.ID: arg})
		},
	)
}

func (s *HookContextSuite) TestSecretGetOwnedSecretLabelLookupFromPendingCreates(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	hookContext.SetPendingSecretCreates(
		map[string]uniter.SecretCreateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(nil, label, false, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *HookContextSuite) TestSecretGetOwnedSecretUpdatePendingCreateLabel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretCreateArg{OwnerTag: s.mockUnit.Tag()}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	hookContext.SetPendingSecretCreates(
		map[string]uniter.SecretCreateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(uri, "foobar", false, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	arg.Label = ptr("foobar")
	c.Assert(hookContext.PendingSecretCreates(), jc.DeepEquals, map[string]uniter.SecretCreateArg{
		uri.ID: arg,
	})
}

func (s *HookContextSuite) TestSecretGetOwnedSecretURILookupFromPendingUpdate(c *gc.C) {
	s.assertSecretGetOwnedSecretURILookup(c,
		func(ctx *context.HookContext, uri *coresecrets.URI, label string, client api.SecretsAccessor, backend secrets.BackendsClient) {
			arg := apiuniter.SecretUpdateArg{}
			arg.URI = uri
			arg.Label = ptr(label)
			arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
			ctx.SetPendingSecretUpdates(
				map[string]uniter.SecretUpdateArg{uri.ID: arg})
		},
	)
}

func (s *HookContextSuite) TestSecretGetOwnedSecretLabelLookupFromPendingUpdatesPeek(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretUpdateArg{}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	hookContext.SetPendingSecretUpdates(
		map[string]uniter.SecretUpdateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(nil, label, false, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	c.Assert(hookContext.PendingSecretTrackLatest(), gc.HasLen, 0)
}

func (s *HookContextSuite) TestSecretGetOwnedSecretLabelLookupFromPendingUpdatesRefresh(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretUpdateArg{}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	hookContext.SetPendingSecretUpdates(
		map[string]uniter.SecretUpdateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(nil, label, true, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	c.Assert(hookContext.PendingSecretTrackLatest(), jc.DeepEquals, map[string]bool{uri.ID: true})
}

func (s *HookContextSuite) TestSecretGetOwnedSecretUpdatePendingLabel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	uri := coresecrets.NewURI()
	label := "label-" + uri.String()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), nil, nil, nil)

	arg := uniter.SecretUpdateArg{}
	arg.URI = uri
	arg.Label = ptr(label)
	arg.Value = coresecrets.NewSecretValue(map[string]string{"foo": "bar"})
	hookContext.SetPendingSecretUpdates(
		map[string]uniter.SecretUpdateArg{uri.ID: arg})

	value, err := hookContext.GetSecret(uri, "foobar", false, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(value.EncodedValues(), jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
	arg.Label = ptr("foobar")
	c.Assert(hookContext.PendingSecretUpdates(), jc.DeepEquals, map[string]uniter.SecretUpdateArg{
		uri.ID: arg,
	})
}

func ptr[T any](v T) *T {
	return &v
}

func (s *HookContextSuite) TestSecretCreateApplicationOwner(c *gc.C) {
	s.assertSecretCreate(c, names.NewApplicationTag("mariadb"))
}

func (s *HookContextSuite) TestSecretCreateUnitOwner(c *gc.C) {
	s.assertSecretCreate(c, names.NewUnitTag("mariadb/0"))
}

func (s *HookContextSuite) assertSecretCreate(c *gc.C, owner names.Tag) {
	defer s.setupMocks(c).Finish()

	data := map[string]string{"foo": "bar"}
	value := coresecrets.NewSecretValue(data)
	expiry := time.Now()
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "SecretsManager")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "CreateSecretURIs")
		c.Check(arg, gc.DeepEquals, params.CreateSecretURIsArg{
			Count: 1,
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "secret:9m4e2mr0ui3e8a215n4g",
			}},
		}
		return nil
	})
	if owner.Kind() == names.ApplicationTagKind {
		s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	}

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	context.SetEnvironmentHookContextSecret(hookContext, "", nil, jujuSecretsAPI, nil)

	uri, err := hookContext.CreateSecret(&jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value:        value,
			RotatePolicy: ptr(coresecrets.RotateDaily),
			ExpireTime:   ptr(expiry),
			Description:  ptr("my secret"),
			Label:        ptr("foo"),
		},
		OwnerTag: owner,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uri.String(), gc.Equals, "secret:9m4e2mr0ui3e8a215n4g")
	c.Assert(hookContext.PendingSecretCreates(), jc.DeepEquals, map[string]uniter.SecretCreateArg{
		uri.ID: {
			SecretUpsertArg: uniter.SecretUpsertArg{
				URI:          uri,
				Value:        value,
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(expiry),
				Description:  ptr("my secret"),
				Label:        ptr("foo"),
			},
			OwnerTag: owner,
		}})
}

func (s *HookContextSuite) TestSecretCreateDupLabel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	data := map[string]string{"foo": "bar"}
	value := coresecrets.NewSecretValue(data)
	apiCaller := basetesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		c.Assert(objType, gc.Equals, "SecretsManager")
		c.Assert(version, gc.Equals, 0)
		c.Assert(id, gc.Equals, "")
		c.Assert(request, gc.Equals, "CreateSecretURIs")
		c.Check(arg, gc.DeepEquals, params.CreateSecretURIsArg{
			Count: 1,
		})
		c.Assert(result, gc.FitsTypeOf, &params.StringResults{})
		*(result.(*params.StringResults)) = params.StringResults{
			[]params.StringResult{{
				Result: "secret:9m4e2mr0ui3e8a215n4g",
			}},
		}
		return nil
	})
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil).Times(2)

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	jujuSecretsAPI := secretsmanager.NewClient(apiCaller)
	context.SetEnvironmentHookContextSecret(hookContext, "", nil, jujuSecretsAPI, nil)

	_, err := hookContext.CreateSecret(&jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value: value,
			Label: ptr("foo"),
		},
		OwnerTag: names.NewApplicationTag("myapp"),
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = hookContext.CreateSecret(&jujuc.SecretCreateArgs{
		SecretUpdateArgs: jujuc.SecretUpdateArgs{
			Value: value,
			Label: ptr("foo"),
		},
		OwnerTag: names.NewApplicationTag("myapp"),
	})
	c.Assert(err, gc.ErrorMatches, `secret with label "foo" already exists`)
}

func (s *HookContextSuite) TestSecretUpdate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	data := map[string]string{"foo": "bar"}
	value := coresecrets.NewSecretValue(data)
	expiry := time.Now()
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
	}, nil, nil)
	err := hookContext.UpdateSecret(uri, &jujuc.SecretUpdateArgs{
		Value:        value,
		RotatePolicy: ptr(coresecrets.RotateDaily),
		ExpireTime:   ptr(expiry),
		Description:  ptr("my secret"),
		Label:        ptr("foo"),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretUpdates(), jc.DeepEquals, map[string]uniter.SecretUpdateArg{
		uri.ID: {
			CurrentRevision: 666,
			SecretUpsertArg: uniter.SecretUpsertArg{
				URI:          uri,
				Value:        value,
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(expiry),
				Description:  ptr("my secret"),
				Label:        ptr("foo"),
			},
		}})
}

func (s *HookContextSuite) TestSecretRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID:  {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
		uri2.ID: {Description: "another secret", LatestRevision: 667, Owner: names.NewUnitTag("mariadb/666")},
	}, nil, nil)
	err := hookContext.RemoveSecret(uri, nil)
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.RemoveSecret(uri2, ptr(666))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRemoves(), jc.DeepEquals, map[string]uniter.SecretDeleteArg{
		uri.ID:  {URI: uri},
		uri2.ID: {URI: uri2, Revision: ptr(666)}})
}

func (s *HookContextSuite) TestSecretGrant(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID:  {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
		uri2.ID: {Description: "another secret", LatestRevision: 667, Owner: names.NewUnitTag("mariadb/666")},
	}, nil, nil)

	app := "mariadb"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.GrantSecret(uri2, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{
		uri.ID: {
			relationKey: {
				URI:             uri,
				ApplicationName: &app,
				RelationKey:     &relationKey,
				Role:            coresecrets.RoleView,
			},
		},
		uri2.ID: {
			relationKey: {
				URI:             uri2,
				ApplicationName: &app,
				RelationKey:     &relationKey,
				Role:            coresecrets.RoleView,
			},
		}})
}

func (s *HookContextSuite) TestSecretGrantSecretNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)

	uri := coresecrets.NewURI()
	app := "mariadb"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(errors.Is(err, errors.NotFound), jc.IsTrue)
}

func (s *HookContextSuite) TestSecretGrantNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(false, nil)

	app := "mariadb"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(errors.Is(err, context.ErrIsNotLeader), jc.IsTrue)
}

func (s *HookContextSuite) TestSecretGrantNoOPSBecauseofExactSameApp(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "application-gitlab",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	app := "gitlab"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
		Role:            ptr(coresecrets.RoleView),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
}

func (s *HookContextSuite) TestSecretGrantNoOPSBecauseofExactSameUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	unit := "gitlab/0"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    &unit,
		RelationKey: &relationKey,
		Role:        ptr(coresecrets.RoleView),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
}

func (s *HookContextSuite) TestSecretGrantNoOPSBecauseApplicationLevelGrantedAlready(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "application-gitlab",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	unit := "gitlab/0"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    &unit,
		RelationKey: &relationKey,
		Role:        ptr(coresecrets.RoleView),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
}

func (s *HookContextSuite) TestSecretGrantFailedRevokeExistingRecordRequired(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID: {
			Description:    "a secret",
			LatestRevision: 666,
			Owner:          names.NewApplicationTag("mariadb"),
			Access: []coresecrets.AccessInfo{
				{
					Target: "unit-gitlab-0",
					Role:   coresecrets.RoleView,
					Scope:  "relation-mariadb.db#gitlab.db",
				},
			},
		},
	}, nil, nil)
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil)
	c.Assert(hookContext.PendingSecretGrants(), jc.DeepEquals, map[string]map[string]uniter.SecretGrantRevokeArgs{})
	app := "gitlab"
	relationKey := "mariadb:db gitlab:db"
	err := hookContext.GrantSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
		Role:            ptr(coresecrets.RoleView),
	})
	c.Assert(err, gc.ErrorMatches, `any unit level grants need to be revoked before granting access to the corresponding application`)
}

func (s *HookContextSuite) TestSecretRevoke(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.mockLeadership.EXPECT().IsLeader().Return(true, nil).AnyTimes()
	hookContext := context.NewMockUnitHookContext(s.mockUnit, model.IAAS, s.mockLeadership)
	context.SetEnvironmentHookContextSecret(hookContext, uri.String(), map[string]jujuc.SecretMetadata{
		uri.ID:  {Description: "a secret", LatestRevision: 666, Owner: names.NewApplicationTag("mariadb")},
		uri2.ID: {Description: "another secret", LatestRevision: 667, Owner: names.NewUnitTag("mariadb/666")},
	}, nil, nil)
	app := "mariadb"
	unit0 := "mariadb/0"
	relationKey := "wordpress:db mysql:server"
	err := hookContext.RevokeSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = hookContext.RevokeSecret(uri2, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRevokes(), jc.DeepEquals,
		map[string][]uniter.SecretGrantRevokeArgs{
			uri.ID: {
				{
					URI:             uri,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
			uri2.ID: {
				{
					URI:             uri2,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
		},
	)

	// No OPS for duplicated revoke.
	err = hookContext.RevokeSecret(uri, &jujuc.SecretGrantRevokeArgs{
		ApplicationName: &app,
		RelationKey:     &relationKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRevokes(), jc.DeepEquals,
		map[string][]uniter.SecretGrantRevokeArgs{
			uri.ID: {
				{
					URI:             uri,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
			uri2.ID: {
				{
					URI:             uri2,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
		},
	)

	// No OPS for unit level revoke because application level revoke exists already.
	err = hookContext.RevokeSecret(uri, &jujuc.SecretGrantRevokeArgs{
		UnitName:    &unit0,
		RelationKey: &relationKey,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hookContext.PendingSecretRevokes(), jc.DeepEquals,
		map[string][]uniter.SecretGrantRevokeArgs{
			uri.ID: {
				{
					URI:             uri,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
			uri2.ID: {
				{
					URI:             uri2,
					ApplicationName: &app,
					RelationKey:     &relationKey,
				},
			},
		},
	)

}

func (s *HookContextSuite) TestHookStorage(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := api.NewMockUniterClient(ctrl)
	st.EXPECT().StorageAttachment(names.NewStorageTag("data/0"), names.NewUnitTag("wordpress/0")).Return(params.StorageAttachment{
		StorageTag: "data/0",
	}, nil)
	s.mockUnit.EXPECT().Tag().Return(names.NewUnitTag("wordpress/0")).AnyTimes()
	ctx := context.NewMockUnitHookContextWithStateAndStorage("wordpress/0", s.mockUnit, st, names.NewStorageTag("data/0"))

	storage, err := ctx.HookStorage()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storage, gc.NotNil)
	c.Assert(storage.Tag().Id(), gc.Equals, "data/0")
}
