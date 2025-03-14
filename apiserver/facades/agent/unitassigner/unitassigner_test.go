// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type testsuite struct {
	testing.IsolationSuite

	statusService *MockStatusService
}

var _ = gc.Suite(&testsuite{})

func (s *testsuite) TestAssignUnits(c *gc.C) {
	f := &fakeState{
		unitMachines: map[string]string{"foo/0": "1/lxd/2"},
	}
	f.results = []state.UnitAssignmentResult{{Unit: "foo/0"}}
	machineService := &fakeMachineService{}
	stubService := &fakeStubService{assignments: map[string][]unit.Name{}}
	api := API{
		st:             f,
		res:            common.NewResources(),
		machineService: machineService,
		networkService: &fakeNetworkService{},
		stubService:    stubService,
	}
	args := params.Entities{Entities: []params.Entity{{Tag: "unit-foo-0"}, {Tag: "unit-bar-1"}}}
	res, err := api.AssignUnits(context.Background(), args)
	c.Assert(f.ids, gc.DeepEquals, []string{"foo/0", "bar/1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Results, gc.HasLen, 2)
	c.Assert(res.Results[0].Error, gc.IsNil)
	c.Assert(res.Results[1].Error, gc.ErrorMatches, `unit "unit-bar-1" not found`)
	c.Assert(machineService.machineNames, jc.SameContents, []machine.Name{machine.Name("1"), machine.Name("1/lxd/2")})
	c.Assert(stubService.assignments, jc.DeepEquals, map[string][]unit.Name{"1/lxd/2": {"foo/0"}})
}

func (s *testsuite) TestWatchUnitAssignment(c *gc.C) {
	f := &fakeState{}
	api := API{st: f, res: common.NewResources()}
	f.ids = []string{"boo", "far"}
	res, err := api.WatchUnitAssignments(context.Background())
	c.Assert(f.watchCalled, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Changes, gc.DeepEquals, f.ids)
}

func (s *testsuite) TestSetStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	status := status.StatusInfo{
		Status:  status.Idle,
		Message: "message",
		Data: map[string]interface{}{
			"foo": "bar",
		},
	}

	s.statusService.EXPECT().SetUnitAgentStatus(gomock.Any(), unit.Name("foo/0"), &status).Return(nil)

	api := s.newAPI(c)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "unit-foo-0",
			Status: status.Status.String(),
			Info:   status.Message,
			Data:   status.Data,
		}, {
			Tag: "foo",
		}},
	}
	res, err := api.SetAgentStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(res.Results, gc.DeepEquals, []params.ErrorResult{
		{},
		{Error: apiservererrors.ServerError(errors.Errorf(`"foo" is not a valid tag`))},
	})
}

func (s *testsuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.statusService = NewMockStatusService(ctrl)

	return ctrl
}

func (s *testsuite) newAPI(c *gc.C) *API {
	return &API{
		statusService: s.statusService,
	}
}

type fakeMachineService struct {
	machineNames []machine.Name
}

func (f *fakeMachineService) CreateMachine(_ context.Context, machineName machine.Name) (string, error) {
	f.machineNames = append(f.machineNames, machineName)
	return "", nil
}

type fakeNetworkService struct {
}

func (f *fakeNetworkService) GetAllSpaces(_ context.Context) (network.SpaceInfos, error) {
	return nil, nil
}

type fakeStubService struct {
	assignments map[string][]unit.Name
}

func (f *fakeStubService) AssignUnitsToMachines(_ context.Context, machineToUnitMap map[string][]unit.Name) error {
	for machine, units := range machineToUnitMap {
		f.assignments[machine] = append(f.assignments[machine], units...)
	}
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

func (f *fakeState) AssignStagedUnits(_ network.SpaceInfos, ids []string) ([]state.UnitAssignmentResult, error) {
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
