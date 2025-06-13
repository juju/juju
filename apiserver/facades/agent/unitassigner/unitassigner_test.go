// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type testsuite struct {
	testhelpers.IsolationSuite

	statusService *MockStatusService
}

func TestTestsuite(t *testing.T) {
	tc.Run(t, &testsuite{})
}

func (s *testsuite) TestAssignUnits(c *tc.C) {
	f := &fakeState{
		unitMachines: map[string]string{"foo/0": "1/lxd/2"},
	}
	f.results = []state.UnitAssignmentResult{{Unit: "foo/0"}}
	machineService := &fakeMachineService{}
	api := API{
		st:             f,
		res:            common.NewResources(),
		machineService: machineService,
		networkService: &fakeNetworkService{},
	}
	args := params.Entities{Entities: []params.Entity{{Tag: "unit-foo-0"}, {Tag: "unit-bar-1"}}}
	res, err := api.AssignUnits(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(f.ids, tc.DeepEquals, []string{"foo/0", "bar/1"})
	c.Assert(res.Results, tc.HasLen, 2)
	c.Check(res.Results[0].Error, tc.IsNil)
	c.Check(res.Results[1].Error, tc.ErrorMatches, `unit "unit-bar-1" not found`)
	c.Check(machineService.machineNames, tc.SameContents, []machine.Name{machine.Name("1"), machine.Name("1/lxd/2")})
}

func (s *testsuite) TestWatchUnitAssignment(c *tc.C) {
	f := &fakeState{}
	api := API{st: f, res: common.NewResources()}
	f.ids = []string{"boo", "far"}
	res, err := api.WatchUnitAssignments(c.Context())
	c.Assert(f.watchCalled, tc.IsTrue)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Changes, tc.DeepEquals, f.ids)
}

func (s *testsuite) TestSetStatus(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().SetUnitAgentStatus(gomock.Any(), unit.Name("foo/0"), gomock.Any()).Return(nil)

	status := status.StatusInfo{
		Status:  status.Idle,
		Message: "message",
		Data: map[string]interface{}{
			"foo": "bar",
		},
	}

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
	res, err := api.SetAgentStatus(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(res.Results, tc.DeepEquals, []params.ErrorResult{
		{},
		{Error: apiservererrors.ServerError(errors.Errorf(`"foo" is not a valid tag`))},
	})
}

func (s *testsuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.statusService = NewMockStatusService(ctrl)

	return ctrl
}

func (s *testsuite) newAPI(c *tc.C) *API {
	return &API{
		statusService: s.statusService,
		clock:         clock.WallClock,
	}
}

type fakeMachineService struct {
	machineNames []machine.Name
}

func (f *fakeMachineService) CreateMachine(_ context.Context, machineName machine.Name, nonce *string) (machine.UUID, error) {
	f.machineNames = append(f.machineNames, machineName)
	return "", nil
}

type fakeNetworkService struct {
}

func (f *fakeNetworkService) GetAllSpaces(_ context.Context) (network.SpaceInfos, error) {
	return nil, nil
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
