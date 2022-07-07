// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

const storageProvisionerFacade = "StorageProvisioner"

// State provides access to a storageprovisioner's view of the state.
type State struct {
	facade base.FacadeCaller
}

// NewState creates a new client-side StorageProvisioner facade.
func NewState(caller base.APICaller) (*State, error) {
	facadeCaller := base.NewFacadeCaller(caller, storageProvisionerFacade)
	return &State{facadeCaller}, nil
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (st *State) WatchApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := st.facade.FacadeCall("WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchBlockDevices watches for changes to the specified machine's block devices.
func (st *State) WatchBlockDevices(m names.MachineTag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.String()}},
	}
	err := st.facade.FacadeCall("WatchBlockDevices", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchMachine watches for changes to the specified machine.
func (st *State) WatchMachine(m names.MachineTag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.String()}},
	}
	err := st.facade.FacadeCall("WatchMachines", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchVolumes watches for lifecycle changes to volumes scoped to the
// entity with the specified tag.
func (st *State) WatchVolumes(scope names.Tag) (watcher.StringsWatcher, error) {
	return st.watchStorageEntities("WatchVolumes", scope)
}

// WatchFilesystems watches for lifecycle changes to volumes scoped to the
// entity with the specified tag.
func (st *State) WatchFilesystems(scope names.Tag) (watcher.StringsWatcher, error) {
	return st.watchStorageEntities("WatchFilesystems", scope)
}

func (st *State) watchStorageEntities(method string, scope names.Tag) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: scope.String()}},
	}
	err := st.facade.FacadeCall(method, args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchVolumeAttachments watches for changes to volume attachments
// scoped to the entity with the specified tag.
func (st *State) WatchVolumeAttachments(scope names.Tag) (watcher.MachineStorageIdsWatcher, error) {
	return st.watchAttachments("WatchVolumeAttachments", scope, apiwatcher.NewVolumeAttachmentsWatcher)
}

// WatchVolumeAttachmentPlans watches for changes to volume attachments
// scoped to the entity with the tag passed to NewState.
func (st *State) WatchVolumeAttachmentPlans(scope names.Tag) (watcher.MachineStorageIdsWatcher, error) {
	return st.watchAttachments("WatchVolumeAttachmentPlans", scope, apiwatcher.NewVolumeAttachmentPlansWatcher)
}

// WatchFilesystemAttachments watches for changes to filesystem attachments
// scoped to the entity with the specified tag.
func (st *State) WatchFilesystemAttachments(scope names.Tag) (watcher.MachineStorageIdsWatcher, error) {
	return st.watchAttachments("WatchFilesystemAttachments", scope, apiwatcher.NewFilesystemAttachmentsWatcher)
}

func (st *State) watchAttachments(
	method string,
	scope names.Tag,
	newWatcher func(base.APICaller, params.MachineStorageIdsWatchResult) watcher.MachineStorageIdsWatcher,
) (watcher.MachineStorageIdsWatcher, error) {
	var results params.MachineStorageIdsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: scope.String()}},
	}
	err := st.facade.FacadeCall(method, args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := newWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// Volumes returns details of volumes with the specified tags.
func (st *State) Volumes(tags []names.VolumeTag) ([]params.VolumeResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.VolumeResults
	err := st.facade.FacadeCall("Volumes", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// Filesystems returns details of filesystems with the specified tags.
func (st *State) Filesystems(tags []names.FilesystemTag) ([]params.FilesystemResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.FilesystemResults
	err := st.facade.FacadeCall("Filesystems", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

func (st *State) VolumeAttachmentPlans(ids []params.MachineStorageId) ([]params.VolumeAttachmentPlanResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.VolumeAttachmentPlanResults
	err := st.facade.FacadeCall("VolumeAttachmentPlans", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

func (st *State) RemoveVolumeAttachmentPlan(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	var results params.ErrorResults
	args := params.MachineStorageIds{
		Ids: ids,
	}
	if err := st.facade.FacadeCall("RemoveVolumeAttachmentPlan", args, &results); err != nil {
		return nil, err
	}
	return results.Results, nil
}

// VolumeAttachments returns details of volume attachments with the specified IDs.
func (st *State) VolumeAttachments(ids []params.MachineStorageId) ([]params.VolumeAttachmentResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.VolumeAttachmentResults
	err := st.facade.FacadeCall("VolumeAttachments", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// VolumeBlockDevices returns details of block devices corresponding to the volume
// attachments with the specified IDs.
func (st *State) VolumeBlockDevices(ids []params.MachineStorageId) ([]params.BlockDeviceResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.BlockDeviceResults
	err := st.facade.FacadeCall("VolumeBlockDevices", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// FilesystemAttachments returns details of filesystem attachments with the specified IDs.
func (st *State) FilesystemAttachments(ids []params.MachineStorageId) ([]params.FilesystemAttachmentResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.FilesystemAttachmentResults
	err := st.facade.FacadeCall("FilesystemAttachments", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// VolumeParams returns the parameters for creating the volumes
// with the specified tags.
func (st *State) VolumeParams(tags []names.VolumeTag) ([]params.VolumeParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.VolumeParamsResults
	err := st.facade.FacadeCall("VolumeParams", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// RemoveVolumeParams returns the parameters for destroying or releasing
// the volumes with the specified tags.
func (st *State) RemoveVolumeParams(tags []names.VolumeTag) ([]params.RemoveVolumeParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.RemoveVolumeParamsResults
	err := st.facade.FacadeCall("RemoveVolumeParams", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// FilesystemParams returns the parameters for creating the filesystems
// with the specified tags.
func (st *State) FilesystemParams(tags []names.FilesystemTag) ([]params.FilesystemParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.FilesystemParamsResults
	err := st.facade.FacadeCall("FilesystemParams", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// RemoveFilesystemParams returns the parameters for destroying or releasing
// the filesystems with the specified tags.
func (st *State) RemoveFilesystemParams(tags []names.FilesystemTag) ([]params.RemoveFilesystemParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.RemoveFilesystemParamsResults
	err := st.facade.FacadeCall("RemoveFilesystemParams", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// VolumeAttachmentParams returns the parameters for creating the volume
// attachments with the specified tags.
func (st *State) VolumeAttachmentParams(ids []params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.VolumeAttachmentParamsResults
	err := st.facade.FacadeCall("VolumeAttachmentParams", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// FilesystemAttachmentParams returns the parameters for creating the
// filesystem attachments with the specified tags.
func (st *State) FilesystemAttachmentParams(ids []params.MachineStorageId) ([]params.FilesystemAttachmentParamsResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.FilesystemAttachmentParamsResults
	err := st.facade.FacadeCall("FilesystemAttachmentParams", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// SetVolumeInfo records the details of newly provisioned volumes.
func (st *State) SetVolumeInfo(volumes []params.Volume) ([]params.ErrorResult, error) {
	args := params.Volumes{Volumes: volumes}
	var results params.ErrorResults
	err := st.facade.FacadeCall("SetVolumeInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumes) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumes), len(results.Results))
	}
	return results.Results, nil
}

// SetFilesystemInfo records the details of newly provisioned filesystems.
func (st *State) SetFilesystemInfo(filesystems []params.Filesystem) ([]params.ErrorResult, error) {
	args := params.Filesystems{Filesystems: filesystems}
	var results params.ErrorResults
	err := st.facade.FacadeCall("SetFilesystemInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(filesystems) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(filesystems), len(results.Results))
	}
	return results.Results, nil
}

func (st *State) CreateVolumeAttachmentPlans(volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	args := params.VolumeAttachmentPlans{VolumeAttachmentPlans: volumeAttachmentPlans}
	var results params.ErrorResults
	err := st.facade.FacadeCall("CreateVolumeAttachmentPlans", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumeAttachmentPlans) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumeAttachmentPlans), len(results.Results))
	}
	return results.Results, nil
}

func (st *State) SetVolumeAttachmentPlanBlockInfo(volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	args := params.VolumeAttachmentPlans{VolumeAttachmentPlans: volumeAttachmentPlans}
	var results params.ErrorResults
	err := st.facade.FacadeCall("SetVolumeAttachmentPlanBlockInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumeAttachmentPlans) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumeAttachmentPlans), len(results.Results))
	}
	return results.Results, nil
}

// SetVolumeAttachmentInfo records the details of newly provisioned volume attachments.
func (st *State) SetVolumeAttachmentInfo(volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
	args := params.VolumeAttachments{VolumeAttachments: volumeAttachments}
	var results params.ErrorResults
	err := st.facade.FacadeCall("SetVolumeAttachmentInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumeAttachments) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumeAttachments), len(results.Results))
	}
	return results.Results, nil
}

// SetFilesystemAttachmentInfo records the details of newly provisioned filesystem attachments.
func (st *State) SetFilesystemAttachmentInfo(filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
	args := params.FilesystemAttachments{FilesystemAttachments: filesystemAttachments}
	var results params.ErrorResults
	err := st.facade.FacadeCall("SetFilesystemAttachmentInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(filesystemAttachments) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(filesystemAttachments), len(results.Results))
	}
	return results.Results, nil
}

// Life requests the life cycle of the entities with the specified tags.
func (st *State) Life(tags []names.Tag) ([]params.LifeResult, error) {
	var results params.LifeResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	if err := st.facade.FacadeCall("Life", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// AttachmentLife requests the life cycle of the attachments with the specified IDs.
func (st *State) AttachmentLife(ids []params.MachineStorageId) ([]params.LifeResult, error) {
	var results params.LifeResults
	args := params.MachineStorageIds{ids}
	if err := st.facade.FacadeCall("AttachmentLife", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// EnsureDead progresses the entities with the specified tags to the Dead
// life cycle state, if they are Alive or Dying.
func (st *State) EnsureDead(tags []names.Tag) ([]params.ErrorResult, error) {
	var results params.ErrorResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	if err := st.facade.FacadeCall("EnsureDead", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// Remove removes the entities with the specified tags from state.
func (st *State) Remove(tags []names.Tag) ([]params.ErrorResult, error) {
	var results params.ErrorResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	if err := st.facade.FacadeCall("Remove", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// RemoveAttachments removes the attachments with the specified IDs from state.
func (st *State) RemoveAttachments(ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	var results params.ErrorResults
	args := params.MachineStorageIds{ids}
	if err := st.facade.FacadeCall("RemoveAttachment", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// InstanceIds returns the provider specific instance ID for each machine,
// or an CodeNotProvisioned error if not set.
func (st *State) InstanceIds(tags []names.MachineTag) ([]params.StringResult, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	err := st.facade.FacadeCall("InstanceId", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected %d result(s), got %d", len(results.Results), len(tags))
	}
	return results.Results, nil
}

// SetStatus sets the status of storage entities.
func (st *State) SetStatus(args []params.EntityStatusArgs) error {
	var result params.ErrorResults
	err := st.facade.FacadeCall("SetStatus", params.SetStatus{args}, &result)
	if err != nil {
		return err
	}
	return result.Combine()
}
