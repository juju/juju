// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystemwatcher_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher/watchertest"
)

type mockBackend struct {
	machineFilesystemsW           *watchertest.StringsWatcher
	unitFilesystemsW              *watchertest.StringsWatcher
	machineFilesystemAttachmentsW *watchertest.StringsWatcher
	unitFilesystemAttachmentsW    *watchertest.StringsWatcher
	modelFilesystemsW             *watchertest.StringsWatcher
	modelFilesystemAttachmentsW   *watchertest.StringsWatcher
	modelVolumeAttachmentsW       *watchertest.StringsWatcher

	filesystems               map[string]*mockFilesystem
	volumeAttachments         map[string]*mockVolumeAttachment
	volumeAttachmentRequested chan names.VolumeTag
}

func (b *mockBackend) Filesystem(tag names.FilesystemTag) (state.Filesystem, error) {
	if f, ok := b.filesystems[tag.Id()]; ok {
		return f, nil
	}
	return nil, errors.NotFoundf("filesystem %s", tag.Id())
}

func (b *mockBackend) VolumeAttachment(host names.Tag, v names.VolumeTag) (state.VolumeAttachment, error) {
	if host.Id() != "0" && host.Id() != "mariadb/0" {
		// The tests all operate on host "0" or "mariadb/0", and the watchers
		// should ignore attachments for other machines, so we should
		// never get here.
		return nil, errors.Errorf("should not get here, unexpected host %v", host)
	}
	// Inform the test code that the volume attachment has been requested.
	// This gives the test a way of knowing when events have been handled,
	// and it's safe to make modifications.
	defer func() {
		select {
		case b.volumeAttachmentRequested <- v:
		default:
		}
	}()
	if a, ok := b.volumeAttachments[v.Id()]; ok {
		return a, nil
	}
	return nil, errors.NotFoundf("attachment for volume %s to host %s", v.Id(), host.Id())
}

func (b *mockBackend) WatchMachineFilesystems(tag names.MachineTag) state.StringsWatcher {
	return b.machineFilesystemsW
}

func (b *mockBackend) WatchMachineFilesystemAttachments(tag names.MachineTag) state.StringsWatcher {
	return b.machineFilesystemAttachmentsW
}

func (b *mockBackend) WatchUnitFilesystemAttachments(tag names.ApplicationTag) state.StringsWatcher {
	return b.unitFilesystemAttachmentsW
}

func (b *mockBackend) WatchModelFilesystems() state.StringsWatcher {
	return b.modelFilesystemsW
}

func (b *mockBackend) WatchModelFilesystemAttachments() state.StringsWatcher {
	return b.modelFilesystemAttachmentsW
}

func (b *mockBackend) WatchModelVolumeAttachments() state.StringsWatcher {
	return b.modelVolumeAttachmentsW
}

func newStringsWatcher() *watchertest.StringsWatcher {
	return watchertest.NewStringsWatcher(make(chan []string, 1))
}

type mockFilesystem struct {
	state.Filesystem
	volume names.VolumeTag
}

func (f *mockFilesystem) Volume() (names.VolumeTag, error) {
	if f.volume == (names.VolumeTag{}) {
		return names.VolumeTag{}, state.ErrNoBackingVolume
	}
	return f.volume, nil
}

type mockVolumeAttachment struct {
	state.VolumeAttachment
	life state.Life
}

func (a *mockVolumeAttachment) Life() state.Life {
	return a.life
}
