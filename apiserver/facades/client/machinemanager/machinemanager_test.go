// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"strings"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentbinary"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineservice "github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

type AddMachineManagerSuite struct {
	authorizer     *apiservertesting.FakeAuthorizer
	modelUUID      coremodel.UUID
	controllerUUID string
	api            *MachineManagerAPI
	cloudService   *commonmocks.MockCloudService

	machineService      *MockMachineService
	networkService      *MockNetworkService
	blockCommandService *MockBlockCommandService
}

func TestAddMachineManagerSuite(t *testing.T) {
	tc.Run(t, &AddMachineManagerSuite{})
}

func (s *AddMachineManagerSuite) SetUpTest(c *tc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.modelUUID = coremodel.GenUUID(c)
	s.controllerUUID = uuid.MustNewUUID().String()
}

func (s *AddMachineManagerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)

	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound).AnyTimes()

	s.api = NewMachineManagerAPI(
		s.controllerUUID,
		s.modelUUID,
		nil,
		ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
		Services{
			BlockCommandService: s.blockCommandService,
			CloudService:        s.cloudService,
			MachineService:      s.machineService,
			NetworkService:      s.networkService,
		},
	)

	c.Cleanup(func() {
		s.blockCommandService = nil
		s.cloudService = nil
		s.machineService = nil
		s.api = nil
		s.networkService = nil
	})

	return ctrl
}

func (s *AddMachineManagerSuite) TestAddMachines(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	apiParams := make([]params.AddMachineParams, 2)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Base: &params.Base{Name: "ubuntu", Channel: "22.04"},
			Jobs: []coremodel.MachineJob{coremodel.JobHostUnits},
		}
	}
	apiParams[0].Disks = []storage.Directive{{Size: 1, Count: 2}, {Size: 2, Count: 1}}
	apiParams[1].Disks = []storage.Directive{{Size: 1, Count: 2, Pool: "three"}}

	// Machine 666.
	s.machineService.EXPECT().AddMachine(gomock.Any(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "22.04/stable",
			OSType:  deployment.Ubuntu,
		},
	})
	// Machine 667.
	s.machineService.EXPECT().AddMachine(gomock.Any(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "22.04/stable",
			OSType:  deployment.Ubuntu,
		},
	})

	machines, err := s.api.AddMachines(c.Context(), params.AddMachines{MachineParams: apiParams})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(machines.Machines, tc.HasLen, 2)
}

func (s *AddMachineManagerSuite) TestAddMachinesStateError(c *tc.C) {
	defer s.setup(c).Finish()

	s.machineService.EXPECT().AddMachine(gomock.Any(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "22.04/stable",
			OSType:  deployment.Ubuntu,
		},
	}).Return(machineservice.AddMachineResults{}, errors.New("boom"))

	results, err := s.api.AddMachines(c.Context(), params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Base: &params.Base{Name: "ubuntu", Channel: "22.04"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.AddMachinesResults{
		Machines: []params.AddMachinesResult{{
			Error: &params.Error{Message: "boom", Code: ""},
		}},
	})
}

type DestroyMachineManagerSuite struct {
	testhelpers.CleanupSuite
	authorizer     *apiservertesting.FakeAuthorizer
	api            *MachineManagerAPI
	modelUUID      coremodel.UUID
	controllerUUID string

	machineService      *MockMachineService
	applicationService  *MockApplicationService
	blockCommandService *MockBlockCommandService
	removalService      *MockRemovalService
}

func TestDestroyMachineManagerSuite(t *testing.T) {
	tc.Run(t, &DestroyMachineManagerSuite{})
}

