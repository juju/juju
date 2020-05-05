// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/os/series"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facades/client/machinemanager"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&MachineManagerSuite{})

type MachineManagerSuite struct {
	coretesting.BaseSuite
	authorizer *apiservertesting.FakeAuthorizer
	st         *mockState
	pool       *mockPool
	api        *machinemanager.MachineManagerAPI

	callContext context.ProviderCallContext
}

func (s *MachineManagerSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	mm, err := machinemanager.NewMachineManagerAPI(s.st, s.st, s.pool, s.authorizer, s.st.ModelTag(), s.callContext, common.NewResources())
	c.Assert(err, jc.ErrorIsNil)
	s.api = mm
}

func (s *MachineManagerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.st = &mockState{
		machines: make(map[string]*mockMachine),
		unitStorageAttachmentsF: func(tag names.UnitTag) ([]state.StorageAttachment, error) {
			if tag.Id() == "foo/0" {
				return []state.StorageAttachment{
					&mockStorageAttachment{unit: tag, storage: names.NewStorageTag("disks/0")},
					&mockStorageAttachment{unit: tag, storage: names.NewStorageTag("disks/1")},
				}, nil
			}
			return nil, nil
		},
	}
	s.pool = &mockPool{}
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin")}
	s.callContext = context.NewCloudCallContext()
	var err error
	s.api, err = machinemanager.NewMachineManagerAPI(s.st, s.st, s.pool, s.authorizer, s.st.ModelTag(), s.callContext, common.NewResources())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MachineManagerSuite) TestAddMachines(c *gc.C) {
	apiParams := make([]params.AddMachineParams, 2)
	for i := range apiParams {
		apiParams[i] = params.AddMachineParams{
			Series: "trusty",
			Jobs:   []model.MachineJob{model.JobHostUnits},
		}
	}
	apiParams[0].Disks = []storage.Constraints{{Size: 1, Count: 2}, {Size: 2, Count: 1}}
	apiParams[1].Disks = []storage.Constraints{{Size: 1, Count: 2, Pool: "three"}}
	machines, err := s.api.AddMachines(params.AddMachines{MachineParams: apiParams})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machines.Machines, gc.HasLen, 2)
	c.Assert(s.st.calls, gc.Equals, 2)
	c.Assert(s.st.machineTemplates, jc.DeepEquals, []state.MachineTemplate{
		{
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
		},
		{
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
		},
	})
}

