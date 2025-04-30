// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"strings"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commonmocks "github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/environs/config"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	stateerrors "github.com/juju/juju/state/errors"
)

type AddMachineManagerSuite struct {
	authorizer    *apiservertesting.FakeAuthorizer
	model         model.ModelInfo
	st            *MockBackend
	storageAccess *MockStorageInterface
	pool          *MockPool
	api           *MachineManagerAPI
	store         *MockObjectStore
	cloudService  *commonmocks.MockCloudService

	controllerConfigService *MockControllerConfigService
	machineService          *MockMachineService
	applicationService      *MockApplicationService
	networkService          *MockNetworkService
	keyUpdaterService       *MockKeyUpdaterService
	blockCommandService     *MockBlockCommandService
	agentBinaryService      *MockAgentBinaryService
}

var _ = gc.Suite(&AddMachineManagerSuite{})

func (s *AddMachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.model = model.ModelInfo{
		UUID: modeltesting.GenModelUUID(c),
	}
}

func (s *AddMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.pool = NewMockPool(ctrl)

	s.st = NewMockBackend(ctrl)

	s.storageAccess = NewMockStorageInterface(ctrl)
	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.store = NewMockObjectStore(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)

	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound).AnyTimes()
	s.agentBinaryService = NewMockAgentBinaryService(ctrl)

	s.api = NewMachineManagerAPI(
		s.model,
		s.controllerConfigService,
		s.st,
		s.cloudService,
		s.machineService,
		s.applicationService,
		s.store,
		nil,
		s.storageAccess,
		s.pool,
		ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		common.NewResources(),
		nil,
		loggertesting.WrapCheckLog(c),
		s.networkService,
		s.keyUpdaterService,
		nil,
		s.blockCommandService,
		s.agentBinaryService,
	)

	return ctrl
}

func (s *AddMachineManagerSuite) TestAddMachines(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	apiParams := make([]params.AddMachineParams, 2)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Base: &params.Base{Name: "ubuntu", Channel: "22.04"},
			Jobs: []model.MachineJob{model.JobHostUnits},
		}
	}
	apiParams[0].Disks = []storage.Directive{{Size: 1, Count: 2}, {Size: 2, Count: 1}}
	apiParams[1].Disks = []storage.Directive{{Size: 1, Count: 2, Pool: "three"}}

	m1 := NewMockMachine(ctrl)
	m1.EXPECT().Id().Return("666").AnyTimes()
	m2 := NewMockMachine(ctrl)
	m2.EXPECT().Id().Return("667/lxd/1").AnyTimes()

	s.st.EXPECT().AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("22.04"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{
			{
				Volume:     state.VolumeParams{Pool: "", Size: 1},
				Attachment: state.VolumeAttachmentParams{ReadOnly: false},
			},
			{
				Volume:     state.VolumeParams{Pool: "", Size: 1},
				Attachment: state.VolumeAttachmentParams{ReadOnly: false},
			},
			{
				Volume:     state.VolumeParams{Pool: "", Size: 2},
				Attachment: state.VolumeAttachmentParams{ReadOnly: false},
			},
		},
	}).Return(m1, nil)
	s.machineService.EXPECT().CreateMachine(gomock.Any(), coremachine.Name("666"))
	s.machineService.EXPECT().CreateMachine(gomock.Any(), coremachine.Name("667/lxd/1"))
	s.machineService.EXPECT().CreateMachine(gomock.Any(), coremachine.Name("667"))
	s.st.EXPECT().AddOneMachine(state.MachineTemplate{
		Base: state.UbuntuBase("22.04"),
		Jobs: []state.MachineJob{state.JobHostUnits},
		Volumes: []state.HostVolumeParams{
			{
				Volume:     state.VolumeParams{Pool: "three", Size: 1},
				Attachment: state.VolumeAttachmentParams{ReadOnly: false},
			},
			{
				Volume:     state.VolumeParams{Pool: "three", Size: 1},
				Attachment: state.VolumeAttachmentParams{ReadOnly: false},
			},
		},
	}).Return(m2, nil)
	s.networkService.EXPECT().GetAllSpaces(gomock.Any())

	machines, err := s.api.AddMachines(context.Background(), params.AddMachines{MachineParams: apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines.Machines, gc.HasLen, 2)
}

