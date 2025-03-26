// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/replicaset/v3"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/modelupgrader"
	"github.com/juju/juju/apiserver/facades/client/modelupgrader/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	"github.com/juju/juju/internal/docker/registry/image"
	registrymocks "github.com/juju/juju/internal/docker/registry/mocks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/provider/lxd"
	coretesting "github.com/juju/juju/internal/testing"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	upgradevalidationmocks "github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var controllerCfg = controller.Config{
	controller.ControllerUUIDKey: coretesting.ControllerTag.Id(),
	controller.CAASImageRepo: `
{
    "serveraddress": "quay.io",
    "auth": "xxxxx==",
    "repository": "test-account"
}
`[1:],
}

func makeBases(os string, vers []string) []state.Base {
	bases := make([]state.Base, len(vers))
	for i, vers := range vers {
		bases[i] = state.Base{OS: os, Channel: vers}
	}
	return bases
}

type modelUpgradeSuite struct {
	jujutesting.IsolationSuite

	adminUser  names.UserTag
	authoriser apiservertesting.FakeAuthorizer

	statePool               *mocks.MockStatePool
	toolsFinder             *mocks.MockToolsFinder
	bootstrapEnviron        *mocks.MockBootstrapEnviron
	blockChecker            *mocks.MockBlockCheckerInterface
	upgradeService          *mocks.MockUpgradeService
	controllerConfigService *mocks.MockControllerConfigService
	registryProvider        *registrymocks.MockRegistry
	cloudSpec               lxd.CloudSpec

	controllerModelAgentService *mocks.MockModelAgentService
	modelAgentServices          map[model.UUID]*mocks.MockModelAgentService
}

var _ = gc.Suite(&modelUpgradeSuite{})

func (s *modelUpgradeSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	adminUser := "admin"
	s.adminUser = names.NewUserTag(adminUser)

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.adminUser,
	}

	s.cloudSpec = lxd.CloudSpec{CloudSpec: environscloudspec.CloudSpec{Type: "lxd"}}
}

func (s *modelUpgradeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.statePool = mocks.NewMockStatePool(ctrl)
	s.toolsFinder = mocks.NewMockToolsFinder(ctrl)
	s.bootstrapEnviron = mocks.NewMockBootstrapEnviron(ctrl)
	s.blockChecker = mocks.NewMockBlockCheckerInterface(ctrl)
	s.registryProvider = registrymocks.NewMockRegistry(ctrl)
	s.upgradeService = mocks.NewMockUpgradeService(ctrl)
	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.controllerModelAgentService = mocks.NewMockModelAgentService(ctrl)
	s.modelAgentServices = map[model.UUID]*mocks.MockModelAgentService{}

	return ctrl
}

func (s *modelUpgradeSuite) newFacade(c *gc.C) *modelupgrader.ModelUpgraderAPI {
	api, err := modelupgrader.NewModelUpgraderAPI(
		coretesting.ControllerTag,
		s.statePool,
		s.toolsFinder,
		func(ctx context.Context) (environs.BootstrapEnviron, error) {
			return s.bootstrapEnviron, nil
		},
		s.blockChecker, s.authoriser,
		func(docker.ImageRepoDetails) (registry.Registry, error) {
			return s.registryProvider, nil
		},
		func(context.Context, names.ModelTag) (environscloudspec.CloudSpec, error) {
			return s.cloudSpec.CloudSpec, nil
		},
		func(modelUUID model.UUID) modelupgrader.ModelAgentService {
			return s.modelAgentServices[modelUUID]
		},
		s.controllerModelAgentService,
		s.controllerConfigService,
		s.upgradeService,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *modelUpgradeSuite) TestUpgradeModelWithInvalidModelTag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := s.newFacade(c)

	_, err := api.UpgradeModel(context.Background(), params.UpgradeModelParams{ModelTag: "!!!"})
	c.Assert(err, gc.ErrorMatches, `"!!!" is not a valid tag`)
}

