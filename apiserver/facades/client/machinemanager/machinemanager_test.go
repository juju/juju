// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
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
	defer s.setup(c).Finish()

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
	defer s.setup(c).Finish()

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

func (s *MachineManagerSuite) assertMachinesDestroyed(c *gc.C, in []string, out params.DestroyMachineResults, expectedCalls ...string) {
	results, err := s.api.DestroyMachineWithParams(params.DestroyMachinesParams{
		MachineTags: in,
	})
	c.Assert(err, jc.ErrorIsNil)

	s.st.CheckCallNames(c, expectedCalls...)
	c.Assert(results, jc.DeepEquals, out)

}

func (s *DestroyMachineManagerSuite) TestDestroyMachineFailedSomeUnitStorageRetrieval(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	units := []machinemanager.Unit{
		s.expectDestroyUnit(ctrl, "foo/0", false, nil),
		s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom")),
		s.expectDestroyUnit(ctrl, "foo/2", false, nil),
	}
	s.assertMachinesDestroyed(c,
		[]string{"machine-0"},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/0: kaboom\ngetting storage for unit foo/1: kaboom\ngetting storage for unit foo/2: kaboom")),
			}},
		},
		"GetBlockForType",
		"GetBlockForType",
		"Machine",
		"UnitStorageAttachments",
		"UnitStorageAttachments",
		"UnitStorageAttachments",
	)
}

func (s *MachineManagerSuite) TestDestroyMachineFailedAllStorageClassification(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.machines["0"] = &mockMachine{}
	s.st.SetErrors(
		errors.New("boom"),
	)
	s.assertMachinesDestroyed(c,
		[]string{"machine-0"},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Error: apiservererrors.ServerError(errors.New("classifying storage for destruction for unit foo/0: boom")),
			}},
		},
		"GetBlockForType",
		"GetBlockForType",
		"Machine",
		"UnitStorageAttachments",
		"StorageInstance",
		"StorageInstance",
		"VolumeAccess",
		"FilesystemAccess",
		"StorageInstanceVolume",
		"UnitStorageAttachments",
		"VolumeAccess",
		"FilesystemAccess",
		"UnitStorageAttachments",
		"VolumeAccess",
		"FilesystemAccess",
	)
}

func (s *MachineManagerSuite) TestDestroyMachineFailedSomeUnitStorageRetrieval(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.machines["0"] = &mockMachine{}
	s.st.unitStorageAttachmentsF = func(tag names.UnitTag) ([]state.StorageAttachment, error) {
		if tag.Id() == "foo/1" {
			return nil, errors.New("kaboom")
		}
		return nil, nil
	}

	s.assertMachinesDestroyed(c,
		[]string{"machine-0"},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/1: kaboom")),
			}},
		},
		"GetBlockForType",
		"GetBlockForType",
		"Machine",
		"UnitStorageAttachments",
		"VolumeAccess",
		"FilesystemAccess",
		"UnitStorageAttachments",
		"UnitStorageAttachments",
		"VolumeAccess",
		"FilesystemAccess",
	)
}

func (s *MachineManagerSuite) TestDestroyMachineFailedSomeStorageRetrievalManyMachines(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectUnpinAppLeaders("1")

	units0 := []machinemanager.Unit{
		s.expectDestroyUnit(ctrl, "foo/1", false, errors.New("kaboom")),
	}
	machine0 := s.expectDestroyMachine(ctrl, units0, nil, false, false, false)
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.assertMachinesDestroyed(c,
		[]string{"machine-0", "machine-1"},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{
				{Error: apiservererrors.ServerError(errors.New("getting storage for unit foo/1: kaboom"))},
				{Info: &params.DestroyMachineInfo{
					MachineId: "1",
					DestroyedUnits: []params.Entity{
						{"unit-bar-0"},
					},
					DetachedStorage: []params.Entity{
						{"storage-disks-0"},
					},
				}},
			},
		},
		"GetBlockForType",
		"GetBlockForType",
		"Machine",
		"UnitStorageAttachments",
		"VolumeAccess",
		"FilesystemAccess",
		"UnitStorageAttachments",
		"UnitStorageAttachments",
		"VolumeAccess",
		"FilesystemAccess",
		"Machine",
		"UnitStorageAttachments",
		"StorageInstance",
		"VolumeAccess",
		"FilesystemAccess",
		"StorageInstanceVolume",
	)
}

