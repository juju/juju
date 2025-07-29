// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

const needsInstanceVolumeId = "23"
const noAttachmentVolumeId = "66"

var (
	releasingVolumeId     = "2"
	releasingFilesystemId = "2"
)

var dyingVolumeAttachmentId = params.MachineStorageId{
	MachineTag:    "machine-0",
	AttachmentTag: "volume-0",
}

var dyingUnitVolumeAttachmentId = params.MachineStorageId{
	MachineTag:    "unit-mariadb-0",
	AttachmentTag: "volume-0",
}

var dyingFilesystemAttachmentId = params.MachineStorageId{
	MachineTag:    "machine-0",
	AttachmentTag: "filesystem-0",
}

var dyingUnitFilesystemAttachmentId = params.MachineStorageId{
	MachineTag:    "unit-mariadb-0",
	AttachmentTag: "filesystem-0",
}

var missingVolumeAttachmentId = params.MachineStorageId{
	MachineTag:    "machine-3",
	AttachmentTag: "volume-1",
}

type mockWatcher struct{}

func (mockWatcher) Kill()       {}
func (mockWatcher) Wait() error { return nil }

func newMockNotifyWatcher() *mockNotifyWatcher {
	return &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
}

type mockNotifyWatcher struct {
	mockWatcher
	changes chan struct{}
}

func (w *mockNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.changes
}

func newMockStringsWatcher() *mockStringsWatcher {
	return &mockStringsWatcher{
		changes: make(chan []string, 1),
	}
}

type mockStringsWatcher struct {
	mockWatcher
	changes chan []string
}

func (w *mockStringsWatcher) Changes() watcher.StringsChannel {
	return w.changes
}

func newMockAttachmentsWatcher() *mockAttachmentsWatcher {
	return &mockAttachmentsWatcher{
		changes: make(chan []watcher.MachineStorageID, 1),
	}
}

type mockAttachmentsWatcher struct {
	mockWatcher
	changes chan []watcher.MachineStorageID
}

func (w *mockAttachmentsWatcher) Changes() watcher.MachineStorageIDsChannel {
	return w.changes
}

func newMockAttachmentPlansWatcher() *mockAttachmentPlansWatcher {
	return &mockAttachmentPlansWatcher{
		changes: make(chan []watcher.MachineStorageID, 1),
	}
}

type mockAttachmentPlansWatcher struct {
	mockWatcher
	changes chan []watcher.MachineStorageID
}

func (w *mockAttachmentPlansWatcher) Changes() watcher.MachineStorageIDsChannel {
	return w.changes
}

type mockVolumeAccessor struct {
	volumesWatcher         *mockStringsWatcher
	attachmentsWatcher     *mockAttachmentsWatcher
	attachmentPlansWatcher *mockAttachmentPlansWatcher
	blockDevicesWatcher    *mockNotifyWatcher
	provisionedMachines    map[string]instance.Id
	provisionedVolumes     map[string]params.Volume
	provisionedAttachments map[params.MachineStorageId]params.VolumeAttachment
	blockDevices           map[params.MachineStorageId]params.BlockDevice

	setVolumeInfo               func([]params.Volume) ([]params.ErrorResult, error)
	setVolumeAttachmentInfo     func([]params.VolumeAttachment) ([]params.ErrorResult, error)
	createVolumeAttachmentPlans func([]params.VolumeAttachmentPlan) ([]params.ErrorResult, error)
}

func (m *mockVolumeAccessor) provisionVolume(tag names.VolumeTag) params.Volume {
	v := params.Volume{
		VolumeTag: tag.String(),
		Info: params.VolumeInfo{
			ProviderId: "vol-" + tag.Id(),
		},
	}
	m.provisionedVolumes[tag.String()] = v
	return v
}

func (w *mockVolumeAccessor) WatchVolumes(context.Context, names.Tag) (watcher.StringsWatcher, error) {
	return w.volumesWatcher, nil
}

func (w *mockVolumeAccessor) WatchVolumeAttachments(context.Context, names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	return w.attachmentsWatcher, nil
}

func (w *mockVolumeAccessor) WatchBlockDevices(_ context.Context, tag names.MachineTag) (watcher.NotifyWatcher, error) {
	return w.blockDevicesWatcher, nil
}

