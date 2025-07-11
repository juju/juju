// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const storageProvisionerFacade = "StorageProvisioner"

// Client provides access to a storageprovisioner facade client.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new client-side StorageProvisioner facade.
func NewClient(caller base.APICaller, options ...Option) (*Client, error) {
	facadeCaller := base.NewFacadeCaller(caller, storageProvisionerFacade, options...)
	return &Client{facadeCaller}, nil
}

// WatchBlockDevices watches for changes to the specified machine's block devices.
func (st *Client) WatchBlockDevices(ctx context.Context, m names.MachineTag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.String()}},
	}
	err := st.facade.FacadeCall(ctx, "WatchBlockDevices", args, &results)
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
func (st *Client) WatchMachine(ctx context.Context, m names.MachineTag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: m.String()}},
	}
	err := st.facade.FacadeCall(ctx, "WatchMachines", args, &results)
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
func (st *Client) WatchVolumes(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error) {
	return st.watchStorageEntities(ctx, "WatchVolumes", scope)
}

// WatchFilesystems watches for lifecycle changes to volumes scoped to the
// entity with the specified tag.
func (st *Client) WatchFilesystems(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error) {
	return st.watchStorageEntities(ctx, "WatchFilesystems", scope)
}

func (st *Client) watchStorageEntities(ctx context.Context, method string, scope names.Tag) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: scope.String()}},
	}
	err := st.facade.FacadeCall(ctx, method, args, &results)
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
func (st *Client) WatchVolumeAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	return st.watchAttachments(ctx, "WatchVolumeAttachments", scope, apiwatcher.NewStorageAttachmentsWatcher)
}

// WatchVolumeAttachmentPlans watches for changes to volume attachments
// scoped to the entity with the tag passed to NewClient.
func (st *Client) WatchVolumeAttachmentPlans(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	return st.watchAttachments(ctx, "WatchVolumeAttachmentPlans", scope, apiwatcher.NewStorageAttachmentsWatcher)
}

// WatchFilesystemAttachments watches for changes to filesystem attachments
// scoped to the entity with the specified tag.
func (st *Client) WatchFilesystemAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	return st.watchAttachments(ctx, "WatchFilesystemAttachments", scope, apiwatcher.NewStorageAttachmentsWatcher)
}