func (s *MachineManagerSuite) assertDestroyMachineWithParams(c *gc.C, in params.DestroyMachinesParams, out params.DestroyMachineResults) {
	s.st.machines["0"] = &mockMachine{}
	results, err := s.api.DestroyMachineWithParams(in)
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.(*mockMachine).keep, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, out)
}

func (s *MachineManagerSuite) TestDestroyMachineWithParamsNoWait(c *gc.C) {
	defer s.setup(c).Finish()

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

func (s *MachineManagerSuite) TestUpgradeSeriesValidateOK(c *gc.C) {
	defer s.setup(c).Finish()

	s.model.EXPECT().Config().Return(config.New(config.UseDefaults, dummy.SampleConfig().Merge(coretesting.Attrs{
		"agent-version":            "2.6.6",
		"enable-os-upgrade":        false,
		"enable-os-refresh-update": false,
	}))).Times(2)

	machine0 := s.expectProvisioningMachine(ctrl)
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
	ch.EXPECT().Manifest().Return(&charm.Manifest{}).AnyTimes()
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	machine0.EXPECT().ApplicationNames().Return([]string{"foo"}, nil)
	app := s.expectValidateApplicationOnMachine(ctrl)
	s.st.EXPECT().Application("foo").Return(app, nil)

	machine0.EXPECT().Units().Return([]machinemanager.Unit{
		s.expectValidateUnit(ctrl, "foo/0", status.Idle, status.Idle),
		s.expectValidateUnit(ctrl, "foo/1", status.Idle, status.Idle),
		s.expectValidateUnit(ctrl, "foo/2", status.Idle, status.Idle),
	}, nil)

	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.setupUpgradeSeries(c)
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.setupUpgradeSeries(c)
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.setupUpgradeSeries(c)
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.setupUpgradeSeries(c)
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.setupUpgradeSeries(c)
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.setupUpgradeSeries(c)
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	s.setupUpgradeSeries(c)
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	machine0.EXPECT().ApplicationNames().Return([]string{"foo"}, nil)
	app := s.expectValidateApplicationOnMachine(ctrl)
	s.st.EXPECT().Application("foo").Return(app, nil)

	machine0.EXPECT().Units().Return([]machinemanager.Unit{
		s.expectValidateUnit(ctrl, "foo/0", status.Executing, status.Active),
		mocks.NewMockUnit(ctrl),
	}, nil)

	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	machine0.EXPECT().ApplicationNames().Return([]string{"foo"}, nil)
	app := s.expectValidateApplicationOnMachine(ctrl)
	s.st.EXPECT().Application("foo").Return(app, nil)

	machine0.EXPECT().Units().Return([]machinemanager.Unit{
		s.expectValidateUnit(ctrl, "foo/0", status.Idle, status.Error),
		mocks.NewMockUnit(ctrl),
	}, nil)

	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
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
	s.st.EXPECT().Machine("0").Return(machine0, nil)

	machineTag := names.NewMachineTag("0")
	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "xenial",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *UpgradeSeriesPrepareMachineManagerSuite) TestUpgradeSeriesPrepareMachineNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	machineTag := names.NewMachineTag("76")
	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "trusty",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "machine 76 not found")
}

func (s *MachineManagerSuite) TestUpgradeSeriesPrepareNotMachineTag(c *gc.C) {
	defer s.setup(c).Finish()

	unitTag := names.NewUnitTag("mysql/0")
	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
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
		params.UpdateSeriesArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "xenial",
		},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *MachineManagerSuite) TestUpgradeSeriesPrepareBlockedChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.st.blockMsg = "TestUpgradeSeriesPrepareBlockedChanges"
	s.st.block = state.ChangeBlock
	_, err := s.api.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
			Entity: params.Entity{
				Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
		},
	)
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), jc.DeepEquals, &params.Error{
		Message: "TestUpgradeSeriesPrepareBlockedChanges",
		Code:    "operation is blocked",
	})
}

