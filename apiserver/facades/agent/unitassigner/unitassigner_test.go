// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(testsuite{})

type testsuite struct{}

func (testsuite) TestAssignUnits(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	spaceService := NewMockSpaceService(ctrl)
	spaceService.EXPECT().GetAllSpaces(gomock.Any())

	f := &fakeState{
		unitMachines: map[string]string{"foo/0": "1/lxd/2"},
	}
	f.results = []state.UnitAssignmentResult{{Unit: "foo/0"}}
	a := &fakeMachineService{}
	api := API{st: f, res: common.NewResources(), machineService: a, spaceService: spaceService}
	args := params.Entities{Entities: []params.Entity{{Tag: "unit-foo-0"}, {Tag: "unit-bar-1"}}}
	res, err := api.AssignUnits(context.Background(), args)
	c.Assert(f.ids, gc.DeepEquals, []string{"foo/0", "bar/1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Results[0].Error, gc.IsNil)
	c.Assert(res.Results[1].Error, gc.ErrorMatches, `unit "unit-bar-1" not found`)
	c.Assert(a.machineIds, jc.SameContents, []string{"1", "1/lxd/2"})
}

func (testsuite) TestWatchUnitAssignment(c *gc.C) {
	f := &fakeState{}
	api := API{st: f, res: common.NewResources()}
	f.ids = []string{"boo", "far"}
	res, err := api.WatchUnitAssignments(context.Background())
	c.Assert(f.watchCalled, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Changes, gc.DeepEquals, f.ids)
}

func (testsuite) TestSetStatus(c *gc.C) {
	f := &fakeStatusSetter{
		res: params.ErrorResults{
			Results: []params.ErrorResult{
				{Error: &params.Error{Message: "boo"}}}}}
	api := API{statusSetter: f}
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{Tag: "foo/0"}},
	}
	res, err := api.SetAgentStatus(context.Background(), args)
	c.Assert(args, jc.DeepEquals, f.args)
	c.Assert(res, jc.DeepEquals, f.res)
	c.Assert(err, gc.Equals, f.err)
}

type fakeMachineService struct {
	machineIds []string
}

func (f *fakeMachineService) CreateMachine(_ context.Context, machineId string) error {
	f.machineIds = append(f.machineIds, machineId)
	return nil
}

type fakeState struct {
	watchCalled  bool
	ids          []string
	unitMachines map[string]string
	results      []state.UnitAssignmentResult
	err          error
}

func (f *fakeState) WatchForUnitAssignment() state.StringsWatcher {
	f.watchCalled = true
	return fakeWatcher{f.ids}
}

func (f *fakeState) AssignStagedUnits(ids []string, allSpaces network.SpaceInfos) ([]state.UnitAssignmentResult, error) {
	f.ids = ids
	return f.results, f.err
}

func (f *fakeState) AssignedMachineId(unit string) (string, error) {
	if len(f.unitMachines) == 0 {
		return "", nil
	}
	return f.unitMachines[unit], nil
}

type fakeWatcher struct {
	changes []string
}

func (f fakeWatcher) Changes() <-chan []string {
	changes := make(chan []string, 1)
	changes <- f.changes
	return changes
}
func (fakeWatcher) Kill() {}

func (fakeWatcher) Wait() error { return nil }

func (fakeWatcher) Stop() error { return nil }

func (fakeWatcher) Err() error { return nil }

type fakeStatusSetter struct {
	args params.SetStatus
	res  params.ErrorResults
	err  error
}

func (f *fakeStatusSetter) SetStatus(_ context.Context, args params.SetStatus) (params.ErrorResults, error) {
	f.args = args
	return f.res, f.err
}
