// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/agent/uniter/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type newLxdProfileSuite struct {
	testing.BaseSuite

	machineTag1 names.MachineTag
	unitTag1    names.UnitTag

	backend *mocks.MockLXDProfileBackendV2
	charm   *mocks.MockLXDProfileCharmV2
	machine *mocks.MockLXDProfileMachineV2
	model   *mocks.MockLXDProfileModelV2
	unit    *mocks.MockLXDProfileUnitV2
}

var _ = gc.Suite(&newLxdProfileSuite{})

func (s *newLxdProfileSuite) SetUpTest(c *gc.C) {
	s.machineTag1 = names.NewMachineTag("1")
	s.unitTag1 = names.NewUnitTag("mysql/1")
}

func (s *newLxdProfileSuite) TestWatchInstanceData(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectUnitAndMachine()
	s.expectWatchInstanceData()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}

	api := s.newAPI()
	results, err := api.WatchInstanceData(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{NotifyWatcherId: "1", Error: nil},
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *newLxdProfileSuite) TestLXDProfileName(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectUnitAndMachine()
	s.expectOneLXDProfileName()

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: s.unitTag1.String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}

	api := s.newAPI()
	results, err := api.LXDProfileName(args)
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
	s.expectOneLXDProfileRequired()

	args := params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "ch:mysql-1"},
			{URL: "ch:testme-3"},
		},
	}

	api := s.newAPI()
	results, err := api.LXDProfileRequired(args)
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
	s.expectModel()
	s.expectModelTypeIAAS()
	s.expectProviderType(c, "lxd")

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("mysql/2").String()},
			{Tag: names.NewMachineTag("2").String()},
		},
	}
	api := s.newAPI()
	results, err := api.CanApplyLXDProfile(args)
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
	s.expectUnitAndMachine()
	s.expectModel()
	s.expectModelTypeIAAS()
	s.expectProviderType(c, "lxd")
	s.expectManual(false)

	s.testCanApplyLXDProfile(c, true)
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileIAASLXDManual(c *gc.C) {
	// model type: IAAS
	// provider type: lxd
	// manual: true
	defer s.setupMocks(c).Finish()
	s.expectUnitAndMachine()
	s.expectModel()
	s.expectModelTypeIAAS()
	s.expectProviderType(c, "lxd")
	s.expectManual(true)

	s.testCanApplyLXDProfile(c, false)
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileCAAS(c *gc.C) {
	// model type: CAAS
	// provider type: k8s
	defer s.setupMocks(c).Finish()
	s.expectModel()
	s.expectModelTypeCAAS()
	s.expectProviderType(c, "k8s")

	s.testCanApplyLXDProfile(c, false)
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileIAASMAASNotManualKVM(c *gc.C) {
	// model type: IAAS
	// provider type: maas
	// manual: false
	// container: KVM
	defer s.setupMocks(c).Finish()
	s.expectUnitAndMachine()
	s.expectModel()
	s.expectModelTypeIAAS()
	s.expectProviderType(c, "maas")
	s.expectManual(false)
	s.expectContainerType(instance.KVM)

	s.testCanApplyLXDProfile(c, false)
}

func (s *newLxdProfileSuite) TestCanApplyLXDProfileIAASMAASNotManualLXD(c *gc.C) {
	// model type: IAAS
	// provider type: maas
	// manual: false
	// container: LXD
	defer s.setupMocks(c).Finish()
	s.expectUnitAndMachine()
	s.expectModel()
	s.expectModelTypeIAAS()
	s.expectProviderType(c, "maas")
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
	api := s.newAPI()
	results, err := api.CanApplyLXDProfile(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{{Result: result, Error: nil}},
	})
}

func (s *newLxdProfileSuite) newAPI() *uniter.LXDProfileAPIv2 {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: s.unitTag1,
	}
	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}
	api := uniter.NewLXDProfileAPIv2(
		s.backend,
		resources,
		authorizer,
		unitAuthFunc,
		loggo.GetLogger("juju.apiserver.facades.agent.uniter"))
	return api
}

func (s *newLxdProfileSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.backend = mocks.NewMockLXDProfileBackendV2(ctrl)
	s.charm = mocks.NewMockLXDProfileCharmV2(ctrl)
	s.machine = mocks.NewMockLXDProfileMachineV2(ctrl)
	s.model = mocks.NewMockLXDProfileModelV2(ctrl)
	s.unit = mocks.NewMockLXDProfileUnitV2(ctrl)
	return ctrl
}

func (s *newLxdProfileSuite) expectUnitAndMachine() {
	s.backend.EXPECT().Unit(s.unitTag1.Id()).Return(s.unit, nil)
	s.unit.EXPECT().AssignedMachineId().Return(s.machineTag1.Id(), nil)

	s.backend.EXPECT().Machine(s.machineTag1.Id()).Return(s.machine, nil)
}

func (s *newLxdProfileSuite) expectOneLXDProfileName() {
	s.machine.EXPECT().CharmProfiles().Return([]string{"default", "juju-model-mysql-1"}, nil)
	s.unit.EXPECT().ApplicationName().Return("mysql")
}

func (s *newLxdProfileSuite) expectOneLXDProfileRequired() {
	s.backend.EXPECT().Charm("ch:mysql-1").Return(s.charm, nil)
	s.charm.EXPECT().LXDProfile().Return(lxdprofile.Profile{Config: map[string]string{"one": "two"}})

	s.backend.EXPECT().Charm("ch:testme-3").Return(nil, errors.NotFoundf("ch:testme-3"))
}

func (s *newLxdProfileSuite) expectModel() {
	s.backend.EXPECT().Model().Return(s.model, nil)
}

func (s *newLxdProfileSuite) expectModelTypeIAAS() {
	s.model.EXPECT().Type().Return(state.ModelTypeIAAS)
}

func (s *newLxdProfileSuite) expectModelTypeCAAS() {
	s.model.EXPECT().Type().Return(state.ModelTypeCAAS)
}

func (s *newLxdProfileSuite) expectProviderType(c *gc.C, pType string) {
	attrs := map[string]interface{}{
		config.TypeKey:          pType,
		config.NameKey:          "testmodel",
		config.UUIDKey:          "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		config.SecretBackendKey: "auto",
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	s.model.EXPECT().ModelConfig().Return(cfg, nil)
}

func (s *newLxdProfileSuite) expectManual(manual bool) {
	s.machine.EXPECT().IsManual().Return(manual, nil)
}

func (s *newLxdProfileSuite) expectContainerType(cType instance.ContainerType) {
	s.machine.EXPECT().ContainerType().Return(cType)
}

func (s *newLxdProfileSuite) expectWatchInstanceData() {
	watcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	watcher.changes <- struct{}{}

	s.machine.EXPECT().WatchInstanceData().Return(watcher)
}