func (s *MachineManagerSuite) TestUpgradeSeriesPrepareNoSeries(c *gc.C) {
	defer s.setup(c).Finish()

	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
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

	s.setupUpgradeSeries(c)
	s.st.machines["0"].SetErrors(apiservererrors.NewErrIncompatibleSeries([]string{"yakkety", "zesty"}, "xenial", "TestCharm"))
	result, err := s.api.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
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

	s.setupUpgradeSeries(c)
	_, err := s.api.UpgradeSeriesComplete(
		params.UpdateSeriesArg{
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
		c.Assert(isLessThan, jc.IsTrue)
	}
}

type mockState struct {
	jtesting.Stub
	machinemanager.Backend
	calls            int
	machineTemplates []state.MachineTemplate
	machines         map[string]*mockMachine
	applications     map[string]*mockApplication
	err              error
	blockMsg         string
	block            state.BlockType

	disableOSUpgrade bool
	disableOSRefresh bool

	unitStorageAttachmentsF func(tag names.UnitTag) ([]state.StorageAttachment, error)
}

func (st *mockState) ControllerTag() names.ControllerTag {
	return coretesting.ControllerTag
}

func (st *mockState) ControllerConfig() (controller.Config, error) {
	return coretesting.FakeControllerConfig(), nil
}

func (st *mockState) APIHostPortsForAgents() ([]network.SpaceHostPorts, error) {
	return []network.SpaceHostPorts{{{
		SpaceAddress: network.NewSpaceAddress("0.2.4.6", network.WithScope(network.ScopeCloudLocal)),
		NetPort:      1,
	}}}, nil
}

func (st *mockState) ToolsStorage() (binarystorage.StorageCloser, error) {
	return &mockToolsStorage{}, nil
}

type mockToolsStorage struct {
	binarystorage.StorageCloser
}

func (*mockToolsStorage) Close() error {
	return nil
}

func (*mockToolsStorage) AllMetadata() ([]binarystorage.Metadata, error) {
	return []binarystorage.Metadata{{
		Version: "2.6.6-ubuntu-amd64",
	}}, nil
}

type mockVolumeAccess struct {
	storagecommon.VolumeAccess
	*mockState
}

func (st *mockVolumeAccess) StorageInstanceVolume(tag names.StorageTag) (state.Volume, error) {
	st.MethodCall(st, "StorageInstanceVolume", tag)
	if err := st.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return &mockVolume{
		detachable: tag.Id() == "disks/0",
	}, nil
}

type mockFilesystemAccess struct {
	storagecommon.FilesystemAccess
	*mockState
}

func (st *mockFilesystemAccess) StorageInstanceFilesystem(tag names.StorageTag) (state.Filesystem, error) {
	st.MethodCall(st, "StorageInstanceFilesystem", tag)
	if err := st.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return nil, nil
}

func (st *mockState) VolumeAccess() storagecommon.VolumeAccess {
	st.MethodCall(st, "VolumeAccess")
	return &mockVolumeAccess{mockState: st}
}

func (st *mockState) FilesystemAccess() storagecommon.FilesystemAccess {
	st.MethodCall(st, "FilesystemAccess")
	return &mockFilesystemAccess{mockState: st}
}

func (st *mockState) AddOneMachine(template state.MachineTemplate) (*state.Machine, error) {
	st.MethodCall(st, "AddOneMachine", template)
	st.calls++
	st.machineTemplates = append(st.machineTemplates, template)
	m := state.Machine{}
	return &m, st.err
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	st.MethodCall(st, "GetBlockForType", t)
	if st.block == t {
		return &mockBlock{t: t, m: st.blockMsg}, true, nil
	} else {
		return nil, false, nil
	}
}

func (st *mockState) ModelTag() names.ModelTag {
	st.MethodCall(st, "ModelTag")
	return names.NewModelTag("deadbeef-2f18-4fd2-967d-db9663db7bea")
}

func (st *mockState) Model() (machinemanager.Model, error) {
	st.MethodCall(st, "Model")
	return &mockModel{
		disableOSUpgrade: st.disableOSUpgrade,
		disableOSRefresh: st.disableOSRefresh,
	}, nil
}

func (st *mockState) CloudCredential(tag names.CloudCredentialTag) (state.Credential, error) {
	st.MethodCall(st, "CloudCredential", tag)
	return state.Credential{}, nil
}

func (st *mockState) Cloud(s string) (cloud.Cloud, error) {
	st.MethodCall(st, "Cloud", s)
	return cloud.Cloud{}, nil
}

func (st *mockState) Machine(id string) (machinemanager.Machine, error) {
	st.MethodCall(st, "Machine", id)
	if m, ok := st.machines[id]; !ok {
		return nil, errors.NotFoundf("machine %v", id)
	} else {
		return m, nil
	}
}

func (st *mockState) Application(id string) (machinemanager.Application, error) {
	st.MethodCall(st, "Application", id)
	if a, ok := st.applications[id]; !ok {
		return nil, errors.NotFoundf("application %s", id)
	} else {
		return a, nil
	}
}

func (st *mockState) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	st.MethodCall(st, "StorageInstance", tag)
	return &mockStorage{
		tag:  tag,
		kind: state.StorageKindBlock,
	}, nil
}

