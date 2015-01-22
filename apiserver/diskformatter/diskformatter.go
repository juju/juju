// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskformatter

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
)

func init() {
	common.RegisterStandardFacade("DiskFormatter", 1, NewDiskFormatterAPI)
}

var logger = loggo.GetLogger("juju.apiserver.diskformatter")

// DiskFormatterAPI provides access to the DiskFormatter API facade.
type DiskFormatterAPI struct {
	st          stateInterface
	resources   *common.Resources
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

// NewDiskFormatterAPI creates a new server-side DiskFormatter API facade.
func NewDiskFormatterAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*DiskFormatterAPI, error) {

	if !authorizer.AuthUnitAgent() {
		return nil, common.ErrPerm
	}

	getAuthFunc := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}

	return &DiskFormatterAPI{
		st:          getState(st),
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

// WatchBlockDevices returns a StringsWatcher for observing changes
// to block devices associated with the unit's machine.
func (a *DiskFormatterAPI) WatchBlockDevices(args params.Entities) (params.StringsWatchResults, error) {
	result := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.StringsWatchResults{}, err
	}
	for i, entity := range args.Entities {
		unit, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		err = common.ErrPerm
		var watcherId string
		var changes []string
		if canAccess(unit) {
			watcherId, changes, err = a.watchOneBlockDevices(unit)
		}
		result.Results[i].StringsWatcherId = watcherId
		result.Results[i].Changes = changes
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (a *DiskFormatterAPI) watchOneBlockDevices(tag names.UnitTag) (string, []string, error) {
	w, err := a.st.WatchUnitMachineBlockDevices(tag)
	if err != nil {
		return "", nil, err
	}
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response.
	if changes, ok := <-w.Changes(); ok {
		return a.resources.Register(w), changes, nil
	}
	return "", nil, watcher.EnsureErr(w)
}

// BlockDevice returns details about each specified block device.
func (a *DiskFormatterAPI) BlockDevice(args params.Entities) (params.BlockDeviceResults, error) {
	result := params.BlockDeviceResults{
		Results: make([]params.BlockDeviceResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.BlockDeviceResults{}, err
	}
	one := func(entity params.Entity) (storage.BlockDevice, error) {
		blockDevice, _, err := a.oneBlockDevice(entity.Tag, canAccess)
		if err != nil {
			return storage.BlockDevice{}, err
		}
		return storageBlockDevice(blockDevice)
	}
	for i, entity := range args.Entities {
		blockDevice, err := one(entity)
		result.Results[i].Result = blockDevice
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// BlockDeviceAttached reports whether or not each of the specified block
// devices is attached to the machine containing the authenticated unit
// agent.
func (a *DiskFormatterAPI) BlockDeviceAttached(args params.Entities) (params.BoolResults, error) {
	result := params.BoolResults{
		Results: make([]params.BoolResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.BoolResults{}, err
	}
	one := func(entity params.Entity) (bool, error) {
		blockDevice, _, err := a.oneBlockDevice(entity.Tag, canAccess)
		if err != nil {
			return false, err
		}
		return blockDevice.Attached(), nil
	}
	for i, entity := range args.Entities {
		attached, err := one(entity)
		result.Results[i].Result = attached
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// BlockDeviceStorageInstance returns details of storage instances corresponding
// to each specified block device.
func (a *DiskFormatterAPI) BlockDeviceStorageInstance(args params.Entities) (params.StorageInstanceResults, error) {
	result := params.StorageInstanceResults{
		Results: make([]params.StorageInstanceResult, len(args.Entities)),
	}
	canAccess, err := a.getAuthFunc()
	if err != nil {
		return params.StorageInstanceResults{}, err
	}
	one := func(entity params.Entity) (storage.StorageInstance, error) {
		_, storageInstance, err := a.oneBlockDevice(entity.Tag, canAccess)
		if err != nil {
			return storage.StorageInstance{}, err
		}
		return storageStorageInstance(storageInstance)
	}
	for i, entity := range args.Entities {
		storageInstance, err := one(entity)
		result.Results[i].Result = storageInstance
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (a *DiskFormatterAPI) oneBlockDevice(tag string, canAccess common.AuthFunc) (state.BlockDevice, state.StorageInstance, error) {
	diskTag, err := names.ParseDiskTag(tag)
	if err != nil {
		return nil, nil, common.ErrPerm
	}
	blockDevice, err := a.st.BlockDevice(diskTag.Id())
	if err != nil {
		return nil, nil, common.ErrPerm
	}
	storageInstanceId, ok := blockDevice.StorageInstance()
	if !ok {
		// not assigned to any storage instance
		return nil, nil, common.ErrPerm
	}
	storageInstance, err := a.st.StorageInstance(storageInstanceId)
	if err != nil || !canAccess(storageInstance.Owner()) {
		return nil, nil, common.ErrPerm
	}
	return blockDevice, storageInstance, nil
}

// NOTE: purposefully not using field keys below, so
// the code breaks if structures change.

func storageBlockDevice(dev state.BlockDevice) (storage.BlockDevice, error) {
	if dev == nil {
		return storage.BlockDevice{}, nil
	}
	info, err := dev.Info()
	if err != nil {
		return storage.BlockDevice{}, err
	}
	return storage.BlockDevice{
		dev.Name(),
		"", // TODO(axw) ProviderId
		info.DeviceName,
		info.Label,
		info.UUID,
		info.Serial,
		info.Size,
		info.FilesystemType,
		info.InUse,
	}, nil
}

func storageStorageInstance(st state.StorageInstance) (storage.StorageInstance, error) {
	if st == nil {
		return storage.StorageInstance{}, nil
	}
	return storage.StorageInstance{
		st.Id(),
		storageStorageKind(st.Kind()),
	}, nil
}

func storageStorageKind(k state.StorageKind) storage.StorageKind {
	switch k {
	case state.StorageKindBlock:
		return storage.StorageKindBlock
	case state.StorageKindFilesystem:
		return storage.StorageKindFilesystem
	default:
		return storage.StorageKindUnknown
	}
}
