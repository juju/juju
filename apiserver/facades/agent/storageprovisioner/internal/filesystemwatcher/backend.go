// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystemwatcher

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/state"
)

// Backend provides access to filesystems and volumes for the
// filesystem watchers to use.
type Backend interface {
	Filesystem(names.FilesystemTag) (state.Filesystem, error)
	VolumeAttachment(names.MachineTag, names.VolumeTag) (state.VolumeAttachment, error)
	WatchMachineFilesystems(names.MachineTag) state.StringsWatcher
	WatchMachineFilesystemAttachments(names.MachineTag) state.StringsWatcher
	WatchModelFilesystems() state.StringsWatcher
	WatchModelFilesystemAttachments() state.StringsWatcher
	WatchModelVolumeAttachments() state.StringsWatcher
}
