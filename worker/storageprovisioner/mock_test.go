// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"strconv"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

const attachedVolumeId = "1"
const needsInstanceVolumeId = "23"

var dyingVolumeAttachmentId = params.MachineStorageId{
	MachineTag:    "machine-0",
	AttachmentTag: "volume-0",
}

var missingVolumeAttachmentId = params.MachineStorageId{
	MachineTag:    "machine-3",
	AttachmentTag: "volume-1",
}

type mockNotifyWatcher struct {
	changes chan struct{}
}

func (*mockNotifyWatcher) Stop() error {
	return nil
}

func (*mockNotifyWatcher) Err() error {
	return nil
}

func (w *mockNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}

type mockStringsWatcher struct {
	changes chan []string
}

func (*mockStringsWatcher) Stop() error {
	return nil
}

func (*mockStringsWatcher) Err() error {
	return nil
}

func (w *mockStringsWatcher) Changes() <-chan []string {
	return w.changes
}

type mockAttachmentsWatcher struct {
	changes chan []params.MachineStorageId
}

func (*mockAttachmentsWatcher) Stop() error {
	return nil
}

func (*mockAttachmentsWatcher) Err() error {
	return nil
}

func (w *mockAttachmentsWatcher) Changes() <-chan []params.MachineStorageId {
	return w.changes
}

type mockEnvironAccessor struct {
	watcher *mockNotifyWatcher
	mu      sync.Mutex
	cfg     *config.Config
}

func (e *mockEnvironAccessor) WatchForEnvironConfigChanges() (apiwatcher.NotifyWatcher, error) {
	return e.watcher, nil
}

func (e *mockEnvironAccessor) EnvironConfig() (*config.Config, error) {
	e.mu.Lock()
	cfg := e.cfg
	e.mu.Unlock()
	return cfg, nil
}

func (e *mockEnvironAccessor) setConfig(cfg *config.Config) {
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
}

func newMockEnvironAccessor(c *gc.C) *mockEnvironAccessor {
	return &mockEnvironAccessor{
		watcher: &mockNotifyWatcher{make(chan struct{}, 1)},
		cfg:     testing.EnvironConfig(c),
	}
}

type mockVolumeAccessor struct {
	volumesWatcher         *mockStringsWatcher
	attachmentsWatcher     *mockAttachmentsWatcher
	blockDevicesWatcher    *mockNotifyWatcher
	provisionedMachines    map[string]instance.Id
	provisionedVolumes     map[string]params.Volume
	provisionedAttachments map[params.MachineStorageId]params.VolumeAttachment
	blockDevices           map[params.MachineStorageId]storage.BlockDevice

	setVolumeInfo           func([]params.Volume) ([]params.ErrorResult, error)
	setVolumeAttachmentInfo func([]params.VolumeAttachment) ([]params.ErrorResult, error)
}

func (w *mockVolumeAccessor) WatchVolumes() (apiwatcher.StringsWatcher, error) {
	return w.volumesWatcher, nil
}

func (w *mockVolumeAccessor) WatchVolumeAttachments() (apiwatcher.MachineStorageIdsWatcher, error) {
	return w.attachmentsWatcher, nil
}

func (w *mockVolumeAccessor) WatchBlockDevices(tag names.MachineTag) (apiwatcher.NotifyWatcher, error) {
	return w.blockDevicesWatcher, nil
}

