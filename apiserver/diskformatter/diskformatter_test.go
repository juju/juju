// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/diskformatter"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&DiskFormatterSuite{})

type DiskFormatterSuite struct {
	coretesting.BaseSuite
	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	tag        names.MachineTag
	st         *mockState
	api        *diskformatter.DiskFormatterAPI
}

func (s *DiskFormatterSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.tag = names.NewMachineTag("0")
	s.authorizer = &apiservertesting.FakeAuthorizer{Tag: s.tag}
	s.st = &mockState{}
	diskformatter.PatchState(s, s.st)

	var err error
	s.api, err = diskformatter.NewDiskFormatterAPI(nil, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *DiskFormatterSuite) TestWatchAttachedVolumes(c *gc.C) {
	results, err := s.api.WatchAttachedVolumes(params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "unit-service-0"},
			{Tag: "disk-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
	c.Assert(s.st.calls, gc.DeepEquals, []call{{
		"WatchBlockDevices", []interface{}{names.NewMachineTag("0")},
	}})
}

func (s *DiskFormatterSuite) TestAttachedVolumes(c *gc.C) {
	machine0 := names.NewMachineTag("0")
	volume0 := names.NewDiskTag("0")
	volume1 := names.NewDiskTag("1")
	volume2 := names.NewDiskTag("2")

	s.st.devices = map[names.MachineTag][]state.BlockDeviceInfo{
		machine0: {{
			DeviceName: "sda",
			Serial:     "capncrunch",
		}, {
			DeviceName: "sdb",
		}},
	}

	s.st.volumes = map[names.DiskTag]*mockVolume{
		volume0: {
			tag: volume0,
			info: &state.VolumeInfo{
				VolumeId: "vol-0",
				Serial:   "capncrunch",
			},
		},
		volume1: {tag: volume1, info: &state.VolumeInfo{VolumeId: "vol-1"}},
		volume2: {tag: volume2, info: &state.VolumeInfo{VolumeId: "vol-2"}},
	}

	s.st.volumeAttachments = []*mockVolumeAttachment{{
		volume0,
		machine0,
		&state.VolumeAttachmentInfo{},
	}, {
		volume1,
		machine0,
		&state.VolumeAttachmentInfo{DeviceName: "sdb"},
	}, {
		volume2,
		machine0,
		nil, // not provisioned
	}}

	results, err := s.api.AttachedVolumes(params.Entities{
		Entities: []params.Entity{
			{Tag: "machine-0"},
			{Tag: "machine-1"},
			{Tag: "unit-service-0"},
			{Tag: "disk-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.VolumeAttachmentsResults{
		Results: []params.VolumeAttachmentsResult{{
			Attachments: []params.VolumeAttachment{{
				VolumeTag:  volume0.String(),
				VolumeId:   "vol-0",
				MachineTag: machine0.String(),
			}, {
				VolumeTag:  volume1.String(),
				VolumeId:   "vol-1",
				MachineTag: machine0.String(),
				DeviceName: "sdb",
			}},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}, {
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}},
	})

	c.Assert(s.st.calls, gc.DeepEquals, []call{{
		"MachineVolumeAttachments", []interface{}{machine0},
	}, {
		"BlockDevices", []interface{}{machine0},
	}, {
		"Volume", []interface{}{volume0},
	}, {
		"Volume", []interface{}{volume1},
	}, {
		"Volume", []interface{}{volume2},
	}})
}

func (s *DiskFormatterSuite) TestVolumeFormattingInfo(c *gc.C) {
	machine0 := names.NewMachineTag("0")
	volume0 := names.NewDiskTag("0")
	volume1 := names.NewDiskTag("1")
	volume2 := names.NewDiskTag("2")
	volume3 := names.NewDiskTag("3")
	storagefs := names.NewStorageTag("fs/0")
	storageblk := names.NewStorageTag("blk/0")

	s.st.devices = map[names.MachineTag][]state.BlockDeviceInfo{
		machine0: {{
			DeviceName: "sda",
			Serial:     "capncrunch",
		}, {
			DeviceName: "sdb",
		}, {
			DeviceName: "sdc",
		}, {
			DeviceName:     "sdd",
			FilesystemType: "afs",
		}},
	}

	s.st.storageInstances = map[names.StorageTag]*mockStorageInstance{
		storagefs:  {kind: state.StorageKindFilesystem},
		storageblk: {kind: state.StorageKindBlock},
	}

	s.st.volumes = map[names.DiskTag]*mockVolume{
		volume0: {
			tag:     volume0,
			storage: storagefs,
			info: &state.VolumeInfo{
				VolumeId: "vol-0",
				Serial:   "capncrunch",
			},
		},
		volume1: {
			tag:     volume1,
			storage: storagefs,
			info:    &state.VolumeInfo{VolumeId: "vol-1"},
		},
		volume2: {
			tag:     volume2,
			storage: storageblk,
			info:    &state.VolumeInfo{VolumeId: "vol-2"},
		},
		volume3: {
			tag:     volume3,
			storage: storagefs,
			info:    &state.VolumeInfo{VolumeId: "vol-3"},
		},
	}

	s.st.volumeAttachments = []*mockVolumeAttachment{{
		volume0,
		machine0,
		&state.VolumeAttachmentInfo{},
	}, {
		volume1,
		machine0,
		&state.VolumeAttachmentInfo{DeviceName: "sdb"},
	}, {
		volume2,
		machine0,
		&state.VolumeAttachmentInfo{DeviceName: "sdc"},
	}, {
		volume3,
		machine0,
		&state.VolumeAttachmentInfo{DeviceName: "sdd"},
	}}

	results, err := s.api.VolumeFormattingInfo(params.VolumeAttachmentIds{
		Ids: []params.VolumeAttachmentId{
			{MachineTag: "machine-0", VolumeTag: "disk-0"},
			{MachineTag: "machine-0", VolumeTag: "disk-1"},
			{MachineTag: "machine-0", VolumeTag: "disk-2"},
			{MachineTag: "machine-0", VolumeTag: "disk-3"},
			{MachineTag: "machine-1", VolumeTag: "disk-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.VolumeFormattingInfoResults{
		Results: []params.VolumeFormattingInfoResult{{
			Result: params.VolumeFormattingInfo{
				NeedsFormatting: true,
				DevicePath:      "/dev/disk/by-id/capncrunch",
			},
		}, {
			Result: params.VolumeFormattingInfo{
				NeedsFormatting: true,
				DevicePath:      "/dev/sdb",
			},
		}, {
			// not assigned to a "filesystem" storage instance
			Result: params.VolumeFormattingInfo{NeedsFormatting: false},
		}, {
			// block device already has a filesystem
			Result: params.VolumeFormattingInfo{NeedsFormatting: false},
		}, {
			// non-matching machine
			Error: &params.Error{Message: "permission denied", Code: "unauthorized access"},
		}},
	})

	c.Assert(s.st.calls, gc.DeepEquals, []call{
		{"Volume", []interface{}{volume0}},
		{"StorageInstance", []interface{}{storagefs}},
		{"VolumeAttachment", []interface{}{machine0, volume0}},
		{"BlockDevices", []interface{}{machine0}},
		{"Volume", []interface{}{volume1}},
		{"StorageInstance", []interface{}{storagefs}},
		{"VolumeAttachment", []interface{}{machine0, volume1}},
		// No call to "BlockDevices", as the results are cached.
		{"Volume", []interface{}{volume2}},
		{"StorageInstance", []interface{}{storageblk}},
		{"Volume", []interface{}{volume3}},
		{"StorageInstance", []interface{}{storagefs}},
		{"VolumeAttachment", []interface{}{machine0, volume3}},
		// No call to "BlockDevices", as the results are cached.
	})
}

type mockState struct {
	calls             []call
	devices           map[names.MachineTag][]state.BlockDeviceInfo
	storageInstances  map[names.StorageTag]*mockStorageInstance
	volumes           map[names.DiskTag]*mockVolume
	volumeAttachments []*mockVolumeAttachment
}

type call struct {
	method string
	args   []interface{}
}

func (st *mockState) recordCall(name string, args ...interface{}) call {
	c := call{name, args}
	st.calls = append(st.calls, c)
	return c
}

func (st *mockState) WatchBlockDevices(tag names.MachineTag) state.NotifyWatcher {
	st.recordCall("WatchBlockDevices", tag)
	c := make(chan struct{}, 1)
	c <- struct{}{}
	return &mockNotifyWatcher{c: c}
}

func (st *mockState) BlockDevices(tag names.MachineTag) ([]state.BlockDeviceInfo, error) {
	st.recordCall("BlockDevices", tag)
	return st.devices[tag], nil
}

func (st *mockState) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	st.recordCall("StorageInstance", tag)
	storageInstance, ok := st.storageInstances[tag]
	if !ok {
		return nil, errors.NotFoundf("storage instance %q", tag.Id())
	}
	return storageInstance, nil
}

func (st *mockState) Volume(tag names.DiskTag) (state.Volume, error) {
	st.recordCall("Volume", tag)
	volume, ok := st.volumes[tag]
	if !ok {
		return nil, errors.NotFoundf("volume %q", tag.Id())
	}
	return volume, nil
}

func (st *mockState) MachineVolumeAttachments(tag names.MachineTag) ([]state.VolumeAttachment, error) {
	st.recordCall("MachineVolumeAttachments", tag)
	var attachments []state.VolumeAttachment
	for _, att := range st.volumeAttachments {
		if att.machine == tag {
			attachments = append(attachments, att)
		}
	}
	return attachments, nil
}

func (st *mockState) VolumeAttachment(machine names.MachineTag, volume names.DiskTag) (state.VolumeAttachment, error) {
	st.recordCall("VolumeAttachment", machine, volume)
	for _, att := range st.volumeAttachments {
		if att.machine == machine && att.volume == volume {
			return att, nil
		}
	}
	return nil, errors.NotFoundf("volume %q on machine %q", volume.Id(), machine.Id())
}

type mockNotifyWatcher struct {
	state.NotifyWatcher
	c chan struct{}
}

func (w *mockNotifyWatcher) Changes() <-chan struct{} {
	return w.c
}

type mockVolume struct {
	state.Volume

	tag     names.DiskTag
	storage names.StorageTag
	info    *state.VolumeInfo
}

func (v *mockVolume) StorageInstance() (names.StorageTag, bool) {
	return v.storage, v.storage != names.StorageTag{}
}

func (v *mockVolume) Info() (state.VolumeInfo, error) {
	if v.info == nil {
		return state.VolumeInfo{}, errors.NotProvisionedf(
			"volume %q", v.tag.Id(),
		)
	}
	return *v.info, nil
}

type mockStorageInstance struct {
	state.StorageInstance
	kind state.StorageKind
}

func (d *mockStorageInstance) Kind() state.StorageKind {
	return d.kind
}

type mockVolumeAttachment struct {
	volume  names.DiskTag
	machine names.MachineTag
	info    *state.VolumeAttachmentInfo
}

func (a *mockVolumeAttachment) Volume() names.DiskTag {
	return a.volume
}

func (a *mockVolumeAttachment) Machine() names.MachineTag {
	return a.machine
}

func (a *mockVolumeAttachment) Info() (state.VolumeAttachmentInfo, error) {
	if a.info == nil {
		return state.VolumeAttachmentInfo{}, errors.NotProvisionedf(
			"volume %q on machine %q", a.volume.Id(), a.machine.Id(),
		)
	}
	return *a.info, nil
}

func (a *mockVolumeAttachment) Params() (state.VolumeAttachmentParams, bool) {
	return state.VolumeAttachmentParams{}, false
}