func (w *mockVolumeAccessor) WatchVolumeAttachmentPlans(context.Context, names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	return w.attachmentPlansWatcher, nil
}

func (v *mockVolumeAccessor) Volumes(_ context.Context, volumes []names.VolumeTag) ([]params.VolumeResult, error) {
	var result []params.VolumeResult
	for _, tag := range volumes {
		if vol, ok := v.provisionedVolumes[tag.String()]; ok {
			result = append(result, params.VolumeResult{Result: vol})
		} else {
			result = append(result, params.VolumeResult{
				Error: apiservererrors.ServerError(errors.NotProvisionedf("volume %q", tag.Id())),
			})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeAttachments(_ context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentResult, error) {
	var result []params.VolumeAttachmentResult
	for _, id := range ids {
		if att, ok := v.provisionedAttachments[id]; ok {
			result = append(result, params.VolumeAttachmentResult{Result: att})
		} else {
			result = append(result, params.VolumeAttachmentResult{
				Error: apiservererrors.ServerError(errors.NotProvisionedf("volume attachment %v", id)),
			})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeBlockDevices(_ context.Context, ids []params.MachineStorageId) ([]params.BlockDeviceResult, error) {
	var result []params.BlockDeviceResult
	for _, id := range ids {
		if dev, ok := v.blockDevices[id]; ok {
			result = append(result, params.BlockDeviceResult{Result: dev})
		} else {
			result = append(result, params.BlockDeviceResult{
				Error: apiservererrors.ServerError(errors.NotFoundf("block device for volume attachment %v", id)),
			})
		}
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeParams(_ context.Context, volumes []names.VolumeTag) ([]params.VolumeParamsResult, error) {
	var result []params.VolumeParamsResult
	for _, tag := range volumes {
		volumeParams := params.VolumeParams{
			VolumeTag: tag.String(),
			Size:      1024,
			Provider:  "dummy",
			Attributes: map[string]interface{}{
				"persistent": tag.String() == "volume-1",
			},
			Tags: map[string]string{
				"very": "fancy",
			},
		}
		if tag.Id() != noAttachmentVolumeId {
			volumeParams.Attachment = &params.VolumeAttachmentParams{
				VolumeTag:  tag.String(),
				MachineTag: "machine-1",
				InstanceId: string(v.provisionedMachines["machine-1"]),
				Provider:   "dummy",
				ReadOnly:   tag.String() == "volume-1",
			}
		}
		result = append(result, params.VolumeParamsResult{Result: volumeParams})
	}
	return result, nil
}

func (v *mockVolumeAccessor) RemoveVolumeParams(_ context.Context, volumes []names.VolumeTag) ([]params.RemoveVolumeParamsResult, error) {
	var result []params.RemoveVolumeParamsResult
	for _, tag := range volumes {
		v, ok := v.provisionedVolumes[tag.String()]
		if !ok {
			result = append(result, params.RemoveVolumeParamsResult{
				Error: &params.Error{Code: params.CodeNotProvisioned},
			})
			continue
		}
		volumeParams := params.RemoveVolumeParams{
			Provider:   "dummy",
			ProviderId: v.Info.ProviderId,
			Destroy:    tag.Id() != releasingVolumeId,
		}
		result = append(result, params.RemoveVolumeParamsResult{Result: volumeParams})
	}
	return result, nil
}

func (v *mockVolumeAccessor) VolumeAttachmentParams(_ context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error) {
	var result []params.VolumeAttachmentParamsResult
	for _, id := range ids {
		// Parameters are returned regardless of whether the attachment
		// exists; this is to support reattachment.
		instanceId, _ := v.provisionedMachines[id.MachineTag]
		result = append(result, params.VolumeAttachmentParamsResult{Result: params.VolumeAttachmentParams{
			MachineTag: id.MachineTag,
			VolumeTag:  id.AttachmentTag,
			InstanceId: string(instanceId),
			Provider:   "dummy",
			ReadOnly:   id.AttachmentTag == "volume-1",
		}})
	}
	return result, nil
}

func (v *mockVolumeAccessor) SetVolumeInfo(_ context.Context, volumes []params.Volume) ([]params.ErrorResult, error) {
	if v.setVolumeInfo != nil {
		return v.setVolumeInfo(volumes)
	}
	return make([]params.ErrorResult, len(volumes)), nil
}

func (v *mockVolumeAccessor) SetVolumeAttachmentInfo(_ context.Context, volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
	if v.setVolumeAttachmentInfo != nil {
		return v.setVolumeAttachmentInfo(volumeAttachments)
	}
	return make([]params.ErrorResult, len(volumeAttachments)), nil
}

func (v *mockVolumeAccessor) CreateVolumeAttachmentPlans(_ context.Context, volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	if v.createVolumeAttachmentPlans != nil {
		return v.createVolumeAttachmentPlans(volumeAttachmentPlans)
	}
	return make([]params.ErrorResult, len(volumeAttachmentPlans)), nil
}

func (v *mockVolumeAccessor) RemoveVolumeAttachmentPlan(_ context.Context, machineIds []params.MachineStorageId) ([]params.ErrorResult, error) {
	return make([]params.ErrorResult, len(machineIds)), nil
}

func (v *mockVolumeAccessor) SetVolumeAttachmentPlanBlockInfo(_ context.Context, volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	return make([]params.ErrorResult, len(volumeAttachmentPlans)), nil
}

func (v *mockVolumeAccessor) VolumeAttachmentPlans(context.Context, []params.MachineStorageId) ([]params.VolumeAttachmentPlanResult, error) {
	return []params.VolumeAttachmentPlanResult{}, nil
}

func newMockVolumeAccessor() *mockVolumeAccessor {
	return &mockVolumeAccessor{
		volumesWatcher:         newMockStringsWatcher(),
		attachmentsWatcher:     newMockAttachmentsWatcher(),
		attachmentPlansWatcher: newMockAttachmentPlansWatcher(),
		blockDevicesWatcher:    newMockNotifyWatcher(),
		provisionedMachines:    make(map[string]instance.Id),
		provisionedVolumes:     make(map[string]params.Volume),
		provisionedAttachments: make(map[params.MachineStorageId]params.VolumeAttachment),
		blockDevices:           make(map[params.MachineStorageId]params.BlockDevice),
	}
}

type mockFilesystemAccessor struct {
	testhelpers.Stub
	filesystemsWatcher             *mockStringsWatcher
	attachmentsWatcher             *mockAttachmentsWatcher
	provisionedMachines            map[string]instance.Id
	provisionedMachinesFilesystems map[string]params.Filesystem
	provisionedFilesystems         map[string]params.Filesystem
	provisionedAttachments         map[params.MachineStorageId]params.FilesystemAttachment

	setFilesystemInfo           func([]params.Filesystem) ([]params.ErrorResult, error)
	setFilesystemAttachmentInfo func([]params.FilesystemAttachment) ([]params.ErrorResult, error)
}

func (m *mockFilesystemAccessor) provisionFilesystem(tag names.FilesystemTag) params.Filesystem {
	f := params.Filesystem{
		FilesystemTag: tag.String(),
		Info: params.FilesystemInfo{
			ProviderId: "fs-" + tag.Id(),
		},
	}
	m.provisionedFilesystems[tag.String()] = f
	return f
}

func (w *mockFilesystemAccessor) WatchFilesystems(_ context.Context, tag names.Tag) (watcher.StringsWatcher, error) {
	w.AddCall("WatchFilesystems", tag)
	return w.filesystemsWatcher, nil
}

func (w *mockFilesystemAccessor) WatchFilesystemAttachments(context.Context, names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	return w.attachmentsWatcher, nil
}

func (v *mockFilesystemAccessor) Filesystems(_ context.Context, filesystems []names.FilesystemTag) ([]params.FilesystemResult, error) {
	var result []params.FilesystemResult
	for _, tag := range filesystems {
		if vol, ok := v.provisionedFilesystems[tag.String()]; ok {
			result = append(result, params.FilesystemResult{Result: vol})
		} else {
			result = append(result, params.FilesystemResult{
				Error: apiservererrors.ServerError(errors.NotProvisionedf("filesystem %q", tag.Id())),
			})
		}
	}
	return result, nil
}

func (v *mockFilesystemAccessor) FilesystemAttachments(_ context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentResult, error) {
	var result []params.FilesystemAttachmentResult
	for _, id := range ids {
		if att, ok := v.provisionedAttachments[id]; ok {
			result = append(result, params.FilesystemAttachmentResult{Result: att})
		} else {
			result = append(result, params.FilesystemAttachmentResult{
				Error: apiservererrors.ServerError(errors.NotProvisionedf("filesystem attachment %v", id)),
			})
		}
	}
	return result, nil
}

func (v *mockFilesystemAccessor) FilesystemParams(_ context.Context, filesystems []names.FilesystemTag) ([]params.FilesystemParamsResult, error) {
	results := make([]params.FilesystemParamsResult, len(filesystems))
	for i, tag := range filesystems {
		filesystemParams := params.FilesystemParams{
			FilesystemTag: tag.String(),
			Size:          1024,
			Provider:      "dummy",
			Tags: map[string]string{
				"very": "fancy",
			},
		}
		if _, ok := names.FilesystemMachine(tag); ok {
			// place all volume-backed filesystems on machine-scoped
			// volumes with the same ID as the filesystem.
			filesystemParams.VolumeTag = names.NewVolumeTag(tag.Id()).String()
		}
		results[i] = params.FilesystemParamsResult{Result: filesystemParams}
	}
	return results, nil
}

func (v *mockFilesystemAccessor) RemoveFilesystemParams(_ context.Context, filesystems []names.FilesystemTag) ([]params.RemoveFilesystemParamsResult, error) {
	results := make([]params.RemoveFilesystemParamsResult, len(filesystems))
	for i, tag := range filesystems {
		f, ok := v.provisionedFilesystems[tag.String()]
		if !ok {
			results = append(results, params.RemoveFilesystemParamsResult{
				Error: &params.Error{Code: params.CodeNotProvisioned},
			})
			continue
		}
		filesystemParams := params.RemoveFilesystemParams{
			Provider:   "dummy",
			ProviderId: f.Info.ProviderId,
			Destroy:    tag.Id() != releasingFilesystemId,
		}
		results[i] = params.RemoveFilesystemParamsResult{Result: filesystemParams}
	}
	return results, nil
}

func (f *mockFilesystemAccessor) FilesystemAttachmentParams(_ context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentParamsResult, error) {
	var result []params.FilesystemAttachmentParamsResult
	for _, id := range ids {
		// Parameters are returned regardless of whether the attachment
		// exists; this is to support reattachment.
		instanceId := f.provisionedMachines[id.MachineTag]
		filesystemId := f.provisionedMachinesFilesystems[id.AttachmentTag].Info.ProviderId
		result = append(result, params.FilesystemAttachmentParamsResult{Result: params.FilesystemAttachmentParams{
			MachineTag:    id.MachineTag,
			ProviderId:    filesystemId,
			FilesystemTag: id.AttachmentTag,
			InstanceId:    string(instanceId),
			Provider:      "dummy",
			ReadOnly:      true,
		}})
	}
	return result, nil
}

func (f *mockFilesystemAccessor) SetFilesystemInfo(_ context.Context, filesystems []params.Filesystem) ([]params.ErrorResult, error) {
	if f.setFilesystemInfo != nil {
		return f.setFilesystemInfo(filesystems)
	}
	return make([]params.ErrorResult, len(filesystems)), nil
}

func (f *mockFilesystemAccessor) SetFilesystemAttachmentInfo(_ context.Context, filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
	if f.setFilesystemAttachmentInfo != nil {
		return f.setFilesystemAttachmentInfo(filesystemAttachments)
	}
	return make([]params.ErrorResult, len(filesystemAttachments)), nil
}

func newMockFilesystemAccessor() *mockFilesystemAccessor {
	return &mockFilesystemAccessor{
		filesystemsWatcher:             newMockStringsWatcher(),
		attachmentsWatcher:             newMockAttachmentsWatcher(),
		provisionedMachines:            make(map[string]instance.Id),
		provisionedFilesystems:         make(map[string]params.Filesystem),
		provisionedMachinesFilesystems: make(map[string]params.Filesystem),
		provisionedAttachments:         make(map[params.MachineStorageId]params.FilesystemAttachment),
	}
}

type mockLifecycleManager struct {
	err               *params.Error
	life              func([]names.Tag) ([]params.LifeResult, error)
	attachmentLife    func(ids []params.MachineStorageId) ([]params.LifeResult, error)
	removeAttachments func([]params.MachineStorageId) ([]params.ErrorResult, error)
	remove            func([]names.Tag) ([]params.ErrorResult, error)
}

func (m *mockLifecycleManager) Life(ctx context.Context, tags []names.Tag) ([]params.LifeResult, error) {
	if m.life != nil {
		return m.life(tags)
	}
	var result []params.LifeResult
	for _, tag := range tags {
		if m.err != nil {
			result = append(result, params.LifeResult{Error: m.err})
			continue
		}
		value := life.Alive
		if tag.Kind() == names.ApplicationTagKind {
			if tag.Id() == "mysql" {
				value = life.Dying
			} else if tag.Id() == "postgresql" {
				value = life.Dead
			}
		}
		result = append(result, params.LifeResult{Life: value})
	}
	return result, nil
}

func (m *mockLifecycleManager) AttachmentLife(_ context.Context, ids []params.MachineStorageId) ([]params.LifeResult, error) {
	if m.attachmentLife != nil {
		return m.attachmentLife(ids)
	}
	var result []params.LifeResult
	for _, id := range ids {
		switch id {
		case dyingVolumeAttachmentId, dyingUnitVolumeAttachmentId,
			dyingFilesystemAttachmentId, dyingUnitFilesystemAttachmentId:
			result = append(result, params.LifeResult{Life: life.Dying})
		case missingVolumeAttachmentId:
			result = append(result, params.LifeResult{
				Error: apiservererrors.ServerError(errors.NotFoundf("attachment %v", id)),
			})
		default:
			result = append(result, params.LifeResult{Life: life.Alive})
		}
	}
	return result, nil
}

func (m *mockLifecycleManager) Remove(_ context.Context, tags []names.Tag) ([]params.ErrorResult, error) {
	if m.remove != nil {
		return m.remove(tags)
	}
	return make([]params.ErrorResult, len(tags)), nil
}

func (m *mockLifecycleManager) RemoveAttachments(_ context.Context, ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	if m.removeAttachments != nil {
		return m.removeAttachments(ids)
	}
	return make([]params.ErrorResult, len(ids)), nil
}

// Set up a dummy storage provider so we can stub out volume creation.
type dummyProvider struct {
	storage.Provider
	dynamic bool

	volumeSourceFunc             func(*storage.Config) (storage.VolumeSource, error)
	filesystemSourceFunc         func(*storage.Config) (storage.FilesystemSource, error)
	createVolumesFunc            func([]storage.VolumeParams) ([]storage.CreateVolumesResult, error)
	createFilesystemsFunc        func([]storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error)
	attachVolumesFunc            func([]storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error)
	attachFilesystemsFunc        func([]storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error)
	detachVolumesFunc            func([]storage.VolumeAttachmentParams) ([]error, error)
	detachFilesystemsFunc        func([]storage.FilesystemAttachmentParams) ([]error, error)
	destroyVolumesFunc           func([]string) ([]error, error)
	releaseVolumesFunc           func([]string) ([]error, error)
	destroyFilesystemsFunc       func([]string) ([]error, error)
	releaseFilesystemsFunc       func([]string) ([]error, error)
	validateVolumeParamsFunc     func(storage.VolumeParams) error
	validateFilesystemParamsFunc func(storage.FilesystemParams) error
}

type dummyVolumeSource struct {
	storage.VolumeSource
	provider          *dummyProvider
	createVolumesArgs [][]storage.VolumeParams
}

type dummyFilesystemSource struct {
	storage.FilesystemSource
	provider              *dummyProvider
	createFilesystemsArgs [][]storage.FilesystemParams
}

func (p *dummyProvider) VolumeSource(providerConfig *storage.Config) (storage.VolumeSource, error) {
	if p.volumeSourceFunc != nil {
		return p.volumeSourceFunc(providerConfig)
	}
	return &dummyVolumeSource{provider: p}, nil
}

func (p *dummyProvider) FilesystemSource(providerConfig *storage.Config) (storage.FilesystemSource, error) {
	if p.filesystemSourceFunc != nil {
		return p.filesystemSourceFunc(providerConfig)
	}
	return &dummyFilesystemSource{provider: p}, nil
}

func (p *dummyProvider) Dynamic() bool {
	return p.dynamic
}

func (s *dummyVolumeSource) ValidateVolumeParams(params storage.VolumeParams) error {
	if s.provider != nil && s.provider.validateVolumeParamsFunc != nil {
		return s.provider.validateVolumeParamsFunc(params)
	}
	return nil
}

// CreateVolumes makes some volumes that we can check later to ensure things went as expected.
func (s *dummyVolumeSource) CreateVolumes(ctx context.Context, params []storage.VolumeParams) ([]storage.CreateVolumesResult, error) {
	if s.provider != nil && s.provider.createVolumesFunc != nil {
		return s.provider.createVolumesFunc(params)
	}

	paramsCopy := make([]storage.VolumeParams, len(params))
	copy(paramsCopy, params)
	s.createVolumesArgs = append(s.createVolumesArgs, paramsCopy)

	results := make([]storage.CreateVolumesResult, len(params))
	for i, p := range params {
		persistent, _ := p.Attributes["persistent"].(bool)
		results[i].Volume = &storage.Volume{
			Tag: p.Tag,
			VolumeInfo: storage.VolumeInfo{
				Size:       p.Size,
				HardwareId: "serial-" + p.Tag.Id(),
				VolumeId:   "id-" + p.Tag.Id(),
				Persistent: persistent,
			},
		}
	}
	return results, nil
}

// DestroyVolumes destroys volumes.
func (s *dummyVolumeSource) DestroyVolumes(ctx context.Context, volumeIds []string) ([]error, error) {
	if s.provider.destroyVolumesFunc != nil {
		return s.provider.destroyVolumesFunc(volumeIds)
	}
	return make([]error, len(volumeIds)), nil
}

// ReleaseVolumes destroys volumes.
func (s *dummyVolumeSource) ReleaseVolumes(ctx context.Context, volumeIds []string) ([]error, error) {
	if s.provider.releaseVolumesFunc != nil {
		return s.provider.releaseVolumesFunc(volumeIds)
	}
	return make([]error, len(volumeIds)), nil
}

// AttachVolumes attaches volumes to machines.
func (s *dummyVolumeSource) AttachVolumes(ctx context.Context, params []storage.VolumeAttachmentParams) ([]storage.AttachVolumesResult, error) {
	if s.provider != nil && s.provider.attachVolumesFunc != nil {
		return s.provider.attachVolumesFunc(params)
	}

	results := make([]storage.AttachVolumesResult, len(params))
	for i, p := range params {
		if p.VolumeId == "" {
			panic("AttachVolumes called with unprovisioned volume")
		}
		if p.InstanceId == "" {
			panic("AttachVolumes called with unprovisioned machine")
		}
		results[i].VolumeAttachment = &storage.VolumeAttachment{
			Volume:  p.Volume,
			Machine: p.Machine,
			VolumeAttachmentInfo: storage.VolumeAttachmentInfo{
				DeviceName: "/dev/sda" + p.Volume.Id(),
				ReadOnly:   p.ReadOnly,
			},
		}
	}
	return results, nil
}

// DetachVolumes detaches volumes from machines.
func (s *dummyVolumeSource) DetachVolumes(ctx context.Context, params []storage.VolumeAttachmentParams) ([]error, error) {
	if s.provider.detachVolumesFunc != nil {
		return s.provider.detachVolumesFunc(params)
	}
	return make([]error, len(params)), nil
}

func (s *dummyFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	if s.provider != nil && s.provider.validateFilesystemParamsFunc != nil {
		return s.provider.validateFilesystemParamsFunc(params)
	}
	return nil
}

// CreateFilesystems makes some filesystems that we can check later to ensure things went as expected.
func (s *dummyFilesystemSource) CreateFilesystems(ctx context.Context, params []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
	if s.provider != nil && s.provider.createFilesystemsFunc != nil {
		return s.provider.createFilesystemsFunc(params)
	}

	paramsCopy := make([]storage.FilesystemParams, len(params))
	copy(paramsCopy, params)
	s.createFilesystemsArgs = append(s.createFilesystemsArgs, paramsCopy)

	results := make([]storage.CreateFilesystemsResult, len(params))
	for i, p := range params {
		results[i].Filesystem = &storage.Filesystem{
			Tag: p.Tag,
			FilesystemInfo: storage.FilesystemInfo{
				Size:       p.Size,
				ProviderId: "id-" + p.Tag.Id(),
			},
		}
	}
	return results, nil
}

// DestroyFilesystems destroys filesystems.
func (s *dummyFilesystemSource) DestroyFilesystems(ctx context.Context, filesystemIds []string) ([]error, error) {
	if s.provider.destroyFilesystemsFunc != nil {
		return s.provider.destroyFilesystemsFunc(filesystemIds)
	}
	return make([]error, len(filesystemIds)), nil
}

// ReleaseFilesystems destroys filesystems.
func (s *dummyFilesystemSource) ReleaseFilesystems(ctx context.Context, filesystemIds []string) ([]error, error) {
	if s.provider.releaseFilesystemsFunc != nil {
		return s.provider.releaseFilesystemsFunc(filesystemIds)
	}
	return make([]error, len(filesystemIds)), nil
}

// AttachFilesystems attaches filesystems to machines.
func (s *dummyFilesystemSource) AttachFilesystems(ctx context.Context, params []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
	if s.provider != nil && s.provider.attachFilesystemsFunc != nil {
		return s.provider.attachFilesystemsFunc(params)
	}

	results := make([]storage.AttachFilesystemsResult, len(params))
	for i, p := range params {
		if p.ProviderId == "" {
			panic("AttachFilesystems called with unprovisioned filesystem")
		}
		if p.InstanceId == "" {
			panic("AttachFilesystems called with unprovisioned machine")
		}
		results[i].FilesystemAttachment = &storage.FilesystemAttachment{
			Filesystem: p.Filesystem,
			Machine:    p.Machine,
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path: "/srv/" + p.ProviderId,
			},
		}
	}
	return results, nil
}

// DetachFilesystems detaches filesystems from machines.
func (s *dummyFilesystemSource) DetachFilesystems(ctx context.Context, params []storage.FilesystemAttachmentParams) ([]error, error) {
	if s.provider.detachFilesystemsFunc != nil {
		return s.provider.detachFilesystemsFunc(params)
	}
	return make([]error, len(params)), nil
}

type mockManagedFilesystemSource struct {
	blockDevices        map[names.VolumeTag]blockdevice.BlockDevice
	filesystems         map[names.FilesystemTag]storage.Filesystem
	attachedFilesystems chan interface{}
}

func (s *mockManagedFilesystemSource) ValidateFilesystemParams(params storage.FilesystemParams) error {
	return nil
}

func (s *mockManagedFilesystemSource) CreateFilesystems(ctx context.Context, args []storage.FilesystemParams) ([]storage.CreateFilesystemsResult, error) {
	results := make([]storage.CreateFilesystemsResult, len(args))
	for i, arg := range args {
		blockDevice, ok := s.blockDevices[arg.Volume]
		if !ok {
			results[i].Error = errors.Errorf("filesystem %v's backing-volume is not attached", arg.Tag.Id())
			continue
		}
		results[i].Filesystem = &storage.Filesystem{
			Tag: arg.Tag,
			FilesystemInfo: storage.FilesystemInfo{
				Size:       blockDevice.SizeMiB,
				ProviderId: blockDevice.DeviceName,
			},
		}
	}
	return results, nil
}

func (s *mockManagedFilesystemSource) DestroyFilesystems(ctx context.Context, filesystemIds []string) ([]error, error) {
	return make([]error, len(filesystemIds)), nil
}

func (s *mockManagedFilesystemSource) ReleaseFilesystems(ctx context.Context, filesystemIds []string) ([]error, error) {
	return make([]error, len(filesystemIds)), nil
}

func (s *mockManagedFilesystemSource) AttachFilesystems(ctx context.Context, args []storage.FilesystemAttachmentParams) ([]storage.AttachFilesystemsResult, error) {
	results := make([]storage.AttachFilesystemsResult, len(args))
	for i, arg := range args {
		filesystem, ok := s.filesystems[arg.Filesystem]
		if !ok {
			results[i].Error = errors.Errorf("filesystem %v has not been created", arg.Filesystem.Id())
			continue
		}
		blockDevice, ok := s.blockDevices[filesystem.Volume]
		if !ok {
			results[i].Error = errors.Errorf("filesystem %v's backing-volume is not attached", filesystem.Tag.Id())
			continue
		}
		results[i].FilesystemAttachment = &storage.FilesystemAttachment{
			Filesystem: arg.Filesystem,
			Machine:    arg.Machine,
			FilesystemAttachmentInfo: storage.FilesystemAttachmentInfo{
				Path:     "/mnt/" + blockDevice.DeviceName,
				ReadOnly: arg.ReadOnly,
			},
		}
	}
	if s.attachedFilesystems != nil {
		s.attachedFilesystems <- results
	}
	return results, nil
}

func (s *mockManagedFilesystemSource) DetachFilesystems(ctx context.Context, params []storage.FilesystemAttachmentParams) ([]error, error) {
	return nil, errors.NotImplementedf("DetachFilesystems")
}

type mockMachineAccessor struct {
	instanceIds map[names.MachineTag]instance.Id
	watcher     *mockNotifyWatcher
}

func (a *mockMachineAccessor) WatchMachine(context.Context, names.MachineTag) (watcher.NotifyWatcher, error) {
	return a.watcher, nil
}

func (a *mockMachineAccessor) InstanceIds(_ context.Context, tags []names.MachineTag) ([]params.StringResult, error) {
	results := make([]params.StringResult, len(tags))
	for i, tag := range tags {
		instanceId, ok := a.instanceIds[tag]
		if !ok {
			results[i].Error = &params.Error{Code: params.CodeNotFound}
		} else if instanceId == "" {
			results[i].Error = &params.Error{Code: params.CodeNotProvisioned}
		} else {
			results[i].Result = string(instanceId)
		}
	}
	return results, nil
}

func newMockMachineAccessor(c *tc.C) *mockMachineAccessor {
	return &mockMachineAccessor{
		instanceIds: make(map[names.MachineTag]instance.Id),
		watcher:     newMockNotifyWatcher(),
	}
}

type mockClock struct {
	clock.Clock
	testhelpers.Stub
	now         time.Time
	onNow       func() time.Time
	onAfter     func(time.Duration) <-chan time.Time
	onAfterFunc func(time.Duration, func()) clock.Timer
}

func (c *mockClock) Now() time.Time {
	c.MethodCall(c, "Now")
	if c.onNow != nil {
		return c.onNow()
	}
	return c.now
}

func (c *mockClock) After(d time.Duration) <-chan time.Time {
	c.MethodCall(c, "After", d)
	if c.onAfter != nil {
		return c.onAfter(d)
	}
	if d > 0 {
		c.now = c.now.Add(d)
	}
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func (c *mockClock) NewTimer(d time.Duration) clock.Timer {
	return mockTimer{time.NewTimer(0)}
}

func (c *mockClock) AfterFunc(d time.Duration, f func()) clock.Timer {
	c.MethodCall(c, "AfterFunc", d, f)
	if c.onAfterFunc != nil {
		return c.onAfterFunc(d, f)
	}
	if d > 0 {
		c.now = c.now.Add(d)
	}
	return mockTimer{time.AfterFunc(0, f)}
}

type mockTimer struct {
	*time.Timer
}

func (t mockTimer) Chan() <-chan time.Time {
	return t.C
}

type mockStatusSetter struct {
	args      []params.EntityStatusArgs
	setStatus func([]params.EntityStatusArgs) error
}

func (m *mockStatusSetter) SetStatus(ctx context.Context, args []params.EntityStatusArgs) error {
	if m.setStatus != nil {
		return m.setStatus(args)
	}
	m.args = append(m.args, args...)
	return nil
}