func (s *MachineManagerSuite) TestNewMachineManagerAPINonClient(c *gc.C) {
	tag := names.NewUnitTag("mysql/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: tag}
	_, err := machinemanager.NewMachineManagerAPI(nil, nil, nil, s.authorizer, names.ModelTag{}, s.callContext, common.NewResources())
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *MachineManagerSuite) TestAddMachinesStateError(c *gc.C) {
	s.st.err = errors.New("boom")
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
	c.Assert(s.st.calls, gc.Equals, 1)
}

func (s *MachineManagerSuite) TestDestroyMachine(c *gc.C) {
	s.st.machines["0"] = &mockMachine{}
	results, err := s.api.DestroyMachine(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
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

func (s *MachineManagerSuite) TestForceDestroyMachine(c *gc.C) {
	s.st.machines["0"] = &mockMachine{}
	results, err := s.api.ForceDestroyMachine(params.Entities{
		Entities: []params.Entity{{Tag: "machine-0"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
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

func (s *MachineManagerSuite) assertMachinesDestroyed(c *gc.C, in []params.Entity, out params.DestroyMachineResults, expectedCalls ...string) {
	results, err := s.api.DestroyMachine(params.Entities{in})
	c.Assert(err, jc.ErrorIsNil)

	s.st.CheckCallNames(c, expectedCalls...)
	c.Assert(results, jc.DeepEquals, out)

}

func (s *MachineManagerSuite) TestDestroyMachineFailedAllStorageRetrieval(c *gc.C) {
	s.st.machines["0"] = &mockMachine{}
	s.st.unitStorageAttachmentsF = func(tag names.UnitTag) ([]state.StorageAttachment, error) {
		return nil, errors.New("kaboom")
	}
	s.assertMachinesDestroyed(c,
		[]params.Entity{{Tag: "machine-0"}},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Error: common.ServerError(errors.New("getting storage for unit foo/0: kaboom\ngetting storage for unit foo/1: kaboom\ngetting storage for unit foo/2: kaboom")),
			}},
		},
		"ModelTag",
		"GetBlockForType",
		"GetBlockForType",
		"Machine",
		"UnitStorageAttachments",
		"UnitStorageAttachments",
		"UnitStorageAttachments",
	)
}

func (s *MachineManagerSuite) TestDestroyMachineFailedAllStorageClassification(c *gc.C) {
	s.st.machines["0"] = &mockMachine{}
	s.st.SetErrors(
		errors.New("boom"),
	)
	s.assertMachinesDestroyed(c,
		[]params.Entity{{Tag: "machine-0"}},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Error: common.ServerError(errors.New("classifying storage for destruction for unit foo/0: boom")),
			}},
		},
		"ModelTag",
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
	s.st.machines["0"] = &mockMachine{}
	s.st.unitStorageAttachmentsF = func(tag names.UnitTag) ([]state.StorageAttachment, error) {
		if tag.Id() == "foo/1" {
			return nil, errors.New("kaboom")
		}
		return nil, nil
	}

	s.assertMachinesDestroyed(c,
		[]params.Entity{{Tag: "machine-0"}},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Error: common.ServerError(errors.New("getting storage for unit foo/1: kaboom")),
			}},
		},
		"ModelTag",
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
	s.st.machines["0"] = &mockMachine{}
	s.st.machines["1"] = &mockMachine{
		unitsF: func() ([]machinemanager.Unit, error) {
			return []machinemanager.Unit{
				&mockUnit{tag: names.NewUnitTag("bar/0")},
			}, nil
		},
	}
	s.st.unitStorageAttachmentsF = func(tag names.UnitTag) ([]state.StorageAttachment, error) {
		if tag.Id() == "foo/1" {
			return nil, errors.New("kaboom")
		}
		if tag.Id() == "bar/0" {
			return []state.StorageAttachment{
				&mockStorageAttachment{unit: tag, storage: names.NewStorageTag("disks/0")},
			}, nil
		}
		return nil, nil
	}

	s.assertMachinesDestroyed(c,
		[]params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
		},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{
				{Error: common.ServerError(errors.New("getting storage for unit foo/1: kaboom"))},
				{Info: &params.DestroyMachineInfo{
					DestroyedUnits: []params.Entity{
						{"unit-bar-0"},
					},
					DetachedStorage: []params.Entity{
						{"storage-disks-0"},
					},
				}},
			},
		},
		"ModelTag",
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

func (s *MachineManagerSuite) TestDestroyMachineWithParamsV4(c *gc.C) {
	apiV4 := s.machineManagerAPIV4()
	s.st.machines["0"] = &mockMachine{}
	results, err := apiV4.DestroyMachineWithParams(params.DestroyMachinesParams{
		Keep:        true,
		Force:       true,
		MachineTags: []string{"machine-0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.st.Machine("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.(*mockMachine).keep, jc.IsTrue)
	c.Assert(results, jc.DeepEquals, params.DestroyMachineResults{
		Results: []params.DestroyMachineResult{{
			Info: &params.DestroyMachineInfo{
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
	noWait := 0 * time.Second
	s.assertDestroyMachineWithParams(c,
		params.DestroyMachinesParams{
			Keep:        true,
			Force:       true,
			MachineTags: []string{"machine-0"},
			MaxWait:     &noWait,
		},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Info: &params.DestroyMachineInfo{
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

func (s *MachineManagerSuite) TestDestroyMachineWithParamsNilWait(c *gc.C) {
	s.assertDestroyMachineWithParams(c,
		params.DestroyMachinesParams{
			Keep:        true,
			Force:       true,
			MachineTags: []string{"machine-0"},
			// This will use max wait of system default for delay between cleanup operations.
		},
		params.DestroyMachineResults{
			Results: []params.DestroyMachineResult{{
				Info: &params.DestroyMachineInfo{
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

func (s *MachineManagerSuite) setupUpgradeSeries(c *gc.C) {
	s.st.machines = map[string]*mockMachine{
		"0": {series: "trusty", units: []string{"foo/0", "test/0"}},
		"1": {series: "trusty", units: []string{"foo/1", "test/1"}},
		"2": {series: "centos7", units: []string{"foo/1", "test/1"}},
		"3": {series: "bionic", isManager: true},
	}
}

func (s *MachineManagerSuite) apiV5() machinemanager.MachineManagerAPIV5 {
	return machinemanager.MachineManagerAPIV5{MachineManagerAPIV6: &machinemanager.MachineManagerAPIV6{s.api}}
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateOK(c *gc.C) {
	s.setupUpgradeSeries(c)
	s.st.machines["0"].unitAgentState = status.Idle

	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
		}},
	}
	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)

	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)

	var expectedUnitNames []string
	for _, unit := range s.st.machines["0"].Principals() {
		expectedUnitNames = append(expectedUnitNames, unit)
	}
	c.Assert(result.UnitNames, gc.DeepEquals, expectedUnitNames)
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateIsControllerError(c *gc.C) {
	s.setupUpgradeSeries(c)
	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("3").String()},
		}},
	}
	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"machine-3 is a controller and cannot be targeted for series upgrade")
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateNoSeriesError(c *gc.C) {
	s.setupUpgradeSeries(c)
	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("1").String()},
		}},
	}
	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results[0].Error, gc.ErrorMatches, "series missing from args")
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateNotFromUbuntuError(c *gc.C) {
	s.setupUpgradeSeries(c)
	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("2").String()},
			Series: "bionic",
		}},
	}

	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"machine-2 is running CentOS and is not valid for Ubuntu series upgrade")
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateNotToUbuntuError(c *gc.C) {
	s.setupUpgradeSeries(c)
	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("1").String()},
			Series: "centos7",
		}},
	}

	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		`series "centos7" is from OS "CentOS" and is not a valid upgrade target`)
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateAlreadyRunningSeriesError(c *gc.C) {
	s.setupUpgradeSeries(c)
	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("1").String()},
			Series: "trusty",
		}},
	}

	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, "machine-1 is already running series trusty")
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateOlderSeriesError(c *gc.C) {
	s.setupUpgradeSeries(c)
	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("1").String()},
			Series: "precise",
		}},
	}

	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"machine machine-1 is running trusty which is a newer series than precise.")
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateUnitNotIdleError(c *gc.C) {
	s.setupUpgradeSeries(c)
	s.st.machines["0"].unitAgentState = status.Executing
	s.st.machines["0"].unitState = status.Active

	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
		}},
	}
	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"unit unit-foo-[0-2] is not ready to start a series upgrade; its agent status is: \"executing\" ")
}

