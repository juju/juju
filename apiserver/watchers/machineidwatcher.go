// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/storagecommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// srvMachineStorageIDsWatcher defines the API wrapping a state.StringsWatcher
// watching machine/storage attachments. This watcher notifies about storage
// entities (volumes/filesystems) being attached to and detached from machines.
//
// TODO(axw) state needs a new watcher, this is a bt of a hack. State watchers
// could do with some deduplication of logic, and I don't want to add to that
// spaghetti right now.
type srvMachineStorageIDsWatcher struct {
	watcherCommon
	watcher state.StringsWatcher
	parser  func([]string) ([]params.MachineStorageId, error)
}

func NewVolumeAttachmentsWatcher(context facade.Context) (facade.Facade, error) {
	return NewMachineStorageIDsWatcher(
		context,
		storagecommon.ParseVolumeAttachmentIds,
	)
}

func NewVolumeAttachmentPlansWatcher(context facade.Context) (facade.Facade, error) {
	return NewMachineStorageIDsWatcher(
		context,
		storagecommon.ParseVolumeAttachmentIds,
	)
}

func NewFilesystemAttachmentsWatcher(context facade.Context) (facade.Facade, error) {
	return NewMachineStorageIDsWatcher(
		context,
		storagecommon.ParseFilesystemAttachmentIds,
	)
}

func NewMachineStorageIDsWatcher(
	context facade.Context,
	parser func([]string) ([]params.MachineStorageId, error),
) (facade.Facade, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
		resources       = context.Resources()
	)

	if !isAgent(auth) {
		return nil, apiservererrors.ErrPerm
	}

	watcher, err := GetWatcherByID(watcherRegistry, resources, id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	stringsWatcher, ok := watcher.(state.StringsWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}
	return &srvMachineStorageIDsWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       stringsWatcher,
		parser:        parser,
	}, nil
}

// Next returns when a change has occurred to an entity of the
// collection being watched since the most recent call to Next
// or the Watch call that created the SrvMachineStorageIdsWatcher.
func (w *srvMachineStorageIDsWatcher) Next() (params.MachineStorageIdsWatchResult, error) {
	if stringChanges, ok := <-w.watcher.Changes(); ok {
		changes, err := w.parser(stringChanges)
		if err != nil {
			return params.MachineStorageIdsWatchResult{}, err
		}
		return params.MachineStorageIdsWatchResult{
			Changes: changes,
		}, nil
	}
	err := w.watcher.Err()
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return params.MachineStorageIdsWatchResult{}, err
}