func (v *mockVolumeAccessor) Volumes(volumes []names.VolumeTag) ([]params.VolumeResult, error) {
	var result []params.VolumeResult
	for _, tag := range volumes {
		if vol, ok := v.provisionedVolumes[tag.String()]; ok {
			result = append(result, params.VolumeResult{Result: vol})
		} else {
			result = append(result, params.VolumeResult{
				Error: common.ServerError(errors.NotProvisionedf("volume %q", tag.Id())),
			})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeAttachments(ids []params.MachineStorageId) ([]params.VolumeAttachmentResult, error) {
	var result []params.VolumeAttachmentResult
	for _, id := range ids {
		if att, ok := v.provisionedAttachments[id]; ok {
			result = append(result, params.VolumeAttachmentResult{Result: att})
		} else {
			result = append(result, params.VolumeAttachmentResult{
				Error: common.ServerError(errors.NotProvisionedf("volume attachment %v", id)),
			})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeBlockDevices(ids []params.MachineStorageId) ([]params.BlockDeviceResult, error) {
	var result []params.BlockDeviceResult
	for _, id := range ids {
		if dev, ok := v.blockDevices[id]; ok {
			result = append(result, params.BlockDeviceResult{Result: dev})
		} else {
			result = append(result, params.BlockDeviceResult{
				Error: common.ServerError(errors.NotFoundf("block device for volume attachment %v", id)),
			})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeParams(volumes []names.VolumeTag) ([]params.VolumeParamsResult, error) {
	var result []params.VolumeParamsResult
	for _, tag := range volumes {
		if _, ok := v.provisionedVolumes[tag.String()]; ok {
			result = append(result, params.VolumeParamsResult{
				Error: &params.Error{Message: "already provisioned"},
			})
		} else {
			volumeParams := params.VolumeParams{
				VolumeTag: tag.String(),
				Size:      1024,
				Provider:  "dummy",
				Attributes: map[string]interface{}{
					"persistent": tag.String() == "volume-1",
				},
			}
			if tag.Id() == attachedVolumeId {
				volumeParams.Attachment = &params.VolumeAttachmentParams{
					VolumeTag:  tag.String(),
					MachineTag: "machine-1",
					Provider:   "dummy",
				}
			}
			result = append(result, params.VolumeParamsResult{Result: volumeParams})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeAttachmentParams(ids []params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error) {
	var result []params.VolumeAttachmentParamsResult
	for _, id := range ids {
		if _, ok := v.provisionedAttachments[id]; ok {
			result = append(result, params.VolumeAttachmentParamsResult{
				Error: &params.Error{Message: "already provisioned"},
			})
		} else {
			instanceId, _ := v.provisionedMachines[id.MachineTag]
			result = append(result, params.VolumeAttachmentParamsResult{Result: params.VolumeAttachmentParams{
				MachineTag: id.MachineTag,
				VolumeTag:  id.AttachmentTag,
				InstanceId: string(instanceId),
				Provider:   "dummy",
			}})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) SetVolumeInfo(volumes []params.Volume) ([]params.ErrorResult, error) {
	return v.setVolumeInfo(volumes)
}

func (v *mockVolumeAccessor) SetVolumeAttachmentInfo(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
	return v.setVolumeAttachmentInfo(volumeAttachments)
}

func newMockVolumeAccessor() *mockVolumeAccessor {
	return &mockVolumeAccessor{
		volumesWatcher:         &mockStringsWatcher{make(chan []string, 1)},
		attachmentsWatcher:     &mockAttachmentsWatcher{make(chan []params.MachineStorageId, 1)},
		blockDevicesWatcher:    &mockNotifyWatcher{make(chan struct{}, 1)},
		provisionedMachines:    make(map[string]instance.Id),
		provisionedVolumes:     make(map[string]params.Volume),
		provisionedAttachments: make(map[params.MachineStorageId]params.VolumeAttachment),
		blockDevices:           make(map[params.MachineStorageId]storage.BlockDevice),
	}
}

type mockFilesystemAccessor struct {
	filesystemsWatcher     *mockStringsWatcher
	attachmentsWatcher     *mockAttachmentsWatcher
	provisionedMachines    map[string]instance.Id
	provisionedFilesystems map[string]params.Filesystem
	provisionedAttachments map[params.MachineStorageId]params.FilesystemAttachment

	setFilesystemInfo           func([]params.Filesystem) ([]params.ErrorResult, error)
	setFilesystemAttachmentInfo func([]params.FilesystemAttachment) ([]params.ErrorResult, error)
}

func (w *mockFilesystemAccessor) WatchFilesystems() (apiwatcher.StringsWatcher, error) {
	return w.filesystemsWatcher, nil
}

func (w *mockFilesystemAccessor) WatchFilesystemAttachments() (apiwatcher.MachineStorageIdsWatcher, error) {
	return w.attachmentsWatcher, nil
}

func (v *mockFilesystemAccessor) Filesystems(filesystems []names.FilesystemTag) ([]params.FilesystemResult, error) {
	var result []params.FilesystemResult
	for _, tag := range filesystems {
		if vol, ok := v.provisionedFilesystems[tag.String()]; ok {
			result = append(result, params.FilesystemResult{Result: vol})
		} else {
			result = append(result, params.FilesystemResult{
				Error: common.ServerError(errors.NotProvisionedf("filesystem %q", tag.Id())),
			})
		}
	}
	return result, nil
}

func (v *mockFilesystemAccessor) FilesystemAttachments(ids []params.MachineStorageId) ([]params.FilesystemAttachmentResult, error) {
	var result []params.FilesystemAttachmentResult
	for _, id := range ids {
		if att, ok := v.provisionedAttachments[id]; ok {
			result = append(result, params.FilesystemAttachmentResult{Result: att})
		} else {
			result = append(result, params.FilesystemAttachmentResult{
				Error: common.ServerError(errors.NotProvisionedf("filesystem attachment %v", id)),
			})
		}
	}
	return result, nil
}

func (v *mockFilesystemAccessor) FilesystemParams(filesystems []names.FilesystemTag) ([]params.FilesystemParamsResult, error) {
	var result []params.FilesystemParamsResult
	for _, tag := range filesystems {
		if _, ok := v.provisionedFilesystems[tag.String()]; ok {
			result = append(result, params.FilesystemParamsResult{
				Error: &params.Error{Message: "already provisioned"},
			})
		} else {
			filesystemParams := params.FilesystemParams{
				FilesystemTag: tag.String(),
				Size:          1024,
				Provider:      "dummy",
			}
			if _, ok := names.FilesystemMachine(tag); ok {
				// place all volume-backed filesystems on machine-scoped
				// volumes with the same ID as the filesystem.
				filesystemParams.VolumeTag = names.NewVolumeTag(tag.Id()).String()
			}
			result = append(result, params.FilesystemParamsResult{Result: filesystemParams})
		}
	}
	return result, nil
}

func (f *mockFilesystemAccessor) FilesystemAttachmentParams(ids []params.MachineStorageId) ([]params.FilesystemAttachmentParamsResult, error) {
	var result []params.FilesystemAttachmentParamsResult
	for _, id := range ids {
		if _, ok := f.provisionedAttachments[id]; ok {
			result = append(result, params.FilesystemAttachmentParamsResult{
				Error: &params.Error{Message: "already provisioned"},
			})
		} else {
			instanceId, _ := f.provisionedMachines[id.MachineTag]
			result = append(result, params.FilesystemAttachmentParamsResult{Result: params.FilesystemAttachmentParams{
				MachineTag:    id.MachineTag,
				FilesystemTag: id.AttachmentTag,
				InstanceId:    string(instanceId),
				Provider:      "dummy",
			}})
		}
	}
	return result, nil
}

func (f *mockFilesystemAccessor) SetFilesystemInfo(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
	return f.setFilesystemInfo(filesystems)
}

func (f *mockFilesystemAccessor) SetFilesystemAttachmentInfo(filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
	return f.setFilesystemAttachmentInfo(filesystemAttachments)
}

func newMockFilesystemAccessor() *mockFilesystemAccessor {
	return &mockFilesystemAccessor{
		filesystemsWatcher:     &mockStringsWatcher{make(chan []string, 1)},
		attachmentsWatcher:     &mockAttachmentsWatcher{make(chan []params.MachineStorageId, 1)},
		provisionedMachines:    make(map[string]instance.Id),
		provisionedFilesystems: make(map[string]params.Filesystem),
		provisionedAttachments: make(map[params.MachineStorageId]params.FilesystemAttachment),
	}
}

type mockLifecycleManager struct {
}

func (m *mockLifecycleManager) Life(volumes []names.Tag) ([]params.LifeResult, error) {
	var result []params.LifeResult
	for _, tag := range volumes {
		id, _ := strconv.Atoi(tag.Id())
		if id <= 100 {
			result = append(result, params.LifeResult{Life: params.Alive})
		} else {
			result = append(result, params.LifeResult{Life: params.Dying})
		}
	}
	return result, nil
}

func (m *mockLifecycleManager) AttachmentLife(ids []params.MachineStorageId) ([]params.LifeResult, error) {
	var result []params.LifeResult
	for _, id := range ids {
		switch id {
		case dyingVolumeAttachmentId:
			result = append(result, params.LifeResult{Life: params.Dying})
		case missingVolumeAttachmentId:
			result = append(result, params.LifeResult{
				Error: common.ServerError(errors.NotFoundf("attachment %v", id)),
			})
		default:
			result = append(result, params.LifeResult{Life: params.Alive})
		}
	}
	return result, nil
}

func (m *mockLifecycleManager) EnsureDead([]names.Tag) ([]params.ErrorResult, error) {
	return nil, nil
}

func (m *mockLifecycleManager) Remove([]names.Tag) ([]params.ErrorResult, error) {
	return nil, nil
}

func (m *mockLifecycleManager) RemoveAttachments([]params.MachineStorageId) ([]params.ErrorResult, error) {
	return nil, nil
}

// Set up a dummy storage provider so we can stub out volume creation.
type dummyProvider struct {
	storage.Provider
	dynamic bool

	volumeSourceFunc func(*config.Config, *storage.Config) (storage.VolumeSource, error)
}

type dummyVolumeSource struct {
	storage.VolumeSource
}

type dummyFilesystemSource struct {
	storage.FilesystemSource
}

func (p *dummyProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	if p.volumeSourceFunc != nil {
		return p.volumeSourceFunc(environConfig, providerConfig)
	}
	return &dummyVolumeSource{}, nil
}

func (*dummyProvider) FilesystemSource(environConfig *config.Config, providerConfig *storage.Config) (storage.FilesystemSource, error) {
	return &dummyFilesystemSource{}, nil
}

func (p *dummyProvider) Dynamic() bool {
	return p.dynamic
}

func (*dummyVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	if params.Tag.Id() == needsInstanceVolumeId {
		return storage.ErrVolumeNeedsInstance
	}
	return nil
}

// CreateVolumes makes some volumes that we can check later to ensure things went as expected.
func (*dummyVolumeSource) CreateVolumes(params []storage.VolumeParams) ([]storage.Volume, []storage.VolumeAttachment, error) {
	var volumes []storage.Volume
	var volumeAttachments []storage.VolumeAttachment
	for _, p := range params {
		persistent, _ := p.Attributes["persistent"].(bool)
		volumes = append(volumes, storage.Volume{
			Tag:        p.Tag,
			Size:       p.Size,
			HardwareId: "serial-" + p.Tag.Id(),
			VolumeId:   "id-" + p.Tag.Id(),
			Persistent: persistent,
		})
		if p.Attachment != nil {
			volumeAttachments = append(volumeAttachments, storage.VolumeAttachment{
				Volume:     p.Tag,
				Machine:    p.Attachment.Machine,
				DeviceName: "/dev/sda" + p.Tag.Id(),
			})
		}
	}
	return volumes, volumeAttachments, nil
}

// AttachVolumes attaches volumes to machines.
func (*dummyVolumeSource) AttachVolumes(params []storage.VolumeAttachmentParams) ([]storage.VolumeAttachment, error) {
	var volumeAttachments []storage.VolumeAttachment
	for _, p := range params {
		if p.VolumeId == "" {
			panic("AttachVolumes called with unprovisioned volume")
		}
		if p.InstanceId == "" {
			panic("AttachVolumes called with unprovisioned machine")
		}
		volumeAttachments = append(volumeAttachments, storage.VolumeAttachment{
			Volume:     p.Volume,
			Machine:    p.Machine,
			DeviceName: "/dev/sda" + p.Volume.Id(),
		})
	}
	return volumeAttachments, nil
}

func (*dummyFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	return nil
}

// CreateFilesystems makes some filesystems that we can check later to ensure things went as expected.
func (*dummyFilesystemSource) CreateFilesystems(params []storage.FilesystemParams) ([]storage.Filesystem, error) {
	var filesystems []storage.Filesystem
	for _, p := range params {
		filesystems = append(filesystems, storage.Filesystem{
			Tag:          p.Tag,
			Size:         p.Size,
			FilesystemId: "id-" + p.Tag.Id(),
		})
	}
	return filesystems, nil
}

// AttachFilesystems attaches filesystems to machines.
func (*dummyFilesystemSource) AttachFilesystems(params []storage.FilesystemAttachmentParams) ([]storage.FilesystemAttachment, error) {
	var filesystemAttachments []storage.FilesystemAttachment
	for _, p := range params {
		if p.FilesystemId == "" {
			panic("AttachFilesystems called with unprovisioned filesystem")
		}
		if p.InstanceId == "" {
			panic("AttachFilesystems called with unprovisioned machine")
		}
		filesystemAttachments = append(filesystemAttachments, storage.FilesystemAttachment{
			Filesystem: p.Filesystem,
			Machine:    p.Machine,
			Path:       "/srv/" + p.FilesystemId,
		})
	}
	return filesystemAttachments, nil
}

type mockManagedFilesystemSource struct {
	blockDevices map[names.VolumeTag]storage.BlockDevice
	filesystems  map[names.FilesystemTag]storage.Filesystem
}

func (s *mockManagedFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	return nil
}

func (s *mockManagedFilesystemSource) CreateFilesystems(args []storage.FilesystemParams) ([]storage.Filesystem, error) {
	var filesystems []storage.Filesystem
	for _, arg := range args {
		blockDevice, ok := s.blockDevices[arg.Volume]
		if !ok {
			return nil, errors.Errorf("filesystem %v's backing-volume is not attached", arg.Tag.Id())
		}
		filesystems = append(filesystems, storage.Filesystem{
			Tag:          arg.Tag,
			Size:         blockDevice.Size,
			FilesystemId: blockDevice.DeviceName,
		})
	}
	return filesystems, nil
}

func (s *mockManagedFilesystemSource) AttachFilesystems(args []storage.FilesystemAttachmentParams) ([]storage.FilesystemAttachment, error) {
	var filesystemAttachments []storage.FilesystemAttachment
	for _, arg := range args {
		if arg.FilesystemId == "" {
			panic("AttachFilesystems called with unprovisioned filesystem")
		}
		if arg.InstanceId == "" {
			panic("AttachFilesystems called with unprovisioned machine")
		}
		filesystem, ok := s.filesystems[arg.Filesystem]
		if !ok {
			return nil, errors.Errorf("filesystem %v has not been created", arg.Filesystem.Id())
		}
		blockDevice, ok := s.blockDevices[filesystem.Volume]
		if !ok {
			return nil, errors.Errorf("filesystem %v's backing-volume is not attached", filesystem.Tag.Id())
		}
		filesystemAttachments = append(filesystemAttachments, storage.FilesystemAttachment{
			Filesystem: arg.Filesystem,
			Machine:    arg.Machine,
			Path:       "/mnt/" + blockDevice.DeviceName,
		})
	}
	return filesystemAttachments, nil
}

func (s *mockManagedFilesystemSource) DetachFilesystems(params []storage.FilesystemAttachmentParams) error {
	return errors.NotImplementedf("DetachFilesystems")
}