func (s *modelUpgradeSuite) TestUpgradeModelWithModelWithNoPermission(c *gc.C) {
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("user"),
	}
	defer s.setupMocks(c).Finish()

	api := s.newFacade(c)

	_, err := api.UpgradeModel(
		context.Background(),
		params.UpgradeModelParams{
			ModelTag:      coretesting.ModelTag.String(),
			TargetVersion: version.MustParse("3.0.0"),
		},
	)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *modelUpgradeSuite) TestUpgradeModelWithChangeNotAllowed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := s.newFacade(c)

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(errors.Errorf("the operation has been blocked"))

	_, err := api.UpgradeModel(
		context.Background(),
		params.UpgradeModelParams{
			ModelTag:      coretesting.ModelTag.String(),
			TargetVersion: version.MustParse("3.0.0"),
		},
	)
	c.Assert(err, gc.ErrorMatches, `the operation has been blocked`)
}

func (s *modelUpgradeSuite) assertUpgradeModelForControllerModelJuju3(c *gc.C, dryRun bool) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	server := upgradevalidationmocks.NewMockServer(ctrl)
	serverFactory := upgradevalidationmocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	ctrlModelUUID := modeltesting.GenModelUUID(c)
	ctrlModelTag := names.NewModelTag(ctrlModelUUID.String())
	model1ModelUUID := modeltesting.GenModelUUID(c)
	ctrlModel := mocks.NewMockModel(ctrl)
	model1 := mocks.NewMockModel(ctrl)
	ctrlModel.EXPECT().IsControllerModel().Return(true).AnyTimes()

	ctrlState := mocks.NewMockState(ctrl)
	state1 := mocks.NewMockState(ctrl)
	ctrlState.EXPECT().Release().AnyTimes()
	ctrlState.EXPECT().Model().Return(ctrlModel, nil)
	state1.EXPECT().Release()

	s.statePool.EXPECT().Get(ctrlModelTag.Id()).Return(ctrlState, nil)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	// Decide/validate target version.
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)

	s.controllerModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("3.9.1"), nil,
	).AnyTimes()
	ctrlModel.EXPECT().Life().Return(state.Alive)
	ctrlModel.EXPECT().Type().Return(state.ModelTypeIAAS)
	s.toolsFinder.EXPECT().FindAgents(
		gomock.Any(),
		common.FindAgentsParams{
			Number:        version.MustParse("3.9.99"),
			ControllerCfg: controllerCfg, ModelType: state.ModelTypeIAAS}).Return(
		[]*coretools.Tools{
			{Version: version.MustParseBinary("3.9.99-ubuntu-amd64")},
		}, nil,
	)

	// 1. Check controller model.
	// - check agent version;
	// - check mongo status;
	ctrlState.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
		Members: []replicaset.MemberStatus{
			{
				Id:      1,
				Address: "1.1.1.1",
				State:   replicaset.PrimaryState,
			},
			{
				Id:      2,
				Address: "2.2.2.2",
				State:   replicaset.SecondaryState,
			},
			{
				Id:      3,
				Address: "3.3.3.3",
				State:   replicaset.SecondaryState,
			},
		},
	}, nil)
	// - check mongo version;
	s.statePool.EXPECT().MongoVersion().Return("4.4", nil)
	// - check if the model has deprecated ubuntu machines;
	ctrlState.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	ctrlState.EXPECT().AllMachinesCount().Return(0, nil)
	serverFactory.EXPECT().RemoteServer(s.cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	ctrlState.EXPECT().AllModelUUIDs().Return([]string{ctrlModelTag.Id(), model1ModelUUID.String()}, nil)

	// 2. Check other models.
	s.statePool.EXPECT().Get(model1ModelUUID.String()).Return(state1, nil)
	state1.EXPECT().Model().Return(model1, nil)
	s.modelAgentServices[ctrlModelUUID] = s.controllerModelAgentService
	s.modelAgentServices[model1ModelUUID] = mocks.NewMockModelAgentService(ctrl)
	s.modelAgentServices[model1ModelUUID].EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("2.9.1"), nil,
	)
	model1.EXPECT().Life().Return(state.Alive)
	// - check agent version;
	//  - check if model migration is ongoing;
	model1.EXPECT().MigrationMode().Return(state.MigrationModeNone)
	// - check if the model has deprecated ubuntu machines;
	state1.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	state1.EXPECT().AllMachinesCount().Return(0, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(s.cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	if !dryRun {
		ctrlState.EXPECT().SetModelAgentVersion(version.MustParse("3.9.99"), nil, false, gomock.Any()).Return(nil)
	}

	api := s.newFacade(c)

	result, err := api.UpgradeModel(
		context.Background(),
		params.UpgradeModelParams{
			ModelTag:      ctrlModelTag.String(),
			TargetVersion: version.MustParse("3.9.99"),
			AgentStream:   "",
			DryRun:        dryRun,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version.MustParse("3.9.99"),
	})
}

func (s *modelUpgradeSuite) TestUpgradeModelForControllerModelJuju3(c *gc.C) {
	s.assertUpgradeModelForControllerModelJuju3(c, false)
}

func (s *modelUpgradeSuite) TestUpgradeModelForControllerModelJuju3DryRun(c *gc.C) {
	s.assertUpgradeModelForControllerModelJuju3(c, true)
}

func (s *modelUpgradeSuite) TestUpgradeModelForControllerDyingHostedModelJuju3(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.1"),
	})

	server := upgradevalidationmocks.NewMockServer(ctrl)
	serverFactory := upgradevalidationmocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	ctrlModelUUID := modeltesting.GenModelUUID(c)
	ctrlModelTag := names.NewModelTag(ctrlModelUUID.String())
	model1ModelUUID := modeltesting.GenModelUUID(c)
	ctrlModel := mocks.NewMockModel(ctrl)
	model1 := mocks.NewMockModel(ctrl)
	ctrlModel.EXPECT().IsControllerModel().Return(true).AnyTimes()

	ctrlState := mocks.NewMockState(ctrl)
	state1 := mocks.NewMockState(ctrl)
	ctrlState.EXPECT().Release().AnyTimes()
	ctrlState.EXPECT().Model().Return(ctrlModel, nil)
	state1.EXPECT().Release()

	s.statePool.EXPECT().Get(ctrlModelTag.Id()).Return(ctrlState, nil)
	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	// Decide/validate target version.
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)

	ctrlModel.EXPECT().Life().Return(state.Alive)
	s.controllerModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("3.9.1"), nil,
	).AnyTimes()
	ctrlModel.EXPECT().Type().Return(state.ModelTypeIAAS)
	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		Number:        version.MustParse("3.9.99"),
		ControllerCfg: controllerCfg, ModelType: state.ModelTypeIAAS}).Return(
		[]*coretools.Tools{
			{Version: version.MustParseBinary("3.9.99-ubuntu-amd64")},
		}, nil,
	)

	// - check mongo status;
	ctrlState.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
		Members: []replicaset.MemberStatus{
			{
				Id:      1,
				Address: "1.1.1.1",
				State:   replicaset.PrimaryState,
			},
			{
				Id:      2,
				Address: "2.2.2.2",
				State:   replicaset.SecondaryState,
			},
			{
				Id:      3,
				Address: "3.3.3.3",
				State:   replicaset.SecondaryState,
			},
		},
	}, nil)
	// - check mongo version;
	s.statePool.EXPECT().MongoVersion().Return("4.4", nil)
	// - check if the model has deprecated ubuntu machines;
	ctrlState.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	ctrlState.EXPECT().AllMachinesCount().Return(0, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(s.cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	ctrlState.EXPECT().AllModelUUIDs().Return([]string{ctrlModelTag.Id(), model1ModelUUID.String()}, nil)

	// 2. Check other models.
	s.statePool.EXPECT().Get(model1ModelUUID.String()).Return(state1, nil)
	state1.EXPECT().Model().Return(model1, nil)
	// Skip this dying model.
	model1.EXPECT().Life().Return(state.Dying)

	s.modelAgentServices[ctrlModelUUID] = s.controllerModelAgentService
	s.modelAgentServices[model1ModelUUID] = mocks.NewMockModelAgentService(ctrl)

	ctrlState.EXPECT().SetModelAgentVersion(version.MustParse("3.9.99"), nil, false, gomock.Any()).Return(nil)

	api := s.newFacade(c)

	result, err := api.UpgradeModel(
		context.Background(),
		params.UpgradeModelParams{
			ModelTag:      ctrlModelTag.String(),
			TargetVersion: version.MustParse("3.9.99"),
			AgentStream:   "",
			DryRun:        false,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version.MustParse("3.9.99"),
	})
}

func (s *modelUpgradeSuite) TestUpgradeModelForControllerModelJuju3Failed(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.PatchValue(&upgradevalidation.MinAgentVersions, map[int]version.Number{
		3: version.MustParse("2.9.2"),
	})

	server := upgradevalidationmocks.NewMockServer(ctrl)
	serverFactory := upgradevalidationmocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	ctrlModelUUID := modeltesting.GenModelUUID(c)
	ctrlModelTag := names.NewModelTag(ctrlModelUUID.String())
	model1ModelUUID := modeltesting.GenModelUUID(c)
	ctrlModel := mocks.NewMockModel(ctrl)
	model1 := mocks.NewMockModel(ctrl)
	ctrlModel.EXPECT().IsControllerModel().Return(true).AnyTimes()

	ctrlState := mocks.NewMockState(ctrl)
	state1 := mocks.NewMockState(ctrl)
	ctrlState.EXPECT().Release().AnyTimes()
	ctrlState.EXPECT().Model().Return(ctrlModel, nil)
	state1.EXPECT().Release()

	s.statePool.EXPECT().Get(ctrlModelTag.Id()).Return(ctrlState, nil)

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	// Decide/validate target version.
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)

	ctrlModel.EXPECT().Life().Return(state.Alive)
	s.controllerModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("2.9.1"), nil,
	).AnyTimes()
	ctrlModel.EXPECT().Type().Return(state.ModelTypeIAAS)
	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		Number:        version.MustParse("3.9.99"),
		ControllerCfg: controllerCfg, ModelType: state.ModelTypeIAAS}).Return(
		[]*coretools.Tools{
			{Version: version.MustParseBinary("3.9.99-ubuntu-amd64")},
		}, nil,
	)

	// - check mongo status;
	ctrlState.EXPECT().MongoCurrentStatus().Return(&replicaset.Status{
		Members: []replicaset.MemberStatus{
			{
				Id:      1,
				Address: "1.1.1.1",
				State:   replicaset.FatalState,
			},
			{
				Id:      2,
				Address: "2.2.2.2",
				State:   replicaset.ArbiterState,
			},
			{
				Id:      3,
				Address: "3.3.3.3",
				State:   replicaset.RecoveringState,
			},
		},
	}, nil)
	// - check mongo version;
	s.statePool.EXPECT().MongoVersion().Return("4.3", nil)
	// - check if the model has deprecated ubuntu machines;
	ctrlState.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	ctrlState.EXPECT().AllMachinesCount().Return(1, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(s.cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("4.0")
	ctrlModel.EXPECT().Owner().Return(names.NewUserTag("admin"))
	ctrlModel.EXPECT().Name().Return("controller")

	ctrlState.EXPECT().AllModelUUIDs().Return([]string{ctrlModelTag.Id(), model1ModelUUID.String()}, nil)
	// 2. Check other models.
	s.statePool.EXPECT().Get(model1ModelUUID.String()).Return(state1, nil)
	state1.EXPECT().Model().Return(model1, nil)
	model1.EXPECT().Life().Return(state.Alive)
	// - check agent version;
	s.modelAgentServices[ctrlModelUUID] = s.controllerModelAgentService
	s.modelAgentServices[model1ModelUUID] = mocks.NewMockModelAgentService(ctrl)
	s.modelAgentServices[model1ModelUUID].EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("2.9.0"), nil,
	)
	//  - check if model migration is ongoing;
	model1.EXPECT().MigrationMode().Return(state.MigrationModeExporting)
	// - check if the model has deprecated ubuntu machines;
	state1.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(map[string]int{
		"ubuntu@20.04": 1, "ubuntu@22.04": 2, "ubuntu@24.04": 3,
	}, nil)
	state1.EXPECT().AllMachinesCount().Return(7, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(s.cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("4.0")
	model1.EXPECT().Owner().Return(names.NewUserTag("admin"))
	model1.EXPECT().Name().Return("model-1")

	api := s.newFacade(c)

	result, err := api.UpgradeModel(
		context.Background(),
		params.UpgradeModelParams{
			ModelTag:      ctrlModelTag.String(),
			TargetVersion: version.MustParse("3.9.99"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error.Error(), gc.Equals, `
cannot upgrade to "3.9.99" due to issues with these models:
"admin/controller":
- upgrading a controller to a newer major.minor version 3.9 not supported
- unable to upgrade, database node 1 (1.1.1.1) has state FATAL, node 2 (2.2.2.2) has state ARBITER, node 3 (3.3.3.3) has state RECOVERING
- mongo version has to be "4.4" at least, but current version is "4.3"
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04
- LXD version has to be at least "5.0.0", but current version is only "4.0.0"
"admin/model-1":
- current model ("2.9.0") has to be upgraded to "2.9.2" at least
- model is under "exporting" mode, upgrade blocked
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04
- LXD version has to be at least "5.0.0", but current version is only "4.0.0"`[1:])
}

func (s *modelUpgradeSuite) assertUpgradeModelJuju3(c *gc.C, ctrlModelVers string, dryRun bool) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	server := upgradevalidationmocks.NewMockServer(ctrl)
	serverFactory := upgradevalidationmocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return []base.Base{
			base.MustParseBaseFromString("ubuntu@24.04"),
			base.MustParseBaseFromString("ubuntu@22.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		}
	})

	modelUUID := modeltesting.GenModelUUID(c)
	model := mocks.NewMockModel(ctrl)
	st := mocks.NewMockState(ctrl)
	st.EXPECT().Release().AnyTimes()

	s.statePool.EXPECT().Get(modelUUID.String()).AnyTimes().Return(st, nil)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	var agentStream string

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	// Decide/validate target version.
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)

	s.modelAgentServices[modelUUID] = mocks.NewMockModelAgentService(ctrl)
	s.modelAgentServices[modelUUID].EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("2.9.1"), nil,
	)
	model.EXPECT().Life().Return(state.Alive)
	model.EXPECT().IsControllerModel().Return(false).AnyTimes()
	s.controllerModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse(ctrlModelVers), nil,
	)
	if ctrlModelVers != "3.9.99" {
		model.EXPECT().Type().Return(state.ModelTypeIAAS)
		s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
			Number:        version.MustParse("3.9.99"),
			ControllerCfg: controllerCfg, ModelType: state.ModelTypeIAAS}).Return(
			[]*coretools.Tools{
				{Version: version.MustParseBinary("3.9.99-ubuntu-amd64")},
			}, nil,
		)
	}

	// - check if the model has deprecated ubuntu machines;
	st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	st.EXPECT().AllMachinesCount().Return(0, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(s.cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("5.2")

	if !dryRun {
		st.EXPECT().SetModelAgentVersion(version.MustParse("3.9.99"), nil, false, gomock.Any()).Return(nil)
	}

	api := s.newFacade(c)

	result, err := api.UpgradeModel(
		context.Background(),
		params.UpgradeModelParams{
			ModelTag:      names.NewModelTag(modelUUID.String()).String(),
			TargetVersion: version.MustParse("3.9.99"),
			AgentStream:   agentStream,
			DryRun:        dryRun,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.UpgradeModelResult{
		ChosenVersion: version.MustParse("3.9.99"),
	})
}

func (s *modelUpgradeSuite) TestUpgradeModelJuju3(c *gc.C) {
	s.assertUpgradeModelJuju3(c, "3.10.0", false)
}

func (s *modelUpgradeSuite) TestUpgradeModelJuju3SameAsController(c *gc.C) {
	s.assertUpgradeModelJuju3(c, "3.9.99", false)
}

func (s *modelUpgradeSuite) TestUpgradeModelJuju3DryRun(c *gc.C) {
	s.assertUpgradeModelJuju3(c, "3.10.0", true)
}

func (s *modelUpgradeSuite) TestUpgradeModelJuju3Failed(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	server := upgradevalidationmocks.NewMockServer(ctrl)
	serverFactory := upgradevalidationmocks.NewMockServerFactory(ctrl)
	s.PatchValue(&upgradevalidation.NewServerFactory,
		func(_ lxd.NewHTTPClientFunc) lxd.ServerFactory {
			return serverFactory
		},
	)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	modelUUID := modeltesting.GenModelUUID(c)
	model := mocks.NewMockModel(ctrl)
	st := mocks.NewMockState(ctrl)
	st.EXPECT().Release()

	s.statePool.EXPECT().Get(modelUUID.String()).AnyTimes().Return(st, nil)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	s.blockChecker.EXPECT().ChangeAllowed(context.Background()).Return(nil)

	// Decide/validate target version.
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)

	s.modelAgentServices[modelUUID] = mocks.NewMockModelAgentService(ctrl)
	s.modelAgentServices[modelUUID].EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("2.9.1"), nil,
	)
	model.EXPECT().Life().Return(state.Alive)
	model.EXPECT().Type().Return(state.ModelTypeIAAS)
	model.EXPECT().IsControllerModel().Return(false).AnyTimes()
	s.controllerModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("3.10.0"), nil,
	)
	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		Number:        version.MustParse("3.9.99"),
		ControllerCfg: controllerCfg, ModelType: state.ModelTypeIAAS}).Return(
		[]*coretools.Tools{
			{Version: version.MustParseBinary("3.9.99-ubuntu-amd64")},
		}, nil,
	)

	// - check if the model has deprecated ubuntu machines;
	st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(map[string]int{
		"ubuntu@20.04": 1, "ubuntu@22.04": 2, "ubuntu@24.04": 3,
	}, nil)
	st.EXPECT().AllMachinesCount().Return(7, nil)
	// - check LXD version.
	serverFactory.EXPECT().RemoteServer(s.cloudSpec).Return(server, nil)
	server.EXPECT().ServerVersion().Return("4.0")
	model.EXPECT().Owner().Return(names.NewUserTag("admin"))
	model.EXPECT().Name().Return("model-1")

	api := s.newFacade(c)

	result, err := api.UpgradeModel(
		context.Background(),
		params.UpgradeModelParams{
			ModelTag:      names.NewModelTag(modelUUID.String()).String(),
			TargetVersion: version.MustParse("3.9.99"),
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error.Error(), gc.Equals, `
cannot upgrade to "3.9.99" due to issues with these models:
"admin/model-1":
- the model hosts 1 ubuntu machine(s) with an unsupported base. The supported bases are: ubuntu@24.04, ubuntu@22.04, ubuntu@20.04
- LXD version has to be at least "5.0.0", but current version is only "4.0.0"`[1:])
}

