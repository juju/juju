// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/registry"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type newLxdProfileSuite struct {
	testing.BaseSuite

	machineTag1 names.MachineTag
	unitTag1    names.UnitTag

	backend *MockLXDProfileBackendV2
	machine *MockLXDProfileMachineV2

	machineService     *MockMachineService
	modelInfoService   *MockModelInfoService
	applicationService *uniter.MockApplicationService
}

var _ = gc.Suite(&newLxdProfileSuite{})

func (s *newLxdProfileSuite) SetUpTest(c *gc.C) {
	s.machineTag1 = names.NewMachineTag("1")
	s.unitTag1 = names.NewUnitTag("mysql/1")
}

func (s *newLxdProfileSuite) TestWatchInstanceData(c *gc.C) {
	defer s.setupMocks(c).Finish()

	watcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	watcher.changes <- struct{}{}
	s.machineService.EXPECT().WatchLXDProfiles(gomock.Any(), "uuid0").Return(watcher, nil)
	s.applicationService.EXPECT().GetUnitMachineUUID(gomock.Any(), coreunit.Name(s.unitTag1.Id())).Return("uuid0", nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}

	api := s.newAPI(c)
	results, err := api.WatchInstanceData(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{NotifyWatcherId: "w-1", Error: nil},
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *newLxdProfileSuite) TestLXDProfileName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetUnitMachineUUID(gomock.Any(), coreunit.Name("mysql/1")).Return("uuid0", nil)
	s.machineService.EXPECT().AppliedLXDProfileNames(gomock.Any(), "uuid0").
		Return([]string{"juju-model-mysql-1"}, nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}

	api := s.newAPI(c)
	results, err := api.LXDProfileName(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Result: "juju-model-mysql-1", Error: nil},
			{Result: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *newLxdProfileSuite) TestLXDProfileRequired(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetCharmLXDProfile(gomock.Any(), applicationcharm.CharmLocator{
		Source:   applicationcharm.CharmHubSource,
		Name:     "mysql",
		Revision: 1,
	}).
		Return(charm.LXDProfile{
			Config: map[string]string{"one": "two"},
		}, 1, nil)

	s.applicationService.EXPECT().GetCharmLXDProfile(gomock.Any(), applicationcharm.CharmLocator{
		Source:   applicationcharm.CharmHubSource,
		Name:     "testme",
		Revision: 3,
	}).Return(charm.LXDProfile{}, 0, errors.NotFoundf("ch:testme-3"))

	args := params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "ch:mysql-1"},
			{URL: "ch:testme-3"},
		},
	}

	api := s.newAPI(c)
	results, err := api.LXDProfileRequired(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: true, Error: nil},
			{Result: false, Error: &params.Error{Message: "ch:testme-3 not found", Code: "not found"}},
		},
	})
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		Type:      model.IAAS,
		CloudType: "lxd",
	}, nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}
	api := s.newAPI(c)
	results, err := api.CanApplyLXDProfile(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Result: false, Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Result: false, Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileIAASLXDNotManual(c *gc.C) {
	// model type: IAAS
	// provider type: lxd
	// manual: false
	defer s.setupMocks(c).Finish()
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		Type:      model.IAAS,
		CloudType: "lxd",
	}, nil)
	s.expectMachine(c)
	s.expectManual(false)

	s.testCanApplyLXDProfile(c, true)
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileIAASLXDManual(c *gc.C) {
	// model type: IAAS
	// provider type: lxd
	// manual: true
	defer s.setupMocks(c).Finish()
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		Type:      model.IAAS,
		CloudType: "lxd",
	}, nil)
	s.expectMachine(c)
	s.expectManual(true)

	s.testCanApplyLXDProfile(c, false)
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileCAAS(c *gc.C) {
	// model type: CAAS
	// provider type: k8s
	defer s.setupMocks(c).Finish()
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		Type:      model.CAAS,
		CloudType: "k8s",
	}, nil)

	s.testCanApplyLXDProfile(c, false)
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileIAASMAASNotManualLXD(c *gc.C) {
	// model type: IAAS
	// provider type: maas
	// manual: false
	// container: LXD
	defer s.setupMocks(c).Finish()
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(model.ModelInfo{
		Type:      model.IAAS,
		CloudType: "maas",
	}, nil)
	s.expectMachine(c)
	s.expectManual(false)
	s.expectContainerType(instance.LXD)

	s.testCanApplyLXDProfile(c, true)
}

func (s *newLxdProfileSuite) testCanApplyLXDProfile(c *gc.C, result bool) {
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.unitTag1.String()},
		},
	}
	api := s.newAPI(c)
	results, err := api.CanApplyLXDProfile(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{{Result: result, Error: nil}},
	})
}

func (s *newLxdProfileSuite) newAPI(c *gc.C) *uniter.LXDProfileAPIv2 {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.unitTag1,
	}
	unitAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}
	watcherRegistry, err := registry.NewRegistry(clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	api := uniter.NewLXDProfileAPIv2(
		s.backend,
		s.machineService,
		watcherRegistry,
		authorizer,
		unitAuthFunc,
		loggertesting.WrapCheckLog(c),
		s.modelInfoService,
		s.applicationService,
	)
	return api
}

func (s *newLxdProfileSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.backend = NewMockLXDProfileBackendV2(ctrl)
	s.machine = NewMockLXDProfileMachineV2(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.applicationService = uniter.NewMockApplicationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	return ctrl
}

func (s *newLxdProfileSuite) expectManual(manual bool) {
	s.machine.EXPECT().IsManual().Return(manual, nil)
}

func (s *newLxdProfileSuite) expectContainerType(cType instance.ContainerType) {
	s.machine.EXPECT().ContainerType().Return(cType)
}

func (s *newLxdProfileSuite) expectMachine(c *gc.C) {
	s.applicationService.EXPECT().GetUnitMachineName(gomock.Any(), coreunit.Name(s.unitTag1.Id())).Return("uuid0", nil)
	s.backend.EXPECT().Machine("uuid0").Return(s.machine, nil)
}
