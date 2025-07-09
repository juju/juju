// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystemwatcher

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// Watchers provides methods for watching filesystems. The watches aggregate
// results from host- and model-scoped watchers, to conform to the behaviour
// of the storageprovisioner worker. The model-level storageprovisioner watches
// model-scoped filesystems that have no backing volume. The host-level worker
// watches both host-scoped filesystems, and model-scoped filesystems whose
// backing volumes are attached to the host.
type Watchers struct {
	Backend Backend
}

// WatchModelManagedFilesystemAttachments returns a strings watcher that
// reports lifecycle changes to attachments of model-scoped filesystem that
// have no backing volume. Volume-backed filesystems are always managed by
// the host to which they are attached.
func (fw Watchers) WatchModelManagedFilesystemAttachments() state.StringsWatcher {
	return newFilteredStringsWatcher(fw.Backend.WatchModelFilesystemAttachments(), func(id string) (bool, error) {
		_, filesystemTag, err := state.ParseFilesystemAttachmentId(id)
		if err != nil {
			return false, errors.Annotate(err, "parsing filesystem attachment ID")
		}
		f, err := fw.Backend.Filesystem(filesystemTag)
		if errors.Is(err, errors.NotFound) {
			return false, nil
		} else if err != nil {
			return false, errors.Trace(err)
		}
		_, err = f.Volume()
		return err == state.ErrNoBackingVolume, nil
	})
}

// WatchMachineManagedFilesystemAttachments returns a strings watcher that
// reports lifecycle changes for attachments to both machine-scoped filesystems,
// and model-scoped, volume-backed filesystems that are attached to the
// specified machine.
func (fw Watchers) WatchMachineManagedFilesystemAttachments(m names.MachineTag) state.StringsWatcher {
	w := &hostFilesystemAttachmentsWatcher{
		stringsWatcherBase:               stringsWatcherBase{out: make(chan []string)},
		backend:                          fw.Backend,
		changes:                          set.NewStrings(),
		hostFilesystemAttachments:        fw.Backend.WatchMachineFilesystemAttachments(m),
		modelFilesystemAttachments:       fw.Backend.WatchModelFilesystemAttachments(),
		modelVolumeAttachments:           fw.Backend.WatchModelVolumeAttachments(),
		modelVolumesAttached:             names.NewSet(),
		modelVolumeFilesystemAttachments: make(map[names.VolumeTag]string),
		hostMatch: func(tag names.Tag) (bool, error) {
			return tag == m, nil
		},
	}

	w.tomb.Go(func() error {
		defer watcher.Stop(w.hostFilesystemAttachments, &w.tomb)
		defer watcher.Stop(w.modelFilesystemAttachments, &w.tomb)
		defer watcher.Stop(w.modelVolumeAttachments, &w.tomb)
		return w.loop()
	})
	return w
}

// WatchUnitManagedFilesystemAttachments returns a strings watcher that
// reports lifecycle changes for attachments to both unit-scoped filesystems,
// and model-scoped, volume-backed filesystems that are attached to units of the
// specified application.
func (fw Watchers) WatchUnitManagedFilesystemAttachments(app names.ApplicationTag) state.StringsWatcher {
	w := &hostFilesystemAttachmentsWatcher{
		stringsWatcherBase:               stringsWatcherBase{out: make(chan []string)},
		backend:                          fw.Backend,
		changes:                          set.NewStrings(),
		hostFilesystemAttachments:        fw.Backend.WatchUnitFilesystemAttachments(app),
		modelFilesystemAttachments:       fw.Backend.WatchModelFilesystemAttachments(),
		modelVolumeAttachments:           fw.Backend.WatchModelVolumeAttachments(),
		modelVolumesAttached:             names.NewSet(),
		modelVolumeFilesystemAttachments: make(map[names.VolumeTag]string),
		hostMatch: func(tag names.Tag) (bool, error) {
			unitApp, err := names.UnitApplication(tag.Id())
			if err != nil {
				return false, errors.Trace(err)
			}
			return unitApp == app.Id(), nil
		},
	}

	w.tomb.Go(func() error {
		defer watcher.Stop(w.hostFilesystemAttachments, &w.tomb)
		defer watcher.Stop(w.modelFilesystemAttachments, &w.tomb)
		defer watcher.Stop(w.modelVolumeAttachments, &w.tomb)
		return w.loop()
	})
	return w
}