func (s *MachineManagerSuite) TestUpgradeSeriesValidateUnitStatusError(c *gc.C) {
	s.setupUpgradeSeries(c)
	s.st.machines["0"].unitAgentState = status.Idle
	s.st.machines["0"].unitState = status.Error

	apiV5 := s.apiV5()
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
			Series: "xenial",
		}},
	}
	results, err := apiV5.UpgradeSeriesValidate(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches,
		"unit unit-foo-[0-2] is not ready to start a series upgrade; its status is: \"error\" ")
}

func (s *MachineManagerSuite) TestUpgradeSeriesPrepare(c *gc.C) {
	s.setupUpgradeSeries(c)
	s.st.machines["0"].unitAgentState = status.Idle

	apiV5 := s.apiV5()
	machineTag := names.NewMachineTag("0")
	result, err := apiV5.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "xenial",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	mach := s.st.machines["0"]
	c.Assert(len(mach.Calls()), gc.Equals, 3)
	mach.CheckCallNames(c, "Principals", "VerifyUnitsSeries", "CreateUpgradeSeriesLock")
	mach.CheckCall(c, 2, "CreateUpgradeSeriesLock", []string{"foo/0", "test/0"}, "xenial")
}

func (s *MachineManagerSuite) TestUpgradeSeriesPrepareMachineNotFound(c *gc.C) {
	apiV5 := s.apiV5()
	machineTag := names.NewMachineTag("76")
	result, err := apiV5.UpgradeSeriesPrepare(
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
	apiV5 := s.apiV5()
	unitTag := names.NewUnitTag("mysql/0")
	result, err := apiV5.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
			Entity: params.Entity{
				Tag: unitTag.String()},
			Series: "trusty",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.ErrorMatches, "\"unit-mysql-0\" is not a valid machine tag")
}