func (s *DestroyMachineManagerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- TestForceDestroyMachineFailedSomeStorageRetrievalManyMachines 
- TestDestroyMachineFailedAllStorageRetrieval
- TestDestroyMachineFailedSomeUnitStorageRetrieval
- TestDestroyMachineFailedSomeStorageRetrievalManyMachines
`)
}

func (s *DestroyMachineManagerSuite) SetUpTest(c *tc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.modelUUID = coremodel.GenUUID(c)
	s.controllerUUID = uuid.MustNewUUID().String()
}

func (s *DestroyMachineManagerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)

	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound).AnyTimes()

	s.api = NewMachineManagerAPI(
		s.controllerUUID,
		s.modelUUID,
		nil,
		ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
		Services{
			ApplicationService:  s.applicationService,
			BlockCommandService: s.blockCommandService,
			MachineService:      s.machineService,
			RemovalService:      s.removalService,
		},
	)

	c.Cleanup(func() {
		s.blockCommandService = nil
		s.machineService = nil
		s.api = nil
		s.api = nil
	})

	return ctrl
}

func (s *DestroyMachineManagerSuite) expectDestroyMachine(
	c *tc.C, ctrl *gomock.Controller, machineName coremachine.Name, unitNames []coreunit.Name,
	containers []coremachine.Name, attemptDestroy, keep, force bool,
) {
	machineUUID := coremachine.GenUUID(c)
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), machineName).Return(machineUUID, nil).MaxTimes(1)

	s.machineService.EXPECT().GetMachineContainers(gomock.Any(), machineName).Return(containers, nil)

	if unitNames == nil {
		unitNames = []coreunit.Name{"foo/0", "foo/1", "foo/2"}
	}

	s.applicationService.EXPECT().GetUnitNamesOnMachine(gomock.Any(), machineName).Return(unitNames, nil)

	if attemptDestroy {
		s.removalService.EXPECT().RemoveMachine(gomock.Any(), machineUUID, force, gomock.Any()).Return("", nil)
	}
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineDryRun(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectDestroyMachine(c, ctrl, "0", nil, nil, false, false, false)

	results, err := s.api.DestroyMachineWithParams(c.Context(), params.DestroyMachinesParams{
		MachineTags: []string{"machine-0"},
		DryRun:      true,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithContainersDryRun(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectDestroyMachine(c, ctrl, "0", nil, []coremachine.Name{"0/lxd/0"}, false, false, false)
	s.expectDestroyMachine(c, ctrl, "0/lxd/0", nil, nil, false, false, false)

	results, err := s.api.DestroyMachineWithParams(c.Context(), params.DestroyMachinesParams{
		MachineTags: []string{"machine-0"},
		DryRun:      true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DestroyedContainers: []params.DestroyMachineResult{{
					Info: &params.DestroyMachineInfo{
						MachineId: "0/lxd/0",
						DestroyedUnits: []params.Entity{
							{"unit-foo-0"},
							{"unit-foo-1"},
							{"unit-foo-2"},
						},
					},
				}},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithParamsNoWait(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectDestroyMachine(c, ctrl, "0", nil, nil, true, true, true)
	s.machineService.EXPECT().SetKeepInstance(gomock.Any(), coremachine.Name("0"), true)

	noWait := 0 * time.Second
	results, err := s.api.DestroyMachineWithParams(c.Context(), params.DestroyMachinesParams{
		Keep:        true,
		Force:       true,
		MachineTags: []string{"machine-0"},
		MaxWait:     &noWait,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithParamsNilWait(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectDestroyMachine(c, ctrl, "0", nil, nil, true, true, true)
	s.machineService.EXPECT().SetKeepInstance(gomock.Any(), coremachine.Name("0"), true)

	results, err := s.api.DestroyMachineWithParams(c.Context(), params.DestroyMachinesParams{
		Keep:        true,
		Force:       true,
		MachineTags: []string{"machine-0"},
		// This will use max wait of system default for delay between cleanup operations.
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithContainers(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectDestroyMachine(c, ctrl, "0", nil, []coremachine.Name{"0/lxd/0"}, true, false, true)
	s.expectDestroyMachine(c, ctrl, "0/lxd/0", nil, nil, false, false, true)

	results, err := s.api.DestroyMachineWithParams(c.Context(), params.DestroyMachinesParams{
		Force:       true,
		MachineTags: []string{"machine-0"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DestroyedContainers: []params.DestroyMachineResult{{
					Info: &params.DestroyMachineInfo{
						MachineId: "0/lxd/0",
						DestroyedUnits: []params.Entity{
							{"unit-foo-0"},
							{"unit-foo-1"},
							{"unit-foo-2"},
						},
					},
				}},
			},
		}},
	})
}

type ProvisioningMachineManagerSuite struct {
	authorizer     *apiservertesting.FakeAuthorizer
	clock          clock.Clock
	cloudService   *commonmocks.MockCloudService
	api            *MachineManagerAPI
	modelUUID      coremodel.UUID
	controllerUUID string

	controllerConfigService *MockControllerConfigService
	controllerNodeService   *MockControllerNodeService
	machineService          *MockMachineService
	statusService           *MockStatusService
	keyUpdaterService       *MockKeyUpdaterService
	modelConfigService      *MockModelConfigService
	bootstrapEnviron        *MockBootstrapEnviron
	blockCommandService     *MockBlockCommandService
	agentBinaryService      *MockAgentBinaryService
	agentPasswordService    *MockAgentPasswordService
}

func TestProvisioningMachineManagerSuite(t *testing.T) {
	tc.Run(t, &ProvisioningMachineManagerSuite{})
}

func (s *ProvisioningMachineManagerSuite) SetUpTest(c *tc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
}

func (s *ProvisioningMachineManagerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = coremodel.GenUUID(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil).AnyTimes()
	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.statusService = NewMockStatusService(ctrl)

	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.bootstrapEnviron = NewMockBootstrapEnviron(ctrl)
	s.clock = testclock.NewClock(time.Now())

	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound).AnyTimes()

	s.agentBinaryService = NewMockAgentBinaryService(ctrl)
	s.agentPasswordService = NewMockAgentPasswordService(ctrl)

	s.api = NewMachineManagerAPI(
		s.controllerUUID,
		s.modelUUID,
		nil,
		ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		loggertesting.WrapCheckLog(c),
		s.clock,
		Services{
			AgentBinaryService:      s.agentBinaryService,
			AgentPasswordService:    s.agentPasswordService,
			BlockCommandService:     s.blockCommandService,
			CloudService:            s.cloudService,
			ControllerConfigService: s.controllerConfigService,
			ControllerNodeService:   s.controllerNodeService,
			KeyUpdaterService:       s.keyUpdaterService,
			MachineService:          s.machineService,
			StatusService:           s.statusService,
			ModelConfigService:      s.modelConfigService,
		},
	)

	c.Cleanup(func() {
		s.blockCommandService = nil
		s.cloudService = nil
		s.controllerConfigService = nil
		s.controllerNodeService = nil
		s.keyUpdaterService = nil
		s.machineService = nil
		s.modelConfigService = nil
		s.api = nil
	})
	return ctrl
}

func (s *ProvisioningMachineManagerSuite) expectProvisioningMachine(arch *string) {
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return("deadbeef", nil)
	s.machineService.EXPECT().GetHardwareCharacteristics(gomock.Any(), coremachine.UUID("deadbeef")).Return(&instance.HardwareCharacteristics{Arch: arch}, nil)
	s.machineService.EXPECT().GetMachineBase(gomock.Any(), coremachine.Name("0")).Return(corebase.MustParseBaseFromString("ubuntu@20.04/stable"), nil)
	if arch != nil {
		s.agentPasswordService.EXPECT().SetMachinePassword(gomock.Any(), coremachine.Name("0"), gomock.Any()).Return(nil).AnyTimes()
	}
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScript(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        true,
		"enable-os-refresh-update": true,
	}))
	c.Assert(err, tc.ErrorIsNil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).Times(2)

	arch := "amd64"
	s.expectProvisioningMachine(&arch)

	metadata := []agentbinary.Metadata{{
		Version: "2.6.6",
		Arch:    arch,
		Size:    4,
		SHA256:  "sha256",
	}}
	s.agentBinaryService.EXPECT().ListAgentBinaries(gomock.Any()).Return(metadata, nil)

	addrs := []string{"0.2.4.6:1"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil).MinTimes(2)
	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(
		gomock.Any(), coremachine.Name("0"),
	).Return([]string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
	}, nil)

	result, err := s.api.ProvisioningScript(c.Context(), params.ProvisioningScriptParams{
		MachineId: "0",
		Nonce:     "nonce",
	})
	c.Assert(err, tc.ErrorIsNil)
	scriptLines := strings.Split(result.Script, "\n")
	provisioningScriptLines := strings.Split(result.Script, "\n")
	c.Assert(scriptLines, tc.HasLen, len(provisioningScriptLines))
	for i, line := range scriptLines {
		if strings.Contains(line, "oldpassword") {
			continue
		}
		c.Assert(line, tc.Equals, provisioningScriptLines[i])
	}
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScriptNoArch(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        false,
		"enable-os-refresh-update": false,
	}))
	c.Assert(err, tc.ErrorIsNil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return("deadbeef", nil)
	s.machineService.EXPECT().GetHardwareCharacteristics(gomock.Any(), coremachine.UUID("deadbeef")).Return(&instance.HardwareCharacteristics{}, nil)

	_, err = s.api.ProvisioningScript(c.Context(), params.ProvisioningScriptParams{
		MachineId: "0",
		Nonce:     "nonce",
	})
	c.Assert(err, tc.ErrorMatches, `getting instance config: arch is not set for "0"`)
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScriptDisablePackageCommands(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        false,
		"enable-os-refresh-update": false,
	}))
	c.Assert(err, tc.ErrorIsNil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).Times(2)

	arch := "amd64"
	s.expectProvisioningMachine(&arch)

	metadata := []agentbinary.Metadata{{
		Version: "2.6.6",
		Arch:    arch,
		Size:    4,
		SHA256:  "sha256",
	}}
	s.agentBinaryService.EXPECT().ListAgentBinaries(gomock.Any()).Return(metadata, nil)

	addrs := []string{"0.2.4.6:1"}
	s.controllerNodeService.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return(addrs, nil).MinTimes(2)

	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(
		gomock.Any(), coremachine.Name("0"),
	).Return([]string{}, nil)

	result, err := s.api.ProvisioningScript(c.Context(), params.ProvisioningScriptParams{
		MachineId: "0",
		Nonce:     "nonce",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Script, tc.Not(tc.Contains), "apt-get update")
	c.Assert(result.Script, tc.Not(tc.Contains), "apt-get upgrade")
}

func (s *ProvisioningMachineManagerSuite) TestRetryProvisioning(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	now := s.clock.Now()

	s.statusService.EXPECT().GetInstanceStatus(gomock.Any(), coremachine.Name("0")).Return(status.StatusInfo{Status: status.ProvisioningError}, nil)
	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status: status.ProvisioningError,
		Data:   map[string]interface{}{"transient": true},
		Since:  &now,
	}).Return(nil)

	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return([]coremachine.Name{"0", "1"}, nil)

	results, err := s.api.RetryProvisioning(c.Context(), params.RetryProvisioningArgs{
		Machines: []string{"machine-0"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{})
}

func (s *ProvisioningMachineManagerSuite) TestRetryProvisioningAll(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	now := s.clock.Now()

	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return([]coremachine.Name{"0", "1"}, nil)

	s.statusService.EXPECT().GetInstanceStatus(gomock.Any(), coremachine.Name("0")).Return(status.StatusInfo{Status: status.ProvisioningError}, nil)
	s.statusService.EXPECT().SetInstanceStatus(gomock.Any(), coremachine.Name("0"), status.StatusInfo{
		Status: status.ProvisioningError,
		Data:   map[string]interface{}{"transient": true},
		Since:  &now,
	}).Return(nil)

	s.statusService.EXPECT().GetInstanceStatus(gomock.Any(), coremachine.Name("1")).Return(status.StatusInfo{Status: status.Pending}, nil)

	results, err := s.api.RetryProvisioning(c.Context(), params.RetryProvisioningArgs{
		All: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{})
}
