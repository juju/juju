// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/names"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/worker/storageprovisioner"
)

const attachedVolumeId = "1"

var dyingVolumeAttachmentId = params.MachineStorageId{
	MachineTag:    "machine-0",
	AttachmentTag: "volume-0",
}

var missingVolumeAttachmentId = params.MachineStorageId{
	MachineTag:    "machine-3",
	AttachmentTag: "volume-1",
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

type mockVolumeAccessor struct {
	volumesWatcher         *mockStringsWatcher
	attachmentsWatcher     *mockAttachmentsWatcher
	provisionedMachines    map[string]instance.Id
	provisionedVolumes     map[string]params.Volume
	provisionedAttachments map[params.MachineStorageId]params.VolumeAttachment

	setVolumeInfo           func([]params.Volume) ([]params.ErrorResult, error)
	setVolumeAttachmentInfo func([]params.VolumeAttachment) ([]params.ErrorResult, error)
}

func (w *mockVolumeAccessor) WatchVolumes() (apiwatcher.StringsWatcher, error) {
	return w.volumesWatcher, nil
}

func (w *mockVolumeAccessor) WatchVolumeAttachments() (apiwatcher.MachineStorageIdsWatcher, error) {
	return w.attachmentsWatcher, nil
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
			volume, _ := v.provisionedVolumes[id.AttachmentTag]
			result = append(result, params.VolumeAttachmentParamsResult{Result: params.VolumeAttachmentParams{
				MachineTag: id.MachineTag,
				VolumeTag:  id.AttachmentTag,
				InstanceId: string(instanceId),
				VolumeId:   volume.VolumeId,
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
		provisionedMachines:    make(map[string]instance.Id),
		provisionedVolumes:     make(map[string]params.Volume),
		provisionedAttachments: make(map[params.MachineStorageId]params.VolumeAttachment),
	}
}

type mockFilesystemAccessor struct {
	storageprovisioner.FilesystemAccessor
}

func newMockFilesystemAccessor() *mockFilesystemAccessor {
	return &mockFilesystemAccessor{}
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
}

type dummyVolumeSource struct {
	storage.VolumeSource
}

func (*dummyProvider) VolumeSource(environConfig *config.Config, providerConfig *storage.Config) (storage.VolumeSource, error) {
	return &dummyVolumeSource{}, nil
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
			Serial:     "serial-" + p.Tag.Id(),
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