func (s *AddMachineManagerSuite) TestAddMachinesStateError(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.EXPECT().AddOneMachine(gomock.Any()).Return(nil, errors.New("boom"))
	s.networkService.EXPECT().GetAllSpaces(gomock.Any())

	results, err := s.api.AddMachines(context.Background(), params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Base: &params.Base{Name: "ubuntu", Channel: "22.04"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.AddMachinesResults{
		Machines: []params.AddMachinesResult{{
			Error: &params.Error{Message: "boom", Code: ""},
		}},
	})
}

type DestroyMachineManagerSuite struct {
	testing.CleanupSuite
	authorizer    *apiservertesting.FakeAuthorizer
	st            *MockBackend
	storageAccess *MockStorageInterface
	leadership    *MockLeadership
	store         *MockObjectStore
	cloudService  *commonmocks.MockCloudService
	api           *MachineManagerAPI
	model         model.ModelInfo

	controllerConfigService *MockControllerConfigService
	machineService          *MockMachineService
	applicationService      *MockApplicationService
	networkService          *MockNetworkService
	keyUpdaterService       *MockKeyUpdaterService
	blockCommandService     *MockBlockCommandService
	agentBinaryService      *MockAgentBinaryService
}

var _ = gc.Suite(&DestroyMachineManagerSuite{})

func (s *DestroyMachineManagerSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.PatchValue(&ClassifyDetachedStorage, mockedClassifyDetachedStorage)
	s.model = model.ModelInfo{
		UUID: modeltesting.GenModelUUID(c),
	}
}

func (s *DestroyMachineManagerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockBackend(ctrl)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.store = NewMockObjectStore(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)

	s.storageAccess = NewMockStorageInterface(ctrl)
	s.storageAccess.EXPECT().VolumeAccess().Return(nil).AnyTimes()
	s.storageAccess.EXPECT().FilesystemAccess().Return(nil).AnyTimes()

	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.leadership = NewMockLeadership(ctrl)

	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound).AnyTimes()
	s.agentBinaryService = NewMockAgentBinaryService(ctrl)

	s.api = NewMachineManagerAPI(
		s.model,
		s.controllerConfigService,
		s.st,
		s.cloudService,
		s.machineService,
		s.applicationService,
		s.store,
		nil,
		s.storageAccess,
		nil,
		ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		nil,
		s.leadership,
		loggertesting.WrapCheckLog(c),
		s.networkService,
		s.keyUpdaterService,
		nil,
		s.blockCommandService,
		s.agentBinaryService,
	)

	return ctrl
}

func (s *DestroyMachineManagerSuite) expectUnpinAppLeaders(id string) {
	machineTag := names.NewMachineTag(id)

	s.leadership.EXPECT().GetMachineApplicationNames(gomock.Any(), id).Return([]string{"foo-app-1"}, nil)
	s.leadership.EXPECT().UnpinApplicationLeadersByName(gomock.Any(), machineTag, []string{"foo-app-1"}).Return(params.PinApplicationsResults{}, nil)
}

func (s *DestroyMachineManagerSuite) expectDestroyMachine(ctrl *gomock.Controller, machineName coremachine.Name, unitNames []coreunit.Name, containers []string, attemptDestroy, keep, force bool) *MockMachine {
	machine := NewMockMachine(ctrl)

	machine.EXPECT().Containers().Return(containers, nil)

	if unitNames == nil {
		unitNames = []coreunit.Name{"foo/0", "foo/1", "foo/2"}
		s.expectDestroyUnit(ctrl, "foo/0", true, nil)
		s.expectDestroyUnit(ctrl, "foo/1", false, nil)
		s.expectDestroyUnit(ctrl, "foo/2", false, nil)
	}

	s.applicationService.EXPECT().GetUnitNamesOnMachine(gomock.Any(), machineName).Return(unitNames, nil)

	if attemptDestroy {
		if force {
			machine.EXPECT().ForceDestroy(gomock.Any()).Return(nil)
		} else {
			if len(containers) > 0 {
				machine.EXPECT().Destroy(gomock.Any()).Return(stateerrors.NewHasContainersError(machineName.String(), containers))
			} else if len(unitNames) > 0 {
				names := transform.Slice(unitNames, func(u coreunit.Name) string { return u.String() })
				machine.EXPECT().Destroy(gomock.Any()).Return(stateerrors.NewHasAssignedUnitsError(machineName.String(), names))
			} else {
				machine.EXPECT().Destroy(gomock.Any()).Return(nil)
			}
		}
	}
	return machine
}