func (st *Client) watchAttachments(
	ctx context.Context,
	method string,
	scope names.Tag,
	newWatcher func(base.APICaller, params.MachineStorageIdsWatchResult) watcher.MachineStorageIDsWatcher,
) (watcher.MachineStorageIDsWatcher, error) {
	var results params.MachineStorageIdsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: scope.String()}},
	}
	err := st.facade.FacadeCall(ctx, method, args, &results)
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
func (st *Client) Volumes(ctx context.Context, tags []names.VolumeTag) ([]params.VolumeResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.VolumeResults
	err := st.facade.FacadeCall(ctx, "Volumes", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// Filesystems returns details of filesystems with the specified tags.
func (st *Client) Filesystems(ctx context.Context, tags []names.FilesystemTag) ([]params.FilesystemResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.FilesystemResults
	err := st.facade.FacadeCall(ctx, "Filesystems", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

func (st *Client) VolumeAttachmentPlans(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentPlanResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.VolumeAttachmentPlanResults
	err := st.facade.FacadeCall(ctx, "VolumeAttachmentPlans", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

func (st *Client) RemoveVolumeAttachmentPlan(ctx context.Context, ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	var results params.ErrorResults
	args := params.MachineStorageIds{
		Ids: ids,
	}
	if err := st.facade.FacadeCall(ctx, "RemoveVolumeAttachmentPlan", args, &results); err != nil {
		return nil, err
	}
	return results.Results, nil
}

// VolumeAttachments returns details of volume attachments with the specified IDs.
func (st *Client) VolumeAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.VolumeAttachmentResults
	err := st.facade.FacadeCall(ctx, "VolumeAttachments", args, &results)
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
func (st *Client) VolumeBlockDevices(ctx context.Context, ids []params.MachineStorageId) ([]params.BlockDeviceResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.BlockDeviceResults
	err := st.facade.FacadeCall(ctx, "VolumeBlockDevices", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// FilesystemAttachments returns details of filesystem attachments with the specified IDs.
func (st *Client) FilesystemAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.FilesystemAttachmentResults
	err := st.facade.FacadeCall(ctx, "FilesystemAttachments", args, &results)
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
func (st *Client) VolumeParams(ctx context.Context, tags []names.VolumeTag) ([]params.VolumeParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.VolumeParamsResults
	err := st.facade.FacadeCall(ctx, "VolumeParams", args, &results)
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
func (st *Client) RemoveVolumeParams(ctx context.Context, tags []names.VolumeTag) ([]params.RemoveVolumeParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.RemoveVolumeParamsResults
	err := st.facade.FacadeCall(ctx, "RemoveVolumeParams", args, &results)
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
func (st *Client) FilesystemParams(ctx context.Context, tags []names.FilesystemTag) ([]params.FilesystemParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.FilesystemParamsResults
	err := st.facade.FacadeCall(ctx, "FilesystemParams", args, &results)
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
func (st *Client) RemoveFilesystemParams(ctx context.Context, tags []names.FilesystemTag) ([]params.RemoveFilesystemParamsResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.RemoveFilesystemParamsResults
	err := st.facade.FacadeCall(ctx, "RemoveFilesystemParams", args, &results)
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
func (st *Client) VolumeAttachmentParams(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.VolumeAttachmentParamsResults
	err := st.facade.FacadeCall(ctx, "VolumeAttachmentParams", args, &results)
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
func (st *Client) FilesystemAttachmentParams(ctx context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentParamsResult, error) {
	args := params.MachineStorageIds{ids}
	var results params.FilesystemAttachmentParamsResults
	err := st.facade.FacadeCall(ctx, "FilesystemAttachmentParams", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// SetVolumeInfo records the details of newly provisioned volumes.
func (st *Client) SetVolumeInfo(ctx context.Context, volumes []params.Volume) ([]params.ErrorResult, error) {
	args := params.Volumes{Volumes: volumes}
	var results params.ErrorResults
	err := st.facade.FacadeCall(ctx, "SetVolumeInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumes) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumes), len(results.Results))
	}
	return results.Results, nil
}

// SetFilesystemInfo records the details of newly provisioned filesystems.
func (st *Client) SetFilesystemInfo(ctx context.Context, filesystems []params.Filesystem) ([]params.ErrorResult, error) {
	args := params.Filesystems{Filesystems: filesystems}
	var results params.ErrorResults
	err := st.facade.FacadeCall(ctx, "SetFilesystemInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(filesystems) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(filesystems), len(results.Results))
	}
	return results.Results, nil
}

func (st *Client) CreateVolumeAttachmentPlans(ctx context.Context, volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	args := params.VolumeAttachmentPlans{VolumeAttachmentPlans: volumeAttachmentPlans}
	var results params.ErrorResults
	err := st.facade.FacadeCall(ctx, "CreateVolumeAttachmentPlans", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumeAttachmentPlans) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumeAttachmentPlans), len(results.Results))
	}
	return results.Results, nil
}

func (st *Client) SetVolumeAttachmentPlanBlockInfo(ctx context.Context, volumeAttachmentPlans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	args := params.VolumeAttachmentPlans{VolumeAttachmentPlans: volumeAttachmentPlans}
	var results params.ErrorResults
	err := st.facade.FacadeCall(ctx, "SetVolumeAttachmentPlanBlockInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumeAttachmentPlans) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumeAttachmentPlans), len(results.Results))
	}
	return results.Results, nil
}

// SetVolumeAttachmentInfo records the details of newly provisioned volume attachments.
func (st *Client) SetVolumeAttachmentInfo(ctx context.Context, volumeAttachments []params.VolumeAttachment) ([]params.ErrorResult, error) {
	args := params.VolumeAttachments{VolumeAttachments: volumeAttachments}
	var results params.ErrorResults
	err := st.facade.FacadeCall(ctx, "SetVolumeAttachmentInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(volumeAttachments) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(volumeAttachments), len(results.Results))
	}
	return results.Results, nil
}

// SetFilesystemAttachmentInfo records the details of newly provisioned filesystem attachments.
func (st *Client) SetFilesystemAttachmentInfo(ctx context.Context, filesystemAttachments []params.FilesystemAttachment) ([]params.ErrorResult, error) {
	args := params.FilesystemAttachments{FilesystemAttachments: filesystemAttachments}
	var results params.ErrorResults
	err := st.facade.FacadeCall(ctx, "SetFilesystemAttachmentInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(filesystemAttachments) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(filesystemAttachments), len(results.Results))
	}
	return results.Results, nil
}

// Life requests the life cycle of the entities with the specified tags.
func (st *Client) Life(ctx context.Context, tags []names.Tag) ([]params.LifeResult, error) {
	var results params.LifeResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	if err := st.facade.FacadeCall(ctx, "Life", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// AttachmentLife requests the life cycle of the attachments with the specified IDs.
func (st *Client) AttachmentLife(ctx context.Context, ids []params.MachineStorageId) ([]params.LifeResult, error) {
	var results params.LifeResults
	args := params.MachineStorageIds{ids}
	if err := st.facade.FacadeCall(ctx, "AttachmentLife", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// Remove removes the entities with the specified tags from state.
func (st *Client) Remove(ctx context.Context, tags []names.Tag) ([]params.ErrorResult, error) {
	var results params.ErrorResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	if err := st.facade.FacadeCall(ctx, "Remove", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// RemoveAttachments removes the attachments with the specified IDs from state.
func (st *Client) RemoveAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	var results params.ErrorResults
	args := params.MachineStorageIds{ids}
	if err := st.facade.FacadeCall(ctx, "RemoveAttachment", args, &results); err != nil {
		return nil, err
	}
	if len(results.Results) != len(ids) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(ids), len(results.Results))
	}
	return results.Results, nil
}

// InstanceIds returns the provider specific instance ID for each machine,
// or an CodeNotProvisioned error if not set.
func (st *Client) InstanceIds(ctx context.Context, tags []names.MachineTag) ([]params.StringResult, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	err := st.facade.FacadeCall(ctx, "InstanceId", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected %d result(s), got %d", len(results.Results), len(tags))
	}
	return results.Results, nil
}

// SetStatus sets the status of storage entities.
func (st *Client) SetStatus(ctx context.Context, args []params.EntityStatusArgs) error {
	var result params.ErrorResults
	err := st.facade.FacadeCall(ctx, "SetStatus", params.SetStatus{args}, &result)
	if err != nil {
		return err
	}
	return result.Combine()
}