// hostFilesystemAttachmentsWatcher is a strings watcher that reports
// lifechcle changes for attachments to both host-scoped filesystems,
// and model-scoped, volume-backed filesystems that are attached to the
// specified host.
//
// NOTE(axw) we use the existence of the *volume* attachment rather than
// filesystem attachment because the filesystem attachment can be destroyed
// before the filesystem, but the volume attachment cannot.
type hostFilesystemAttachmentsWatcher struct {
	stringsWatcherBase
	changes                          set.Strings
	backend                          Backend
	hostFilesystemAttachments        state.StringsWatcher
	modelFilesystemAttachments       state.StringsWatcher
	modelVolumeAttachments           state.StringsWatcher
	modelVolumesAttached             names.Set
	modelVolumeFilesystemAttachments map[names.VolumeTag]string
	hostMatch                        func(names.Tag) (bool, error)
}

func (w *hostFilesystemAttachmentsWatcher) loop() error {
	defer close(w.out)
	var out chan<- []string
	var machineFilesystemAttachmentsReceived bool
	var modelFilesystemAttachmentsReceived bool
	var modelVolumeAttachmentsReceived bool
	var sentFirst bool
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case values, ok := <-w.hostFilesystemAttachments.Changes():
			if !ok {
				return watcher.EnsureErr(w.hostFilesystemAttachments)
			}
			machineFilesystemAttachmentsReceived = true
			for _, v := range values {
				w.changes.Add(v)
			}
		case values, ok := <-w.modelFilesystemAttachments.Changes():
			if !ok {
				return watcher.EnsureErr(w.modelFilesystemAttachments)
			}
			modelFilesystemAttachmentsReceived = true
			for _, id := range values {
				hostTag, filesystemTag, err := state.ParseFilesystemAttachmentId(id)
				if err != nil {
					return errors.Annotate(err, "parsing filesystem attachment ID")
				}
				match, err := w.hostMatch(hostTag)
				if err != nil {
					return errors.Annotate(err, "parsing filesystem host tag")
				}
				if !match {
					continue
				}
				if err := w.modelFilesystemAttachmentChanged(id, filesystemTag); err != nil {
					return errors.Trace(err)
				}
			}
		case values, ok := <-w.modelVolumeAttachments.Changes():
			if !ok {
				return watcher.EnsureErr(w.modelVolumeAttachments)
			}
			modelVolumeAttachmentsReceived = true
			for _, id := range values {
				hostTag, volumeTag, err := state.ParseVolumeAttachmentId(id)
				if err != nil {
					return errors.Annotate(err, "parsing volume attachment ID")
				}
				match, err := w.hostMatch(hostTag)
				if err != nil {
					return errors.Annotate(err, "parsing volume host tag")
				}
				if !match {
					continue
				}
				if err := w.modelVolumeAttachmentChanged(hostTag, volumeTag); err != nil {
					return errors.Trace(err)
				}
			}
		case out <- w.changes.SortedValues():
			w.changes = set.NewStrings()
			out = nil
		}
		// NOTE(axw) we don't send any changes until we have received
		// an initial event from each of the watchers. This ensures
		// that we provide a complete view of the world in the initial
		// event, which is expected of all watchers.
		if machineFilesystemAttachmentsReceived &&
			modelFilesystemAttachmentsReceived &&
			modelVolumeAttachmentsReceived &&
			(!sentFirst || len(w.changes) > 0) {
			sentFirst = true
			out = w.out
		}
	}
}