func (st *mockState) UnitStorageAttachments(tag names.UnitTag) ([]state.StorageAttachment, error) {
	st.MethodCall(st, "UnitStorageAttachments", tag)
	return st.unitStorageAttachmentsF(tag)
}

type mockBlock struct {
	state.Block
	t state.BlockType
	m string
}

func (st *mockBlock) Id() string {
	return "id"
}

func (st *mockBlock) Tag() (names.Tag, error) {
	return names.ParseTag("machine-1")
}

func (st *mockBlock) Type() state.BlockType {
	return state.ChangeBlock
}

func (st *mockBlock) Message() string {
	return st.m
}

func (st *mockBlock) ModelUUID() string {
	return "uuid"
}

type mockMachine struct {
	jtesting.Stub
	machinemanager.Machine

	id                       string
	keep                     bool
	series                   string
	units                    []string
	containers               []string
	unitAgentState           status.Status
	unitState                status.Status
	isManager                bool
	isLockedForSeriesUpgrade bool

	unitsF func() ([]machinemanager.Unit, error)
}

func (m *mockMachine) Id() string {
	m.MethodCall(m, "Id")
	return m.id
}

func (m *mockMachine) Tag() names.Tag {
	m.MethodCall(m, "Tag")
	return names.NewMachineTag(m.id)
}

func (m *mockMachine) Destroy() error {
	m.MethodCall(m, "Destroy")
	if len(m.containers) > 0 {
		return stateerrors.NewHasContainersError(m.id, m.containers)
	}
	if len(m.units) > 0 {
		return stateerrors.NewHasAssignedUnitsError(m.id, m.units)
	}
	return nil
}

func (m *mockMachine) ForceDestroy(maxWait time.Duration) error {
	m.MethodCall(m, "ForceDestroy", maxWait)
	return nil
}

func (m *mockMachine) Principals() []string {
	m.MethodCall(m, "Principals")
	return m.units
}

func (m *mockMachine) SetKeepInstance(keep bool) error {
	m.MethodCall(m, "SetKeepInstance", keep)
	m.keep = keep
	return nil
}

func (m *mockMachine) Series() string {
	m.MethodCall(m, "Release")
	return m.series
}

func (m *mockMachine) Containers() ([]string, error) {
	m.MethodCall(m, "Containers")
	return m.containers, nil
}

func (m *mockMachine) Units() ([]machinemanager.Unit, error) {
	m.MethodCall(m, "Units")
	if m.unitsF != nil {
		return m.unitsF()
	}
	return []machinemanager.Unit{
		&mockUnit{tag: names.NewUnitTag("foo/0"), agentStatus: m.unitAgentState, unitStatus: m.unitState},
		&mockUnit{tag: names.NewUnitTag("foo/1"), agentStatus: m.unitAgentState, unitStatus: m.unitState},
		&mockUnit{tag: names.NewUnitTag("foo/2"), agentStatus: m.unitAgentState, unitStatus: m.unitState},
	}, nil
}

func (m *mockMachine) VerifyUnitsSeries(units []string, series string, force bool) ([]machinemanager.Unit, error) {
	m.MethodCall(m, "VerifyUnitsSeries", units, series, force)
	out := make([]machinemanager.Unit, len(m.units))
	for i, name := range m.units {
		out[i] = &mockUnit{
			tag:         names.NewUnitTag(name),
			agentStatus: m.unitAgentState,
			unitStatus:  m.unitState,
		}
	}
	return out, m.NextErr()
}

func (m *mockMachine) CreateUpgradeSeriesLock(unitTags []string, series string) error {
	m.MethodCall(m, "CreateUpgradeSeriesLock", unitTags, series)
	return m.NextErr()
}

func (m *mockMachine) RemoveUpgradeSeriesLock() error {
	m.MethodCall(m, "RemoveUpgradeSeriesLock")
	return m.NextErr()
}

