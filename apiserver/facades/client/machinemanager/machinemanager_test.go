// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/apiserver/facades/client/machinemanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/os"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	stateerrors "github.com/juju/juju/state/errors"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MachineManagerSuite{})

type MachineManagerSuite struct {
	authorizer  *apiservertesting.FakeAuthorizer
	callContext context.ProviderCallContext
}

func (s *MachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *MachineManagerSuite) TestNewMachineManagerAPINonClient(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := machinemanager.NewMachineManagerAPI(
		nil,
		nil,
		nil,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
			ModelTag:   names.ModelTag{},
		},
		s.callContext,
		common.NewResources(),
		nil,
		nil,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

var _ = gc.Suite(&AddMachineManagerSuite{})

type AddMachineManagerSuite struct {
	authorizer    *apiservertesting.FakeAuthorizer
	st            *mocks.MockBackend
	storageAccess *mocks.MockStorageInterface
	pool          *mocks.MockPool
	api           *machinemanager.MachineManagerAPI
	model         *mocks.MockModel

	callContext context.ProviderCallContext
}

func (s *AddMachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *AddMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.pool = mocks.NewMockPool(ctrl)
	s.model = mocks.NewMockModel(ctrl)

	s.st = mocks.NewMockBackend(ctrl)
	s.storageAccess = mocks.NewMockStorageInterface(ctrl)
	s.st.EXPECT().GetBlockForType(state.ChangeBlock).Return(nil, false, nil).AnyTimes()

	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st,
		s.storageAccess,
		s.pool,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		s.callContext,
		common.NewResources(),
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *AddMachineManagerSuite) TestAddMachines(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	apiParams := make([]params.AddMachineParams, 2)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Series: "trusty",
			Jobs:   []model.MachineJob{model.JobHostUnits},
		}
	}
	apiParams[0].Disks = []storage.Constraints{{Size: 1, Count: 2}, {Size: 2, Count: 1}}
	apiParams[1].Disks = []storage.Constraints{{Size: 1, Count: 2, Pool: "three"}}

	s.st.EXPECT().AddOneMachine(state.MachineTemplate{
		Series: "trusty",
		Jobs:   []state.MachineJob{state.JobHostUnits},
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
	}).Return(&state.Machine{}, nil)
	s.st.EXPECT().AddOneMachine(state.MachineTemplate{
		Series: "trusty",
		Jobs:   []state.MachineJob{state.JobHostUnits},
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
	}).Return(&state.Machine{}, nil)

	machines, err := s.api.AddMachines(params.AddMachines{MachineParams: apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines.Machines, gc.HasLen, 2)
}