func (s *DestroyMachineManagerSuite) expectDestroyUnit(ctrl *gomock.Controller, name coreunit.Name, hasStorage bool, retrievalErr error) {
	unitTag := names.NewUnitTag(name.String())
	if retrievalErr != nil {
		s.storageAccess.EXPECT().UnitStorageAttachments(unitTag).Return(nil, retrievalErr)
	} else if !hasStorage {
		s.storageAccess.EXPECT().UnitStorageAttachments(unitTag).Return([]state.StorageAttachment{}, nil)
	} else {
		s.storageAccess.EXPECT().UnitStorageAttachments(unitTag).Return([]state.StorageAttachment{
			s.expectDestroyStorage(ctrl, "disks/0", true),
			s.expectDestroyStorage(ctrl, "disks/1", false),
		}, nil)
	}
}

func (s *DestroyMachineManagerSuite) expectDestroyStorage(ctrl *gomock.Controller, id string, detachable bool) *MockStorageAttachment {
	storageInstanceTag := names.NewStorageTag(id)
	storageAttachment := NewMockStorageAttachment(ctrl)
	storageAttachment.EXPECT().StorageInstance().Return(storageInstanceTag)

	storageInstance := NewMockStorageInstance(ctrl)
	storageInstance.EXPECT().StorageTag().Return(storageInstanceTag).AnyTimes()
	s.storageAccess.EXPECT().StorageInstance(storageInstanceTag).Return(storageInstance, nil)

	return storageAttachment
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineFailedAllStorageRetrieval(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectDestroyUnit(ctrl, "foo/0", false, errors.New("kaboom"))
	s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom"))
	s.expectDestroyUnit(ctrl, "foo/2", false, errors.New("kaboom"))

	machine0 := s.expectDestroyMachine(ctrl, "0", []coreunit.Name{"foo/0", "foo/1", "foo/2"}, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	noWait := 0 * time.Second
	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		MachineTags: []string{"machine-0"},
		MaxWait:     &noWait,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/0: kaboom\ngetting storage for unit foo/1: kaboom\ngetting storage for unit foo/2: kaboom")),
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineFailedSomeUnitStorageRetrieval(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectDestroyUnit(ctrl, "foo/0", false, nil)
	s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom"))
	s.expectDestroyUnit(ctrl, "foo/2", false, nil)

	machine0 := s.expectDestroyMachine(ctrl, "0", []coreunit.Name{"foo/0", "foo/1", "foo/2"}, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	noWait := 0 * time.Second
	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		MachineTags: []string{"machine-0"},
		MaxWait:     &noWait,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/1: kaboom")),
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineFailedSomeStorageRetrievalManyMachines(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("1")

	s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom"))
	machine0 := s.expectDestroyMachine(ctrl, "0", []coreunit.Name{"foo/1"}, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	machine1 := s.expectDestroyMachine(ctrl, "1", []coreunit.Name{}, nil, true, false, false)
	s.st.EXPECT().Machine("1").Return(machine1, nil)

	noWait := 0 * time.Second
	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		MachineTags: []string{"machine-0", "machine-1"},
		MaxWait:     &noWait,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{
			{Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/1: kaboom"))},
			{Info: &params.DestroyMachineInfo{
				MachineId: "1",
			}},
		},
	})
}

func (s *DestroyMachineManagerSuite) TestForceDestroyMachineFailedSomeStorageRetrievalManyMachines(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")
	s.expectUnpinAppLeaders("1")

	s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom"))
	machine0 := s.expectDestroyMachine(ctrl, "0", []coreunit.Name{"foo/1"}, nil, true, false, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.expectDestroyUnit(ctrl, "bar/0", true, nil)
	machine1 := s.expectDestroyMachine(ctrl, "1", []coreunit.Name{"bar/0"}, nil, true, false, true)
	s.st.EXPECT().Machine("1").Return(machine1, nil)

	noWait := 0 * time.Second
	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		Force:       true,
		MachineTags: []string{"machine-0", "machine-1"},
		MaxWait:     &noWait,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{
			{Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-1"},
				},
			}},
			{Info: &params.DestroyMachineInfo{
				MachineId: "1",
				DestroyedUnits: []params.Entity{
					{"unit-bar-0"},
				},
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-1"},
				},
			}},
		},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineDryRun(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	machine0 := s.expectDestroyMachine(ctrl, "0", nil, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		MachineTags: []string{"machine-0"},
		DryRun:      true,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-1"},
				},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithContainersDryRun(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	machine0 := s.expectDestroyMachine(ctrl, "0", nil, []string{"0/lxd/0"}, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)
	container0 := s.expectDestroyMachine(ctrl, "0/lxd/0", nil, nil, false, false, false)
	s.st.EXPECT().Machine("0/lxd/0").Return(container0, nil)

	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		MachineTags: []string{"machine-0"},
		DryRun:      true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-1"},
				},
				DestroyedContainers: []params.DestroyMachineResult{{
					Info: &params.DestroyMachineInfo{
						MachineId: "0/lxd/0",
						DestroyedUnits: []params.Entity{
							{"unit-foo-0"},
							{"unit-foo-1"},
							{"unit-foo-2"},
						},
						DetachedStorage: []params.Entity{
							{"storage-disks-0"},
						},
						DestroyedStorage: []params.Entity{
							{"storage-disks-1"},
						},
					},
				}},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithParamsNoWait(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	machine0 := s.expectDestroyMachine(ctrl, "0", nil, nil, true, true, true)
	s.machineService.EXPECT().SetKeepInstance(gomock.Any(), coremachine.Name("0"), true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	noWait := 0 * time.Second
	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		Keep:        true,
		Force:       true,
		MachineTags: []string{"machine-0"},
		MaxWait:     &noWait,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-1"},
				},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithParamsNilWait(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	machine0 := s.expectDestroyMachine(ctrl, "0", nil, nil, true, true, true)
	s.machineService.EXPECT().SetKeepInstance(gomock.Any(), coremachine.Name("0"), true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		Keep:        true,
		Force:       true,
		MachineTags: []string{"machine-0"},
		// This will use max wait of system default for delay between cleanup operations.
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-1"},
				},
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithContainers(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.leadership.EXPECT().GetMachineApplicationNames(gomock.Any(), "0").Return([]string{"foo-app-1"}, nil)

	machine0 := s.expectDestroyMachine(ctrl, "0", nil, []string{"0/lxd/0"}, true, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		Force:       false,
		MachineTags: []string{"machine-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Error: apiservererrors.ServerError(stateerrors.NewHasContainersError("0", []string{"0/lxd/0"})),
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithContainersWithForce(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	s.expectUnpinAppLeaders("0/lxd/0")

	machine0 := s.expectDestroyMachine(ctrl, "0", nil, []string{"0/lxd/0"}, true, false, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)
	container0 := s.expectDestroyMachine(ctrl, "0/lxd/0", nil, nil, true, false, true)
	s.st.EXPECT().Machine("0/lxd/0").Return(container0, nil)

	results, err := s.api.DestroyMachineWithParams(context.Background(), params.DestroyMachinesParams{
		Force:       true,
		MachineTags: []string{"machine-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
				DestroyedUnits: []params.Entity{
					{"unit-foo-0"},
					{"unit-foo-1"},
					{"unit-foo-2"},
				},
				DetachedStorage: []params.Entity{
					{"storage-disks-0"},
				},
				DestroyedStorage: []params.Entity{
					{"storage-disks-1"},
				},
				DestroyedContainers: []params.DestroyMachineResult{{
					Info: &params.DestroyMachineInfo{
						MachineId: "0/lxd/0",
						DestroyedUnits: []params.Entity{
							{"unit-foo-0"},
							{"unit-foo-1"},
							{"unit-foo-2"},
						},
						DetachedStorage: []params.Entity{
							{"storage-disks-0"},
						},
						DestroyedStorage: []params.Entity{
							{"storage-disks-1"},
						},
					},
				}},
			},
		}},
	})
}

// // Alternate placing storage instaces in detached, then destroyed
func mockedClassifyDetachedStorage(
	_ storagecommon.VolumeAccess,
	_ storagecommon.FilesystemAccess,
	storage []state.StorageInstance,
) ([]params.Entity, []params.Entity, error) {
	destroyed := make([]params.Entity, 0)
	detached := make([]params.Entity, 0)
	for i, stor := range storage {
		if i%2 == 0 {
			detached = append(detached, params.Entity{stor.StorageTag().String()})
		} else {
			destroyed = append(destroyed, params.Entity{stor.StorageTag().String()})
		}
	}
	return destroyed, detached, nil
}

type ProvisioningMachineManagerSuite struct {
	authorizer   *apiservertesting.FakeAuthorizer
	st           *MockBackend
	ctrlSt       *MockControllerBackend
	pool         *MockPool
	store        *MockObjectStore
	cloudService *commonmocks.MockCloudService
	api          *MachineManagerAPI
	model        model.ModelInfo

	controllerConfigService *MockControllerConfigService
	machineService          *MockMachineService
	applicationService      *MockApplicationService
	networkService          *MockNetworkService
	keyUpdaterService       *MockKeyUpdaterService
	modelConfigService      *MockModelConfigService
	bootstrapEnviron        *MockBootstrapEnviron
	blockCommandService     *MockBlockCommandService
	agentBinaryService      *MockAgentBinaryService
}

var _ = gc.Suite(&ProvisioningMachineManagerSuite{})

func (s *ProvisioningMachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
}

func (s *ProvisioningMachineManagerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.model = model.ModelInfo{
		UUID: modeltesting.GenModelUUID(c),
	}

	s.st = NewMockBackend(ctrl)

	s.ctrlSt = NewMockControllerBackend(ctrl)
	s.ctrlSt.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(coretesting.FakeControllerConfig(), nil).AnyTimes()
	s.machineService = NewMockMachineService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)

	s.pool = NewMockPool(ctrl)
	s.pool.EXPECT().SystemState().Return(s.ctrlSt, nil).AnyTimes()

	s.cloudService = commonmocks.NewMockCloudService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)
	s.modelConfigService = NewMockModelConfigService(ctrl)
	s.bootstrapEnviron = NewMockBootstrapEnviron(ctrl)
	s.store = NewMockObjectStore(ctrl)

	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound).AnyTimes()

	s.machineService.EXPECT().GetBootstrapEnviron(gomock.Any()).Return(s.bootstrapEnviron, nil).AnyTimes()
	s.agentBinaryService = NewMockAgentBinaryService(ctrl)

	s.api = NewMachineManagerAPI(
		s.model,
		s.controllerConfigService,
		s.st,
		s.cloudService,
		s.machineService,
		s.applicationService,
		s.store,
		nil,
		nil,
		s.pool,
		ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		common.NewResources(),
		nil,
		loggertesting.WrapCheckLog(c),
		s.networkService,
		s.keyUpdaterService,
		s.modelConfigService,
		s.blockCommandService,
		s.agentBinaryService,
	)
	return ctrl
}

func (s *ProvisioningMachineManagerSuite) expectProvisioningMachine(ctrl *gomock.Controller, arch *string) *MockMachine {
	machine := NewMockMachine(ctrl)
	machine.EXPECT().Base().Return(state.Base{OS: "ubuntu", Channel: "20.04/stable"}).AnyTimes()
	machine.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	s.machineService.EXPECT().GetMachineUUID(gomock.Any(), coremachine.Name("0")).Return("deadbeef", nil)
	s.machineService.EXPECT().HardwareCharacteristics(gomock.Any(), coremachine.UUID("deadbeef")).Return(&instance.HardwareCharacteristics{Arch: arch}, nil)
	if arch != nil {
		machine.EXPECT().SetPassword(gomock.Any()).Return(nil)
	}

	return machine
}

func (s *ProvisioningMachineManagerSuite) expectProvisioningStorageCloser(ctrl *gomock.Controller) *MockStorageCloser {
	storageCloser := NewMockStorageCloser(ctrl)
	storageCloser.EXPECT().AllMetadata().Return([]binarystorage.Metadata{{
		Version: "2.6.6-ubuntu-amd64",
	}}, nil)
	storageCloser.EXPECT().Close().Return(nil)

	return storageCloser
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScript(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        true,
		"enable-os-refresh-update": true,
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).Times(2)

	arch := "amd64"
	machine0 := s.expectProvisioningMachine(ctrl, &arch)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	storageCloser := s.expectProvisioningStorageCloser(ctrl)
	s.st.EXPECT().ToolsStorage(gomock.Any()).Return(storageCloser, nil)

	s.ctrlSt.EXPECT().APIHostPortsForAgents(gomock.Any()).Return([]network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}, nil).Times(2)
	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(
		gomock.Any(), coremachine.Name("0"),
	).Return([]string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
	}, nil)

	result, err := s.api.ProvisioningScript(context.Background(), params.ProvisioningScriptParams{
		MachineId: "0",
		Nonce:     "nonce",
	})
	c.Assert(err, jc.ErrorIsNil)
	scriptLines := strings.Split(result.Script, "\n")
	provisioningScriptLines := strings.Split(result.Script, "\n")
	c.Assert(scriptLines, gc.HasLen, len(provisioningScriptLines))
	for i, line := range scriptLines {
		if strings.Contains(line, "oldpassword") {
			continue
		}
		c.Assert(line, gc.Equals, provisioningScriptLines[i])
	}
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScriptNoArch(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        false,
		"enable-os-refresh-update": false,
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	machine0 := s.expectProvisioningMachine(ctrl, nil)
	s.st.EXPECT().Machine("0").Return(machine0, nil)
	_, err = s.api.ProvisioningScript(context.Background(), params.ProvisioningScriptParams{
		MachineId: "0",
		Nonce:     "nonce",
	})
	c.Assert(err, gc.ErrorMatches, `getting instance config: arch is not set for "machine-0"`)
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScriptDisablePackageCommands(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cfg, err := config.New(config.NoDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        false,
		"enable-os-refresh-update": false,
	}))
	c.Assert(err, jc.ErrorIsNil)

	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).Times(2)

	arch := "amd64"
	machine0 := s.expectProvisioningMachine(ctrl, &arch)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	storageCloser := s.expectProvisioningStorageCloser(ctrl)
	s.st.EXPECT().ToolsStorage(gomock.Any()).Return(storageCloser, nil)

	s.ctrlSt.EXPECT().APIHostPortsForAgents(gomock.Any()).Return([]network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}, nil).Times(2)

	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(
		gomock.Any(), coremachine.Name("0"),
	).Return([]string{}, nil)

	result, err := s.api.ProvisioningScript(context.Background(), params.ProvisioningScriptParams{
		MachineId: "0",
		Nonce:     "nonce",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Script, gc.Not(jc.Contains), "apt-get update")
	c.Assert(result.Script, gc.Not(jc.Contains), "apt-get upgrade")
}

type statusMatcher struct {
	c        *gc.C
	expected status.StatusInfo
}

func (m statusMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(status.StatusInfo)
	m.c.Assert(ok, jc.IsTrue)
	if !ok {
		return false
	}

	m.c.Assert(obtained.Since, gc.NotNil)
	obtained.Since = nil
	m.c.Assert(obtained, jc.DeepEquals, m.expected)
	return true
}

func (m statusMatcher) String() string {
	return "Match the status.StatusInfo value"
}

func (s *ProvisioningMachineManagerSuite) TestRetryProvisioning(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	machine0 := NewMockMachine(ctrl)
	machine0.EXPECT().Id().Return("0")
	machine0.EXPECT().InstanceStatus().Return(status.StatusInfo{Status: "provisioning error"}, nil)
	machine0.EXPECT().SetInstanceStatus(statusMatcher{c: c, expected: status.StatusInfo{
		Status: status.ProvisioningError,
		Data:   map[string]interface{}{"transient": true},
	}}).Return(nil)
	machine1 := NewMockMachine(ctrl)
	machine1.EXPECT().Id().Return("1")
	s.st.EXPECT().AllMachines().Return([]Machine{machine0, machine1}, nil)

	results, err := s.api.RetryProvisioning(context.Background(), params.RetryProvisioningArgs{
		Machines: []string{"machine-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{})
}

func (s *ProvisioningMachineManagerSuite) TestRetryProvisioningAll(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	machine0 := NewMockMachine(ctrl)
	machine0.EXPECT().InstanceStatus().Return(status.StatusInfo{Status: "provisioning error"}, nil)
	machine0.EXPECT().SetInstanceStatus(statusMatcher{c: c, expected: status.StatusInfo{
		Status: status.ProvisioningError,
		Data:   map[string]interface{}{"transient": true},
	}}).Return(nil)
	machine1 := NewMockMachine(ctrl)
	machine1.EXPECT().InstanceStatus().Return(status.StatusInfo{Status: "pending"}, nil)
	s.st.EXPECT().AllMachines().Return([]Machine{machine0, machine1}, nil)

	results, err := s.api.RetryProvisioning(context.Background(), params.RetryProvisioningArgs{
		All: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{})
}