func (m *mockMachine) CompleteUpgradeSeries() error {
	m.MethodCall(m, "CompleteUpgradeSeries")
	return m.NextErr()
}

func (m *mockMachine) IsManager() bool {
	m.MethodCall(m, "IsManager")
	return m.isManager
}

func (m *mockMachine) IsLockedForSeriesUpgrade() (bool, error) {
	m.MethodCall(m, "IsLockedForSeriesUpgrade")
	return m.isLockedForSeriesUpgrade, nil
}

func (m *mockMachine) UpgradeSeriesStatus() (model.UpgradeSeriesStatus, error) {
	m.MethodCall(m, "UpgradeSeriesStatus")
	return model.UpgradeSeriesNotStarted, nil
}

func (m *mockMachine) SetUpgradeSeriesStatus(status model.UpgradeSeriesStatus, message string) error {
	m.MethodCall(m, "SetUpgradeSeriesStatus", status, message)
	return nil
}

func (m *mockMachine) ApplicationNames() ([]string, error) {
	m.MethodCall(m, "ApplicationNames")
	return []string{"foo"}, nil
}

func (m *mockMachine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	m.MethodCall(m, "HardwareCharacteristics")
	arch := "amd64"
	return &instance.HardwareCharacteristics{
		Arch: &arch,
	}, nil
}

func (m *mockMachine) SetPassword(p string) error {
	m.MethodCall(m, "SetPassword")
	return nil
}

type mockApplication struct {
	jtesting.Stub
	charm       *mockCharm
	charmOrigin *state.CharmOrigin
}

func (a *mockApplication) Name() string {
	return "foo"
}

func (a *mockApplication) Charm() (machinemanager.Charm, bool, error) {
	a.MethodCall(a, "Charm")
	if a.charm == nil {
		return &mockCharm{}, false, nil
	}
	return a.charm, false, nil
}

func (a *mockApplication) CharmOrigin() *state.CharmOrigin {
	if a.charmOrigin == nil {
		return &state.CharmOrigin{}
	}
	return a.charmOrigin
}

type mockCharm struct {
	jtesting.Stub
	meta *mockMeta
}

func (a *mockCharm) URL() *charm.URL {
	a.MethodCall(a, "URL")
	return nil
}

func (a *mockCharm) Meta() *charm.Meta {
	a.MethodCall(a, "Meta")
	if a.meta == nil {
		return &charm.Meta{Series: []string{"xenial"}}
	}
	return nil
}

func (a *mockCharm) Manifest() *charm.Manifest {
	a.MethodCall(a, "Manifest")
	if a.meta == nil {
		return &charm.Manifest{}
	}
	return nil
}

func (a *mockCharm) String() string {
	a.MethodCall(a, "String")
	return ""
}

type mockMeta struct {
	jtesting.Stub
	series []string
}

func (a *mockMeta) ComputedSeries() []string {
	a.MethodCall(a, "ComputedSeries")
	return a.series
}

type mockUnit struct {
	tag         names.UnitTag
	agentStatus status.Status
	unitStatus  status.Status
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return u.tag
}

func (u *mockUnit) Name() string {
	return u.tag.String()
}

func (u *mockUnit) AgentStatus() (status.StatusInfo, error) {
	return status.StatusInfo{Status: u.agentStatus}, nil
}

func (u *mockUnit) Status() (status.StatusInfo, error) {
	return status.StatusInfo{Status: u.unitStatus}, nil
}

func (u *mockUnit) ApplicationName() string {
	return strings.Split(u.tag.String(), "-")[1]
}

type mockStorage struct {
	state.StorageInstance
	tag  names.StorageTag
	kind state.StorageKind
}

func (a *mockStorage) StorageTag() names.StorageTag {
	return a.tag
}

func (a *mockStorage) Kind() state.StorageKind {
	return a.kind
}

type mockStorageAttachment struct {
	state.StorageAttachment
	unit    names.UnitTag
	storage names.StorageTag
}

func (a *mockStorageAttachment) Unit() names.UnitTag {
	return a.unit
}

func (a *mockStorageAttachment) StorageInstance() names.StorageTag {
	return a.storage
}

type mockVolume struct {
	state.Volume
	detachable bool
}

func (v *mockVolume) Detachable() bool {
	return v.detachable
}