func (s *MachineManagerSuite) TestUpgradeSeriesPreparePermissionDenied(c *gc.C) {
	user := names.NewUserTag("fred")
	s.setAPIUser(c, user)
	apiV5 := s.apiV5()
	machineTag := names.NewMachineTag("0")
	_, err := apiV5.UpgradeSeriesPrepare(
		params.UpdateSeriesArg{
			Entity: params.Entity{
				Tag: machineTag.String()},
			Series: "xenial",
		},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *MachineManagerSuite) TestUpgradeSeriesPrepareBlockedChanges(c *gc.C) {
	apiV5 := s.apiV5()
	s.st.blockMsg = "TestUpgradeSeriesPrepareBlockedChanges"
	s.st.block = state.ChangeBlock
	_, err := apiV5.UpgradeSeriesPrepare(
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
	apiV5 := s.apiV5()
	result, err := apiV5.UpgradeSeriesPrepare(
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

func (s *MachineManagerSuite) TestUpgradeSeriesPrepareIncompatibleSeries(c *gc.C) {
	s.setupUpgradeSeries(c)
	s.st.machines["0"].SetErrors(&state.ErrIncompatibleSeries{
		SeriesList: []string{"yakkety", "zesty"},
		Series:     "xenial",
		CharmName:  "TestCharm",
	})
	apiV5 := s.apiV5()
	result, err := apiV5.UpgradeSeriesPrepare(
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

func (s *MachineManagerSuite) TestUpgradeSeriesPrepareRemoveLockAfterFail(c *gc.C) {
	// TODO managed upgrade series
}

func (s *MachineManagerSuite) TestUpgradeSeriesComplete(c *gc.C) {
	s.setupUpgradeSeries(c)
	apiV5 := s.apiV5()
	_, err := apiV5.UpgradeSeriesComplete(
		params.UpdateSeriesArg{
			Entity: params.Entity{Tag: names.NewMachineTag("0").String()},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

// TestIsSeriesLessThan tests a validation method which is not very complicated
// but complex enough to warrant being exported from an export test package for
// testing.
func (s *MachineManagerSuite) TestIsSeriesLessThan(c *gc.C) {
	ss := series.SupportedSeries()

	// get the series versions
	vs := make([]string, 0, len(ss))
	for _, ser := range ss {
		ver, err := series.SeriesVersion(ser)
		c.Assert(err, jc.ErrorIsNil)
		vs = append(vs, ver)
	}

	// sort the values, so the lexicographical order is determined
	sort.Strings(vs)

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
	err              error
	blockMsg         string
	block            state.BlockType

	unitStorageAttachmentsF func(tag names.UnitTag) ([]state.StorageAttachment, error)
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
	return &mockModel{}, nil
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

	keep           bool
	series         string
	units          []string
	unitAgentState status.Status
	unitState      status.Status
	isManager      bool

	unitsF func() ([]machinemanager.Unit, error)
}

func (m *mockMachine) Destroy() error {
	m.MethodCall(m, "Destroy")
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
	m.MethodCall(m, "Series")
	return m.series
}

func (m *mockMachine) Units() ([]machinemanager.Unit, error) {
	m.MethodCall(m, "Units")
	if m.unitsF != nil {
		return m.unitsF()
	}
	return []machinemanager.Unit{
		&mockUnit{tag: names.NewUnitTag("foo/0")},
		&mockUnit{tag: names.NewUnitTag("foo/1")},
		&mockUnit{tag: names.NewUnitTag("foo/2")},
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

func (s *MachineManagerSuite) machineManagerAPIV4() machinemanager.MachineManagerAPIV4 {
	apiV5 := s.apiV5()
	return machinemanager.MachineManagerAPIV4{&apiV5}
}