func (w *hostFilesystemAttachmentsWatcher) modelFilesystemAttachmentChanged(
	filesystemAttachmentId string,
	filesystemTag names.FilesystemTag,
) error {
	filesystem, err := w.backend.Filesystem(filesystemTag)
	if errors.Is(err, errors.NotFound) {
		// Filesystem removed: nothing more to do.
		return nil
	} else if err != nil {
		return errors.Annotate(err, "getting filesystem")
	}
	volumeTag, err := filesystem.Volume()
	if err == state.ErrNoBackingVolume {
		// Filesystem has no backing volume: nothing more to do.
		return nil
	} else if err != nil {
		return errors.Annotate(err, "getting filesystem volume")
	}
	w.modelVolumeFilesystemAttachments[volumeTag] = filesystemAttachmentId
	if w.modelVolumesAttached.Contains(volumeTag) {
		w.changes.Add(filesystemAttachmentId)
	}
	return nil
}

func (w *hostFilesystemAttachmentsWatcher) modelVolumeAttachmentChanged(hostTag names.Tag, volumeTag names.VolumeTag) error {
	va, err := w.backend.VolumeAttachment(hostTag, volumeTag)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Annotate(err, "getting volume attachment")
	}
	if errors.Is(err, errors.NotFound) || va.Life() == state.Dead {
		filesystemAttachmentId, ok := w.modelVolumeFilesystemAttachments[volumeTag]
		if ok {
			// If the volume attachment is Dead/removed,
			// the filesystem attachment must have been
			// removed. We don't get a change notification
			// for removed entities, so we use this to
			// clean up.
			delete(w.modelVolumeFilesystemAttachments, volumeTag)
			w.changes.Remove(filesystemAttachmentId)
			w.modelVolumesAttached.Remove(volumeTag)
		}
		return nil
	}
	w.modelVolumesAttached.Add(volumeTag)
	if filesystemAttachmentId, ok := w.modelVolumeFilesystemAttachments[volumeTag]; ok {
		w.changes.Add(filesystemAttachmentId)
	}
	return nil
}

type filteredStringsWatcher struct {
	stringsWatcherBase
	w      state.StringsWatcher
	filter func(string) (bool, error)
}

func newFilteredStringsWatcher(w state.StringsWatcher, filter func(string) (bool, error)) *filteredStringsWatcher {
	fw := &filteredStringsWatcher{
		stringsWatcherBase: stringsWatcherBase{out: make(chan []string)},
		w:                  w,
		filter:             filter,
	}
	fw.tomb.Go(func() error {
		defer watcher.Stop(fw.w, &fw.tomb)
		return fw.loop()
	})
	return fw
}

func (fw *filteredStringsWatcher) loop() error {
	defer close(fw.out)
	var out chan []string
	var values []string
	for {
		select {
		case <-fw.tomb.Dying():
			return tomb.ErrDying
		case in, ok := <-fw.w.Changes():
			if !ok {
				return watcher.EnsureErr(fw.w)
			}
			values = make([]string, 0, len(in))
			for _, value := range in {
				ok, err := fw.filter(value)
				if err != nil {
					return errors.Trace(err)
				} else if ok {
					values = append(values, value)
				}
			}
			out = fw.out
		case out <- values:
			out = nil
		}
	}
}

type stringsWatcherBase struct {
	tomb tomb.Tomb
	out  chan []string
}

// Err is part of the state.StringsWatcher interface.
func (w *stringsWatcherBase) Err() error {
	return w.tomb.Err()
}

// Stop is part of the state.StringsWatcher interface.
func (w *stringsWatcherBase) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill is part of the state.StringsWatcher interface.
func (w *stringsWatcherBase) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the state.StringsWatcher interface.
func (w *stringsWatcherBase) Wait() error {
	return w.tomb.Wait()
}

// Changes is part of the state.StringsWatcher interface.
func (w *stringsWatcherBase) Changes() <-chan []string {
	return w.out
}