func (s *AddMachineManagerSuite) TestAddMachinesStateError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.st.EXPECT().AddOneMachine(gomock.Any()).Return(&state.Machine{}, errors.New("boom"))

	results, err := s.api.AddMachines(params.AddMachines{
		MachineParams: []params.AddMachineParams{{
			Series: "trusty",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.AddMachinesResults{
		Machines: []params.AddMachinesResult{{
			Error: &params.Error{Message: "boom", Code: ""},
		}},
	})
}

var _ = gc.Suite(&DestroyMachineManagerSuite{})

type DestroyMachineManagerSuite struct {
	testing.CleanupSuite
	authorizer    *apiservertesting.FakeAuthorizer
	st            *mocks.MockBackend
	storageAccess *mocks.MockStorageInterface
	leadership    *mocks.MockLeadership
	api           *machinemanager.MachineManagerAPI
}

func (s *DestroyMachineManagerSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.PatchValue(&machinemanager.ClassifyDetachedStorage, mockedClassifyDetachedStorage)
}

func (s *DestroyMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockBackend(ctrl)
	s.st.EXPECT().GetBlockForType(state.RemoveBlock).Return(nil, false, nil).AnyTimes()
	s.st.EXPECT().GetBlockForType(state.ChangeBlock).Return(nil, false, nil).AnyTimes()

	s.storageAccess = mocks.NewMockStorageInterface(ctrl)
	s.storageAccess.EXPECT().VolumeAccess().Return(nil).AnyTimes()
	s.storageAccess.EXPECT().FilesystemAccess().Return(nil).AnyTimes()

	s.leadership = mocks.NewMockLeadership(ctrl)

	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st,
		s.storageAccess,
		nil,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		nil,
		nil,
		s.leadership,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *DestroyMachineManagerSuite) apiV4() machinemanager.MachineManagerAPIV4 {
	return machinemanager.MachineManagerAPIV4{
		MachineManagerAPIV5: &machinemanager.MachineManagerAPIV5{
			MachineManagerAPIV6: &machinemanager.MachineManagerAPIV6{
				MachineManagerAPIV7: &machinemanager.MachineManagerAPIV7{
					MachineManagerAPIV8: &machinemanager.MachineManagerAPIV8{
						MachineManagerAPIV9: &machinemanager.MachineManagerAPIV9{
							MachineManagerAPI: s.api,
						},
					},
				},
			},
		},
	}
}

func (s *DestroyMachineManagerSuite) expectUnpinAppLeaders(id string) {
	machineTag := names.NewMachineTag(id)

	s.leadership.EXPECT().GetMachineApplicationNames(id).Return([]string{"foo-app-1"}, nil)
	s.leadership.EXPECT().UnpinApplicationLeadersByName(machineTag, []string{"foo-app-1"}).Return(params.PinApplicationsResults{}, nil)
}

func (s *DestroyMachineManagerSuite) expectDestroyMachine(ctrl *gomock.Controller, units []machinemanager.Unit, containers []string, attemptDestroy, keep, force bool) *mocks.MockMachine {
	machine := mocks.NewMockMachine(ctrl)
	if keep {
		machine.EXPECT().SetKeepInstance(true).Return(nil)
	}

	machine.EXPECT().Containers().Return(containers, nil)

	if units == nil {
		units = []machinemanager.Unit{
			s.expectDestroyUnit(ctrl, "foo/0", true, nil),
			s.expectDestroyUnit(ctrl, "foo/1", false, nil),
			s.expectDestroyUnit(ctrl, "foo/2", false, nil),
		}
	}
	machine.EXPECT().Units().Return(units, nil)

	if attemptDestroy {
		if force {
			machine.EXPECT().ForceDestroy(gomock.Any()).Return(nil)
		} else {
			if len(containers) > 0 {
				machine.EXPECT().Destroy().Return(stateerrors.NewHasContainersError("0", containers))
			} else if len(units) > 0 {
				machine.EXPECT().Destroy().Return(stateerrors.NewHasAssignedUnitsError("0", []string{"foo/0", "foo/1", "foo/2"}))
			} else {
				machine.EXPECT().Destroy().Return(nil)
			}
		}
	}
	return machine
}

func (s *DestroyMachineManagerSuite) expectDestroyUnit(ctrl *gomock.Controller, name string, hasStorage bool, retrievalErr error) *mocks.MockUnit {
	unitTag := names.NewUnitTag(name)
	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(unitTag).AnyTimes()
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
	return unit
}

func (s *DestroyMachineManagerSuite) expectDestroyStorage(ctrl *gomock.Controller, id string, detachable bool) *mocks.MockStorageAttachment {
	storageInstanceTag := names.NewStorageTag(id)
	storageAttachment := mocks.NewMockStorageAttachment(ctrl)
	storageAttachment.EXPECT().StorageInstance().Return(storageInstanceTag)

	storageInstance := mocks.NewMockStorageInstance(ctrl)
	storageInstance.EXPECT().StorageTag().Return(storageInstanceTag).AnyTimes()
	s.storageAccess.EXPECT().StorageInstance(storageInstanceTag).Return(storageInstance, nil)

	return storageAttachment
}

func (s *DestroyMachineManagerSuite) TestDestroyMachine(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	machine0 := s.expectDestroyMachine(ctrl, []machinemanager.Unit{}, nil, true, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachine(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
				MachineId: "0",
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithUnits(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.leadership.EXPECT().GetMachineApplicationNames("0").Return([]string{"foo-app-1"}, nil)

	machine0 := s.expectDestroyMachine(ctrl, nil, nil, true, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachine(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Error: apiservererrors.ServerError(stateerrors.NewHasAssignedUnitsError("0", []string{"foo/0", "foo/1", "foo/2"})),
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestForceDestroyMachine(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	machine0 := s.expectDestroyMachine(ctrl, nil, nil, true, false, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.ForceDestroyMachine(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
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

func (s *DestroyMachineManagerSuite) TestDestroyMachineFailedAllStorageRetrieval(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	units := []machinemanager.Unit{
		s.expectDestroyUnit(ctrl, "foo/0", false, errors.New("kaboom")),
		s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom")),
		s.expectDestroyUnit(ctrl, "foo/2", false, errors.New("kaboom")),
	}
	machine0 := s.expectDestroyMachine(ctrl, units, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachine(params.Entities{[]params.Entity{{Tag: "machine-0"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/0: kaboom\ngetting storage for unit foo/1: kaboom\ngetting storage for unit foo/2: kaboom")),
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineFailedSomeUnitStorageRetrieval(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	units := []machinemanager.Unit{
		s.expectDestroyUnit(ctrl, "foo/0", false, nil),
		s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom")),
		s.expectDestroyUnit(ctrl, "foo/2", false, nil),
	}
	machine0 := s.expectDestroyMachine(ctrl, units, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachine(params.Entities{[]params.Entity{{Tag: "machine-0"}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/1: kaboom")),
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineFailedSomeStorageRetrievalManyMachines(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("1")

	units0 := []machinemanager.Unit{
		s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom")),
	}
	machine0 := s.expectDestroyMachine(ctrl, units0, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	units1 := []machinemanager.Unit{}
	machine1 := s.expectDestroyMachine(ctrl, units1, nil, true, false, false)
	s.st.EXPECT().Machine("1").Return(machine1, nil)

	results, err := s.api.DestroyMachine(params.Entities{[]params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
	}})
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
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")
	s.expectUnpinAppLeaders("1")

	units0 := []machinemanager.Unit{
		s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom")),
	}
	machine0 := s.expectDestroyMachine(ctrl, units0, nil, true, false, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	units1 := []machinemanager.Unit{
		s.expectDestroyUnit(ctrl, "bar/0", true, nil),
	}
	machine1 := s.expectDestroyMachine(ctrl, units1, nil, true, false, true)
	s.st.EXPECT().Machine("1").Return(machine1, nil)

	results, err := s.api.ForceDestroyMachine(params.Entities{[]params.Entity{
		{Tag: "machine-0"},
		{Tag: "machine-1"},
	}})
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

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithParamsV4(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	machine0 := s.expectDestroyMachine(ctrl, nil, nil, true, true, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	apiV4 := s.apiV4()
	results, err := apiV4.DestroyMachineWithParams(params.DestroyMachinesParams{
		Keep:        true,
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
			},
		}},
	})
}

func (s *DestroyMachineManagerSuite) TestDestroyMachineWithParamsNoWait(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	machine0 := s.expectDestroyMachine(ctrl, nil, nil, true, true, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	noWait := 0 * time.Second
	results, err := s.api.DestroyMachineWithParams(params.DestroyMachinesParams{
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
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	machine0 := s.expectDestroyMachine(ctrl, nil, nil, true, true, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachineWithParams(params.DestroyMachinesParams{
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
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.leadership.EXPECT().GetMachineApplicationNames("0").Return([]string{"foo-app-1"}, nil)

	machine0 := s.expectDestroyMachine(ctrl, nil, []string{"0/lxd/0"}, true, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	results, err := s.api.DestroyMachineWithParams(params.DestroyMachinesParams{
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
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUnpinAppLeaders("0")

	s.expectUnpinAppLeaders("0/lxd/0")

	machine0 := s.expectDestroyMachine(ctrl, nil, []string{"0/lxd/0"}, true, false, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil)
	container0 := s.expectDestroyMachine(ctrl, nil, nil, true, false, true)
	s.st.EXPECT().Machine("0/lxd/0").Return(container0, nil)

	results, err := s.api.DestroyMachineWithParams(params.DestroyMachinesParams{
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

// Alternate placing storage instaces in detached, then destroyed
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

var _ = gc.Suite(&ProvisioningMachineManagerSuite{})

type ProvisioningMachineManagerSuite struct {
	authorizer *apiservertesting.FakeAuthorizer
	st         *mocks.MockBackend
	ctrlSt     *mocks.MockControllerBackend
	pool       *mocks.MockPool
	model      *mocks.MockModel
	api        *machinemanager.MachineManagerAPI

	callContext context.ProviderCallContext
}

func (s *ProvisioningMachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *ProvisioningMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockBackend(ctrl)

	s.ctrlSt = mocks.NewMockControllerBackend(ctrl)
	s.ctrlSt.EXPECT().ControllerConfig().Return(coretesting.FakeControllerConfig(), nil).AnyTimes()
	s.ctrlSt.EXPECT().ControllerTag().Return(coretesting.ControllerTag).AnyTimes()

	s.pool = mocks.NewMockPool(ctrl)
	s.pool.EXPECT().SystemState().Return(s.ctrlSt, nil).AnyTimes()

	s.model = mocks.NewMockModel(ctrl)
	s.model.EXPECT().UUID().Return("uuid").AnyTimes()
	s.model.EXPECT().ModelTag().Return(coretesting.ModelTag).AnyTimes()
	s.st.EXPECT().Model().Return(s.model, nil).AnyTimes()

	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st,
		nil,
		s.pool,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		s.callContext,
		common.NewResources(),
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *ProvisioningMachineManagerSuite) expectProvisioningMachine(ctrl *gomock.Controller, arch *string) *mocks.MockMachine {
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Series().Return("focal").AnyTimes()
	machine.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	machine.EXPECT().HardwareCharacteristics().Return(&instance.HardwareCharacteristics{Arch: arch}, nil)
	if arch != nil {
		machine.EXPECT().SetPassword(gomock.Any()).Return(nil)
	}

	return machine
}

func (s *ProvisioningMachineManagerSuite) expectProvisioningStorageCloser(ctrl *gomock.Controller) *mocks.MockStorageCloser {
	storageCloser := mocks.NewMockStorageCloser(ctrl)
	storageCloser.EXPECT().AllMetadata().Return([]binarystorage.Metadata{{
		Version: "2.6.6-ubuntu-amd64",
	}}, nil)
	storageCloser.EXPECT().Close().Return(nil)

	return storageCloser
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScript(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        true,
		"enable-os-refresh-update": true,
	}))).Times(2)

	arch := "amd64"
	machine0 := s.expectProvisioningMachine(ctrl, &arch)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	storageCloser := s.expectProvisioningStorageCloser(ctrl)
	s.st.EXPECT().ToolsStorage().Return(storageCloser, nil)

	s.ctrlSt.EXPECT().APIHostPortsForAgents().Return([]network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}, nil).Times(2)

	result, err := s.api.ProvisioningScript(params.ProvisioningScriptParams{
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
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        false,
		"enable-os-refresh-update": false,
	})))

	machine0 := s.expectProvisioningMachine(ctrl, nil)
	s.st.EXPECT().Machine("0").Return(machine0, nil)
	_, err := s.api.ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: "0",
		Nonce:     "nonce",
	})
	c.Assert(err, gc.ErrorMatches, `getting instance config: arch is not set for "machine-0"`)
}

func (s *ProvisioningMachineManagerSuite) TestProvisioningScriptDisablePackageCommands(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        false,
		"enable-os-refresh-update": false,
	}))).Times(2)

	arch := "amd64"
	machine0 := s.expectProvisioningMachine(ctrl, &arch)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	storageCloser := s.expectProvisioningStorageCloser(ctrl)
	s.st.EXPECT().ToolsStorage().Return(storageCloser, nil)

	s.ctrlSt.EXPECT().APIHostPortsForAgents().Return([]network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}, nil).Times(2)

	result, err := s.api.ProvisioningScript(params.ProvisioningScriptParams{
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
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.st.EXPECT().GetBlockForType(state.ChangeBlock).Return(nil, false, nil).AnyTimes()

	machine0 := mocks.NewMockMachine(ctrl)
	machine0.EXPECT().Id().Return("0")
	machine0.EXPECT().InstanceStatus().Return(status.StatusInfo{Status: "provisioning error"}, nil)
	machine0.EXPECT().SetInstanceStatus(statusMatcher{c: c, expected: status.StatusInfo{
		Status: status.ProvisioningError,
		Data:   map[string]interface{}{"transient": true},
	}}).Return(nil)
	machine1 := mocks.NewMockMachine(ctrl)
	machine1.EXPECT().Id().Return("1")
	s.st.EXPECT().AllMachines().Return([]machinemanager.Machine{machine0, machine1}, nil)

	results, err := s.api.RetryProvisioning(params.RetryProvisioningArgs{
		Machines: []string{"machine-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{})
}

func (s *ProvisioningMachineManagerSuite) TestRetryProvisioningAll(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.st.EXPECT().GetBlockForType(state.ChangeBlock).Return(nil, false, nil).AnyTimes()

	machine0 := mocks.NewMockMachine(ctrl)
	machine0.EXPECT().InstanceStatus().Return(status.StatusInfo{Status: "provisioning error"}, nil)
	machine0.EXPECT().SetInstanceStatus(statusMatcher{c: c, expected: status.StatusInfo{
		Status: status.ProvisioningError,
		Data:   map[string]interface{}{"transient": true},
	}}).Return(nil)
	machine1 := mocks.NewMockMachine(ctrl)
	machine1.EXPECT().InstanceStatus().Return(status.StatusInfo{Status: "pending"}, nil)
	s.st.EXPECT().AllMachines().Return([]machinemanager.Machine{machine0, machine1}, nil)

	results, err := s.api.RetryProvisioning(params.RetryProvisioningArgs{
		All: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{})
}

type UpgradeSeriesMachineManagerSuite struct{}

func (s *UpgradeSeriesMachineManagerSuite) expectValidateMachine(ctrl *gomock.Controller, series string, isManager, isLockedForSeriesUpgrade bool) *mocks.MockMachine {
	machine := mocks.NewMockMachine(ctrl)
	machine.EXPECT().Tag().Return(names.NewMachineTag("0")).AnyTimes()
	machine.EXPECT().Series().Return(series).AnyTimes()
	machine.EXPECT().Id().Return("0").AnyTimes()

	machine.EXPECT().IsManager().Return(isManager)
	if isManager {
		return machine
	}
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(isLockedForSeriesUpgrade, nil)
	if isLockedForSeriesUpgrade {
		machine.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesNotStarted, nil)
		return machine
	}

	return machine
}

func (s *UpgradeSeriesMachineManagerSuite) expectValidateApplicationOnMachine(ctrl *gomock.Controller) *mocks.MockApplication {
	app := mocks.NewMockApplication(ctrl)
	ch := mocks.NewMockCharm(ctrl)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()
	ch.EXPECT().Meta().Return(&charm.Meta{Series: []string{"xenial"}}).AnyTimes()
	app.EXPECT().Charm().Return(ch, true, nil)
	app.EXPECT().CharmOrigin().Return(&state.CharmOrigin{})

	return app
}

var _ = gc.Suite(&UpgradeSeriesValidateMachineManagerSuite{})

type UpgradeSeriesValidateMachineManagerSuite struct {
	*UpgradeSeriesMachineManagerSuite
	authorizer *apiservertesting.FakeAuthorizer
	st         *mocks.MockBackend
	api        *machinemanager.MachineManagerAPI

	callContext context.ProviderCallContext
}

func (s *UpgradeSeriesValidateMachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *UpgradeSeriesValidateMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockBackend(ctrl)

	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st,
		nil,
		nil,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		s.callContext,
		common.NewResources(),
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *UpgradeSeriesValidateMachineManagerSuite) expectValidateUnit(ctrl *gomock.Controller, unitName string, unitAgentState, unitState status.Status) *mocks.MockUnit {
	unitTag := names.NewUnitTag(unitName)
	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().Name().Return(unitTag.String()).AnyTimes()
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: unitAgentState}, nil)
	if unitAgentState != status.Executing && unitAgentState != status.Error {
		unit.EXPECT().Status().Return(status.StatusInfo{Status: unitState}, nil)
		if unitState != status.Executing && unitState != status.Error {
			unit.EXPECT().UnitTag().Return(unitTag)
		}
	}
	return unit
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateOK(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	machine0.EXPECT().ApplicationNames().Return([]string{"foo"}, nil)
	app := s.expectValidateApplicationOnMachine(ctrl)
	s.st.EXPECT().Application("foo").Return(app, nil)

	machine0.EXPECT().Units().Return([]machinemanager.Unit{
		s.expectValidateUnit(ctrl, "foo/0", status.Idle, status.Idle),
		s.expectValidateUnit(ctrl, "foo/1", status.Idle, status.Idle),
		s.expectValidateUnit(ctrl, "foo/2", status.Idle, status.Idle),
	}, nil)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
		}},
	}
	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)

	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.UnitNames, gc.DeepEquals, []string{"foo/0", "foo/1", "foo/2"})
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateIsControllerError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", true, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
		}},
	}
	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"machine-0 is a controller and cannot be targeted for series upgrade")
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateIsLockedForSeriesUpgradeError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, true)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
		}},
	}
	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		`upgrade series lock found for "0"; series upgrade is in the "not started" state`)
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateNoSeriesError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
		}},
	}
	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results[0].Error, gc.ErrorMatches, "series missing from args")
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateNotFromUbuntuError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "centos7", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "bionic",
		}},
	}

	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"machine-0 is running CentOS and is not valid for Ubuntu series upgrade")
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateNotToUbuntuError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "centos7",
		}},
	}

	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		`series "centos7" is from OS "CentOS" and is not a valid upgrade target`)
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateAlreadyRunningSeriesError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "trusty",
		}},
	}

	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "machine-0 is already running series trusty")
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateOlderSeriesError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "precise",
		}},
	}

	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"machine machine-0 is running trusty which is a newer series than precise.")
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateUnitNotIdleError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	machine0.EXPECT().ApplicationNames().Return([]string{"foo"}, nil)
	app := s.expectValidateApplicationOnMachine(ctrl)
	s.st.EXPECT().Application("foo").Return(app, nil)

	machine0.EXPECT().Units().Return([]machinemanager.Unit{
		s.expectValidateUnit(ctrl, "foo/0", status.Executing, status.Active),
		mocks.NewMockUnit(ctrl),
	}, nil)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
		}},
	}
	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"unit unit-foo-0 is not ready to start a series upgrade; its agent status is: \"executing\" ")
}