func (s *modelUpgradeSuite) TestCannotUpgradePastControllerVersion(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	api := s.newFacade(c)

	modelUUID := modeltesting.GenModelUUID(c)
	model := mocks.NewMockModel(ctrl)
	st := mocks.NewMockState(ctrl)
	st.EXPECT().Release().AnyTimes()

	s.statePool.EXPECT().Get(modelUUID.String()).AnyTimes().Return(st, nil)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	s.blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(controllerCfg, nil)
	s.modelAgentServices[modelUUID] = mocks.NewMockModelAgentService(ctrl)
	s.modelAgentServices[modelUUID].EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("2.9.1"), nil,
	)
	model.EXPECT().Life().Return(state.Alive)
	model.EXPECT().IsControllerModel().Return(false)
	s.controllerModelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(
		version.MustParse("3.9.99"), nil,
	)

	_, err := api.UpgradeModel(context.Background(),
		params.UpgradeModelParams{
			ModelTag:      names.NewModelTag(modelUUID.String()).String(),
			TargetVersion: version.MustParse("3.12.0"),
		},
	)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade to a version "3.12.0" greater than that of the controller "3.9.99"`)
}

func (s *modelUpgradeSuite) TestFindToolsIAAS(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	simpleStreams := []*coretools.Tools{
		{Version: version.MustParseBinary("2.9.6-ubuntu-amd64")},
	}

	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		MajorVersion: 2, ModelType: state.ModelTypeIAAS}).Return(simpleStreams, nil)

	api := s.newFacade(c)
	result, err := api.FindAgents(context.Background(), common.FindAgentsParams{MajorVersion: 2, ModelType: state.ModelTypeIAAS})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coretools.Versions{
		&coretools.Tools{Version: version.MustParseBinary("2.9.6-ubuntu-amd64")},
	})
}

func (s *modelUpgradeSuite) TestFindToolsCAASReleasedDefault(c *gc.C) {
	s.assertFindToolsCAASReleased(c, "", "amd64")
}

func (s *modelUpgradeSuite) TestFindToolsCAASReleased(c *gc.C) {
	s.assertFindToolsCAASReleased(c, "arm64", "arm64")
}

func (s *modelUpgradeSuite) assertFindToolsCAASReleased(c *gc.C, wantArch, expectArch string) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	simpleStreams := []*coretools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.9-ubuntu-arm64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-arm64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-arm64")},
	}
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	gomock.InOrder(
		s.toolsFinder.EXPECT().FindAgents(
			gomock.Any(),
			common.FindAgentsParams{
				MajorVersion: 2, MinorVersion: 9,
				ModelType: state.ModelTypeCAAS,
				Arch:      wantArch,
			},
		).Return(simpleStreams, nil),
		s.registryProvider.EXPECT().Tags("jujud-operator").Return(coretools.Versions{
			image.NewImageInfo(version.MustParse("2.9.8")),
			image.NewImageInfo(version.MustParse("2.9.9")),
			image.NewImageInfo(version.MustParse("2.9.10.1")),
			image.NewImageInfo(version.MustParse("2.9.10")),
			image.NewImageInfo(version.MustParse("2.9.11")),
			image.NewImageInfo(version.MustParse("2.9.12")), // skip: it's not released in simplestream yet.
		}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.9").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10.1").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.11").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().Close().Return(nil),
	)

	api := s.newFacade(c)
	result, err := api.FindAgents(context.Background(), common.FindAgentsParams{MajorVersion: 2, MinorVersion: 9, ModelType: state.ModelTypeCAAS, Arch: wantArch})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coretools.Versions{
		&coretools.Tools{Version: version.MustParseBinary("2.9.9-ubuntu-" + expectArch)},
		&coretools.Tools{Version: version.MustParseBinary("2.9.10.1-ubuntu-" + expectArch)},
		&coretools.Tools{Version: version.MustParseBinary("2.9.10-ubuntu-" + expectArch)},
		&coretools.Tools{Version: version.MustParseBinary("2.9.11-ubuntu-" + expectArch)},
	})
}

func (s *modelUpgradeSuite) TestFindToolsCAASReleasedExact(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	simpleStreams := []*coretools.Tools{
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
	}
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	gomock.InOrder(
		s.toolsFinder.EXPECT().FindAgents(gomock.Any(),
			common.FindAgentsParams{
				Number:    version.MustParse("2.9.10"),
				ModelType: state.ModelTypeCAAS,
			}).Return(simpleStreams, nil),
		s.registryProvider.EXPECT().Tags("jujud-operator").Return(coretools.Versions{
			image.NewImageInfo(version.MustParse("2.9.8")),
			image.NewImageInfo(version.MustParse("2.9.9")),
			image.NewImageInfo(version.MustParse("2.9.10.1")),
			image.NewImageInfo(version.MustParse("2.9.10")),
			image.NewImageInfo(version.MustParse("2.9.11")),
			image.NewImageInfo(version.MustParse("2.9.12")),
		}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10").Return([]string{"amd64"}, nil),
		s.registryProvider.EXPECT().Close().Return(nil),
	)

	api := s.newFacade(c)
	result, err := api.FindAgents(context.Background(), common.FindAgentsParams{
		Number: version.MustParse("2.9.10"), ModelType: state.ModelTypeCAAS})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coretools.Versions{
		&coretools.Tools{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
	})
}

func (s *modelUpgradeSuite) TestFindToolsCAASNonReleased(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	simpleStreams := []*coretools.Tools{
		{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.9.12-ubuntu-amd64")},
	}
	s.PatchValue(&coreos.HostOS, func() ostype.OSType { return ostype.Ubuntu })

	gomock.InOrder(
		s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
			MajorVersion: 2, MinorVersion: 9, AgentStream: envtools.DevelStream,
			ModelType: state.ModelTypeCAAS,
		}).Return(simpleStreams, nil),
		s.registryProvider.EXPECT().Tags("jujud-operator").Return(coretools.Versions{
			image.NewImageInfo(version.MustParse("2.9.8")), // skip: it's not released in simplestream yet.
			image.NewImageInfo(version.MustParse("2.9.9")),
			image.NewImageInfo(version.MustParse("2.9.10.1")),
			image.NewImageInfo(version.MustParse("2.9.10")),
			image.NewImageInfo(version.MustParse("2.9.11")),
			image.NewImageInfo(version.MustParse("2.9.12")),
			image.NewImageInfo(version.MustParse("2.9.13")), // skip: it's not released in simplestream yet.
		}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.9").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10.1").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.10").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.11").Return([]string{"amd64", "arm64"}, nil),
		s.registryProvider.EXPECT().GetArchitectures("jujud-operator", "2.9.12").Return(nil, errors.NotFoundf("2.9.12")), // This can only happen on a non-official registry account.
		s.registryProvider.EXPECT().Close().Return(nil),
	)

	api := s.newFacade(c)
	result, err := api.FindAgents(
		context.Background(),
		common.FindAgentsParams{
			MajorVersion: 2, MinorVersion: 9, AgentStream: envtools.DevelStream,
			ModelType: state.ModelTypeCAAS,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, coretools.Versions{
		&coretools.Tools{Version: version.MustParseBinary("2.9.9-ubuntu-amd64")},
		&coretools.Tools{Version: version.MustParseBinary("2.9.10.1-ubuntu-amd64")},
		&coretools.Tools{Version: version.MustParseBinary("2.9.10-ubuntu-amd64")},
		&coretools.Tools{Version: version.MustParseBinary("2.9.11-ubuntu-amd64")},
	})
}

func (s *modelUpgradeSuite) TestDecideVersionFindToolUseAgentVersionMajorMinor(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		MajorVersion: 3, MinorVersion: 666,
		ModelType: state.ModelTypeIAAS,
	}).Return(nil, errors.New(`fail to exit early`))

	api := s.newFacade(c)
	targetVersion, err := api.DecideVersion(
		context.Background(),
		version.MustParse("3.9.99"), common.FindAgentsParams{
			MajorVersion: 3, MinorVersion: 666, ModelType: state.ModelTypeIAAS},
	)
	c.Assert(err, gc.ErrorMatches, `cannot find agents from simple streams: fail to exit early`)
	c.Assert(targetVersion, gc.DeepEquals, version.Zero)
}

func (s *modelUpgradeSuite) TestDecideVersionFindToolUseTargetMajor(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		Number:    version.MustParse("4.9.99"),
		ModelType: state.ModelTypeIAAS,
	}).Return(nil, errors.New(`fail to exit early`))

	api := s.newFacade(c)
	targetVersion, err := api.DecideVersion(
		context.Background(),
		version.MustParse("3.9.99"),
		common.FindAgentsParams{Number: version.MustParse("4.9.99"), ModelType: state.ModelTypeIAAS},
	)
	c.Assert(err, gc.ErrorMatches, `cannot find agents from simple streams: fail to exit early`)
	c.Assert(targetVersion, gc.DeepEquals, version.Zero)
}

func (s *modelUpgradeSuite) TestDecideVersionValidateAndUseTargetVersion(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	simpleStreams := []*coretools.Tools{
		{Version: version.MustParseBinary("3.9.98-ubuntu-amd64")},
	}

	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		Number: version.MustParse("3.9.98"), ModelType: state.ModelTypeIAAS,
	}).Return(simpleStreams, nil)

	api := s.newFacade(c)
	targetVersion, err := api.DecideVersion(
		context.Background(),
		version.MustParse("2.9.99"),
		common.FindAgentsParams{
			Number: version.MustParse("3.9.98"), ModelType: state.ModelTypeIAAS},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(targetVersion, gc.DeepEquals, version.MustParse("3.9.98"))
}

func (s *modelUpgradeSuite) TestDecideVersionNewestMinor(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	simpleStreams := []*coretools.Tools{
		{Version: version.MustParseBinary("2.9.100-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.10.99-ubuntu-amd64")},
		{Version: version.MustParseBinary("2.11.99-ubuntu-amd64")},
		{Version: version.MustParseBinary("3.666.0-ubuntu-amd64")},
	}

	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		MajorVersion: 2,
		ModelType:    state.ModelTypeIAAS,
	}).Return(simpleStreams, nil)

	api := s.newFacade(c)
	targetVersion, err := api.DecideVersion(
		context.Background(),
		version.MustParse("2.9.99"),
		common.FindAgentsParams{
			MajorVersion: 2, MinorVersion: 0,
			ModelType: state.ModelTypeIAAS},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(targetVersion, gc.DeepEquals, version.MustParse("2.9.100"))
}

func (s *modelUpgradeSuite) TestDecideVersionIgnoresNewerMajor(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	st := mocks.NewMockState(ctrl)
	model := mocks.NewMockModel(ctrl)
	st.EXPECT().Model().AnyTimes().Return(model, nil)

	simpleStreams := []*coretools.Tools{
		{Version: version.MustParseBinary("2.9.100-ubuntu-amd64")},
		{Version: version.MustParseBinary("3.666.0-ubuntu-amd64")},
	}

	s.toolsFinder.EXPECT().FindAgents(gomock.Any(), common.FindAgentsParams{
		MajorVersion: 2,
		ModelType:    state.ModelTypeIAAS,
	}).Return(simpleStreams, nil)

	api := s.newFacade(c)
	targetVersion, err := api.DecideVersion(
		context.Background(),
		version.MustParse("2.9.99"),
		common.FindAgentsParams{
			MajorVersion: 2,
			ModelType:    state.ModelTypeIAAS},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(targetVersion, gc.DeepEquals, version.MustParse("2.9.100"))
}
