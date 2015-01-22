// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/diskformatter"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&DiskFormatterSuite{})

type DiskFormatterSuite struct {
	coretesting.BaseSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	tag        names.UnitTag
	st         *mockState
	api        *diskformatter.DiskFormatterAPI
}

func (s *DiskFormatterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.tag = names.NewUnitTag("service/0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: s.tag}
	s.st = &mockState{}
	diskformatter.PatchState(s, s.st)

	var err error
	s.api, err = diskformatter.NewDiskFormatterAPI(nil, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DiskFormatterSuite) TestWatchBlockDevices(c *gc.C) {
	results, err := s.api.WatchBlockDevices(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-service-0"},
			{Tag: "disk-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: &params.Error{Message: "WatchUnitMachineBlockDevices fails", Code: ""}},
			// disk-1 does not exist, so we get ErrPerm.
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
	c.Assert(s.st.calls, gc.DeepEquals, []string{"WatchUnitMachineBlockDevices"})
	c.Assert(s.st.unitTags, gc.DeepEquals, []names.UnitTag{s.tag})
}

func (s *DiskFormatterSuite) TestBlockDevices(c *gc.C) {
	s.st.devices = map[string]state.BlockDevice{
		"0": &mockBlockDevice{
			name:            "0",
			storageInstance: "storage/0",
			info:            &state.BlockDeviceInfo{},
			attached:        true,
		},
		"1": &mockBlockDevice{
			storageInstance: "storage/1",
			attached:        true,
		},
		"2": &mockBlockDevice{
			attached: true,
		},
		"3": &mockBlockDevice{
			name:            "3",
			storageInstance: "storage/0",
			attached:        true,
		},
		"4": &mockBlockDevice{
			attached: false,
		},
	}
	s.st.storageInstances = map[string]state.StorageInstance{
		"storage/0": &mockStorageInstance{owner: s.tag},
		"storage/1": &mockStorageInstance{owner: names.NewServiceTag("mysql")},
	}

	results, err := s.api.BlockDevices(params.Entities{
		Entities: []params.Entity{
			{Tag: "disk-0"},
			{Tag: "disk-1"}, // different owner
			{Tag: "disk-2"}, // no storage instance
			{Tag: "disk-3"}, // not provisioned
			{Tag: "disk-4"}, // unattached
			{Tag: "disk-5"}, // missing
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.BlockDeviceResults{
		Results: []params.BlockDeviceResult{
			{Result: storage.BlockDevice{Name: "0"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: `block device "3" not provisioned`, Code: "not provisioned"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
	c.Assert(s.st.calls, gc.DeepEquals, []string{
		"BlockDevice", "StorageInstance",
		"BlockDevice", "StorageInstance",
		"BlockDevice", // no storage instance
		"BlockDevice", "StorageInstance",
		"BlockDevice", // unattached
		"BlockDevice", // missing
	})
	c.Assert(s.st.blockDeviceNames, gc.DeepEquals, []string{
		"0", "1", "2", "3", "4", "5",
	})
	c.Assert(s.st.storageInstanceIds, gc.DeepEquals, []string{
		"storage/0", "storage/1", "storage/0",
	})
}

func (s *DiskFormatterSuite) TestBlockDeviceStorageInstances(c *gc.C) {
	s.st.devices = map[string]state.BlockDevice{
		"0": &mockBlockDevice{
			name:            "0",
			storageInstance: "storage/0",
			info:            &state.BlockDeviceInfo{},
			attached:        true,
		},
		"1": &mockBlockDevice{
			name:            "1",
			storageInstance: "storage/1",
			info:            &state.BlockDeviceInfo{},
			attached:        true,
		},
	}
	s.st.storageInstances = map[string]state.StorageInstance{
		"storage/0": &mockStorageInstance{
			id:    "storage/0",
			owner: s.tag,
			kind:  state.StorageKindBlock,
		},
		"storage/1": &mockStorageInstance{
			id:    "storage/1",
			owner: s.tag,
			kind:  state.StorageKindFilesystem,
		},
	}

	results, err := s.api.BlockDeviceStorageInstances(params.Entities{
		Entities: []params.Entity{{Tag: "disk-0"}, {Tag: "disk-1"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StorageInstanceResults{
		Results: []params.StorageInstanceResult{
			{Result: storage.StorageInstance{
				Id:   "storage/0",
				Kind: storage.StorageKindBlock,
			}},
			{Result: storage.StorageInstance{
				Id:   "storage/1",
				Kind: storage.StorageKindFilesystem,
			}},
		},
	})
	c.Assert(s.st.calls, gc.DeepEquals, []string{
		"BlockDevice", "StorageInstance",
		"BlockDevice", "StorageInstance",
	})
	c.Assert(s.st.blockDeviceNames, gc.DeepEquals, []string{
		"0", "1",
	})
	c.Assert(s.st.storageInstanceIds, gc.DeepEquals, []string{
		"storage/0", "storage/1",
	})
}

type mockState struct {
	calls            []string
	devices          map[string]state.BlockDevice
	storageInstances map[string]state.StorageInstance

	unitTags           []names.UnitTag
	blockDeviceNames   []string
	storageInstanceIds []string
}

func (st *mockState) WatchUnitMachineBlockDevices(tag names.UnitTag) (watcher.StringsWatcher, error) {
	st.calls = append(st.calls, "WatchUnitMachineBlockDevices")
	st.unitTags = append(st.unitTags, tag)
	return nil, errors.New("WatchUnitMachineBlockDevices fails")
}

func (st *mockState) BlockDevice(name string) (state.BlockDevice, error) {
	st.calls = append(st.calls, "BlockDevice")
	st.blockDeviceNames = append(st.blockDeviceNames, name)
	blockDevice, ok := st.devices[name]
	if !ok {
		return nil, errors.NotFoundf("block device %q", name)
	}
	return blockDevice, nil
}

func (st *mockState) StorageInstance(id string) (state.StorageInstance, error) {
	st.calls = append(st.calls, "StorageInstance")
	st.storageInstanceIds = append(st.storageInstanceIds, id)
	storageInstance, ok := st.storageInstances[id]
	if !ok {
		return nil, errors.NotFoundf("storage instance %q", id)
	}
	return storageInstance, nil
}

type mockBlockDevice struct {
	state.BlockDevice
	name            string
	storageInstance string
	attached        bool
	info            *state.BlockDeviceInfo
}

func (d *mockBlockDevice) Name() string {
	return d.name
}

func (d *mockBlockDevice) Attached() bool {
	return d.attached
}

func (d *mockBlockDevice) Info() (state.BlockDeviceInfo, error) {
	if d.info == nil {
		return state.BlockDeviceInfo{}, errors.NotProvisionedf("block device %q", d.name)
	}
	return *d.info, nil
}

func (d *mockBlockDevice) StorageInstance() (string, bool) {
	return d.storageInstance, d.storageInstance != ""
}

type mockStorageInstance struct {
	state.StorageInstance
	id    string
	owner names.Tag
	kind  state.StorageKind
}

func (d *mockStorageInstance) Id() string {
	return d.id
}

func (d *mockStorageInstance) Owner() names.Tag {
	return d.owner
}

func (d *mockStorageInstance) Kind() state.StorageKind {
	return d.kind
}