func (s *UpgradeSeriesValidateMachineManagerSuite) TestUpgradeSeriesValidateUnitStatusError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectValidateMachine(ctrl, "trusty", false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	machine0.EXPECT().ApplicationNames().Return([]string{"foo"}, nil)
	app := s.expectValidateApplicationOnMachine(ctrl)
	s.st.EXPECT().Application("foo").Return(app, nil)

	machine0.EXPECT().Units().Return([]machinemanager.Unit{
		s.expectValidateUnit(ctrl, "foo/0", status.Idle, status.Error),
		mocks.NewMockUnit(ctrl),
	}, nil)

	args := params.UpdateChannelArgs{
		Args: []params.UpdateChannelArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
		}},
	}
	results, err := s.api.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"unit unit-foo-[0-2] is not ready to start a series upgrade; its status is: \"error\" ")
}

var _ = gc.Suite(&UpgradeSeriesPrepareMachineManagerSuite{})

type UpgradeSeriesPrepareMachineManagerSuite struct {
	*UpgradeSeriesMachineManagerSuite
	authorizer *apiservertesting.FakeAuthorizer
	st         *mocks.MockBackend
	api        *machinemanager.MachineManagerAPI

	callContext context.ProviderCallContext
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TearDownTest(c *gc.C) {
	s.authorizer = nil
	s.st = nil
	s.api = nil
	s.callContext = nil
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockBackend(ctrl)
	s.st.EXPECT().GetBlockForType(state.ChangeBlock).Return(nil, false, nil).AnyTimes()

	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st,
		nil,
		nil,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		s.callContext,
		common.NewResources(),
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) expectPrepareMachine(ctrl *gomock.Controller, upgradeSeriesErr error) *mocks.MockMachine {
	machine := s.expectValidateMachine(ctrl, "trusty", false, false)

	machine.EXPECT().Units().Return([]machinemanager.Unit{
		s.expectPrepareUnit(ctrl, "foo/0"),
		s.expectPrepareUnit(ctrl, "foo/1"),
		s.expectPrepareUnit(ctrl, "foo/2"),
	}, nil)

	machine.EXPECT().CreateUpgradeSeriesLock([]string{"foo/0", "foo/1", "foo/2"}, "xenial")

	machine.EXPECT().ApplicationNames().Return([]string{"foo"}, nil)
	app := s.expectValidateApplicationOnMachine(ctrl)
	s.st.EXPECT().Application("foo").Return(app, nil)

	machine.EXPECT().SetUpgradeSeriesStatus(
		model.UpgradeSeriesPrepareStarted,
		"started upgrade series from \"trusty\" to \"xenial\"",
	).Return(upgradeSeriesErr)

	if upgradeSeriesErr != nil {
		machine.EXPECT().RemoveUpgradeSeriesLock().Return(nil)
	}

	return machine
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) expectPrepareUnit(ctrl *gomock.Controller, unitName string) *mocks.MockUnit {
	unit := mocks.NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag(unitName))

	return unit
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPrepare(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectPrepareMachine(ctrl, nil)
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	machineTag := names.NewMachineTag("0")
	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateChannelArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "xenial",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPrepareMachineNotFound(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.st.EXPECT().Machine("76").Return(nil, errors.NotFoundf("machine 76"))

	machineTag := names.NewMachineTag("76")
	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateChannelArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "trusty",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "machine 76 not found")
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPrepareNotMachineTag(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	unitTag := names.NewUnitTag("mysql/0")
	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateChannelArg{
			Entity: params.Entity{
				Tag: unitTag.String()},
			Series: "trusty",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "\"unit-mysql-0\" is not a valid machine tag")
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	mm, err := machinemanager.NewMachineManagerAPI(s.st,
		nil,
		nil,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		s.callContext,
		common.NewResources(),
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = mm
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPreparePermissionDenied(c *gc.C) {
	user := names.NewUserTag("fred")
	s.setAPIUser(c, user)
	machineTag := names.NewMachineTag("0")
	_, err := s.api.UpgradeSeriesPrepare(
		params.UpdateChannelArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "xenial",
		},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPrepareNoSeries(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.st.EXPECT().Machine("0").Return(nil, nil).Times(1)

	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateChannelArg{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeBadRequest,
			Message: `series missing from args`,
		},
	})
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPrepareIncompatibleSeries(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := s.expectPrepareMachine(ctrl, apiservererrors.NewErrIncompatibleSeries([]string{"yakkety", "zesty"}, "xenial", "TestCharm"))
	s.st.EXPECT().Machine("0").Return(machine0, nil).Times(2)

	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateChannelArg{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
			Force:  false,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeIncompatibleSeries,
			Message: "series \"xenial\" not supported by charm \"TestCharm\", supported series are: yakkety, zesty",
		},
	})
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPrepareRemoveLockAfterFail(c *gc.C) {
	// TODO managed upgrade series
}

var _ = gc.Suite(&UpgradeSeriesCompleteMachineManagerSuite{})

type UpgradeSeriesCompleteMachineManagerSuite struct {
	authorizer *apiservertesting.FakeAuthorizer
	st         *mocks.MockBackend
	api        *machinemanager.MachineManagerAPI

	callContext context.ProviderCallContext
}

func (s *UpgradeSeriesCompleteMachineManagerSuite) SetUpTest(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.callContext = context.NewEmptyCloudCallContext()
}

func (s *UpgradeSeriesCompleteMachineManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = mocks.NewMockBackend(ctrl)
	s.st.EXPECT().GetBlockForType(state.ChangeBlock).Return(nil, false, nil).AnyTimes()
	s.st.EXPECT().GetBlockForType(state.ChangeBlock).Return(nil, false, nil).AnyTimes()

	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st,
		nil,
		nil,
		machinemanager.ModelAuthorizer{
			Authorizer: s.authorizer,
		},
		s.callContext,
		common.NewResources(),
		nil,
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *UpgradeSeriesCompleteMachineManagerSuite) TestUpgradeSeriesComplete(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	machine0 := mocks.NewMockMachine(ctrl)
	machine0.EXPECT().CompleteUpgradeSeries().Return(nil)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	_, err := s.api.UpgradeSeriesComplete(
		params.UpdateChannelArg{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&IsSeriesLessThanMachineManagerSuite{})

type IsSeriesLessThanMachineManagerSuite struct{}

// TestIsSeriesLessThan tests a validation method which is not very complicated
// but complex enough to warrant being exported from an export test package for
// testing.
func (s *IsSeriesLessThanMachineManagerSuite) TestIsSeriesLessThan(c *gc.C) {
	workloadSeries, err := series.AllWorkloadSeries("", "")
	c.Assert(err, jc.ErrorIsNil)
	ss := workloadSeries.Values()

	// Group series by OS and check the list for
	// each OS separately.
	seriesByOS := make(map[os.OSType][]string)
	for _, ser := range ss {
		seriesOS, err := series.GetOSFromSeries(ser)
		c.Assert(err, jc.ErrorIsNil)
		seriesList := seriesByOS[seriesOS]
		seriesList = append(seriesList, ser)
		seriesByOS[seriesOS] = seriesList
	}

	for seriesOS, seriesList := range seriesByOS {
		c.Logf("checking series for %v", seriesOS)
		s.assertSeriesLessThan(c, seriesList)
	}
}

func (s *IsSeriesLessThanMachineManagerSuite) assertSeriesLessThan(c *gc.C, seriesList []string) {
	// get the series versions
	vs := make([]string, 0, len(seriesList))
	for _, ser := range seriesList {
		ver, err := series.SeriesVersion(ser)
		c.Assert(err, jc.ErrorIsNil)
		vs = append(vs, ver)
	}

	// sort the values, so the lexicographical order is determined
	sort.Slice(vs, func(i, j int) bool {
		v1 := vs[i]
		v2 := vs[j]
		v1Int, err1 := strconv.Atoi(v1)
		v2Int, err2 := strconv.Atoi(v2)
		if err1 == nil && err2 == nil {
			return v1Int < v2Int
		}
		return v1 < v2
	})

	// check that the IsSeriesLessThan works for all supported series
	for i := range vs {

		// We need both the series and the next series in the list. So
		// we provide a check here to prevent going out of bounds.
		if i+1 > len(vs)-1 {
			break
		}

		// get the series for the specified version
		s1, err := series.VersionSeries(vs[i])
		c.Assert(err, jc.ErrorIsNil)
		s2, err := series.VersionSeries(vs[i+1])
		c.Assert(err, jc.ErrorIsNil)

		isLessThan, err := machinemanager.IsSeriesLessThan(s1, s2)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(isLessThan, jc.IsTrue, gc.Commentf("%q < %q", s1, s2))
	}
}
