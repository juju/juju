// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
)

func TestUnitAssigner(t *testing.T) {
	tc.Run(t, &unitAssignerSuite{})
}

type unitAssignerSuite struct {
	watcherRegistry *facademocks.MockWatcherRegistry
}

func (s *unitAssignerSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *unitAssignerSuite) TestWatchUnitAssignments(c *tc.C) {
	defer s.setUpMocks(c).Finish()

	api := &API{
		watcherRegistry: s.watcherRegistry,
	}

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	result, err := api.WatchUnitAssignments(context.Background())
	c.Assert(err, tc.IsNil)
	c.Check(result.StringsWatcherId, tc.Equals, "1")
	c.Check(result.Changes, tc.DeepEquals, []string{""})
}

func (s *unitAssignerSuite) TestWatchUnitAssignmentsMultipleCalls(c *tc.C) {
	defer s.setUpMocks(c).Finish()

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

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	result1, err := api.WatchUnitAssignments(context.Background())
	c.Assert(err, tc.IsNil)
	c.Check(result1.StringsWatcherId, tc.Equals, "1")
	c.Check(result1.Changes, tc.DeepEquals, []string{""})

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("2", nil)
	result2, err := api.WatchUnitAssignments(context.Background())
	c.Assert(err, tc.IsNil)
	c.Check(result2.StringsWatcherId, tc.Equals, "2")
	c.Check(result2.Changes, tc.DeepEquals, []string{""})
}
