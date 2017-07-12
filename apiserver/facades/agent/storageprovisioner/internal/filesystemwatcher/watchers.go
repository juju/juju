// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystemwatcher

import (
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v1"

	"github.com/juju/errors"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/utils/set"
)

// Watchers provides methods for watching filesystems. The watches aggregate
// results from machine- and model-scoped watchers, to conform to the behaviour
// of the storageprovisioner worker. The model-level storageprovisioner watches
// model-scoped filesystems that have no backing volume. The machine-level worker
// watches both machine-scoped filesytems, and model-scoped filesystems whose
// backing volumes are attached to the machine.
type Watchers struct {
	Backend Backend
}

// WatchModelManagedFilesystems returns a strings watcher that reports
// model-scoped filesystems that have no backing volume. Volume-backed
// filesystems are always managed by the machine to which they are attached.
func (fw Watchers) WatchModelManagedFilesystems() state.StringsWatcher {
	return newFilteredStringsWatcher(fw.Backend.WatchModelFilesystems(), func(id string) (bool, error) {
		f, err := fw.Backend.Filesystem(names.NewFilesystemTag(id))
		if errors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Trace(err)
		}
		_, err = f.Volume()
		return err == state.ErrNoBackingVolume, nil
	})
}

// WatchMachineManagedFilesystems returns a strings watcher that reports both
// machine-scoped filesystems, and model-scoped, volume-backed filesystems
// that are attached to the specified machine.
func (fw Watchers) WatchMachineManagedFilesystems(m names.MachineTag) state.StringsWatcher {
	w := &machineFilesystemsWatcher{
		stringsWatcherBase:     stringsWatcherBase{out: make(chan []string)},
		backend:                fw.Backend,
		machine:                m,
		changes:                make(set.Strings),
		machineFilesystems:     fw.Backend.WatchMachineFilesystems(m),
		modelFilesystems:       fw.Backend.WatchModelFilesystems(),
		modelVolumeAttachments: fw.Backend.WatchModelVolumeAttachments(),
		modelVolumesAttached:   make(set.Tags),
		modelVolumeFilesystems: make(map[names.VolumeTag]names.FilesystemTag),
	}
	go func() {
		defer w.tomb.Done()
		defer watcher.Stop(w.machineFilesystems, &w.tomb)
		defer watcher.Stop(w.modelFilesystems, &w.tomb)
		defer watcher.Stop(w.modelVolumeAttachments, &w.tomb)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// machineFilesystemsWatcher is a strings watcher that reports both
// machine-scoped filesystems, and model-scoped, volume-backed filesystems
// that are attached to the specified machine.
//
// NOTE(axw) we use the existence of the *volume* attachment rather than
// filesystem attachment because the filesystem attachment can be destroyed
// before the filesystem, but the volume attachment cannot.
type machineFilesystemsWatcher struct {
	stringsWatcherBase
	changes                set.Strings
	backend                Backend
	machine                names.MachineTag
	machineFilesystems     state.StringsWatcher
	modelFilesystems       state.StringsWatcher
	modelVolumeAttachments state.StringsWatcher
	modelVolumesAttached   set.Tags
	modelVolumeFilesystems map[names.VolumeTag]names.FilesystemTag
}

func (w *machineFilesystemsWatcher) loop() error {
	defer close(w.out)
	var out chan<- []string
	var machineFilesystemsReceived bool
	var modelFilesystemsReceived bool
	var modelVolumeAttachmentsReceived bool
	var sentFirst bool
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case values, ok := <-w.machineFilesystems.Changes():
			if !ok {
				return watcher.EnsureErr(w.machineFilesystems)
			}
			machineFilesystemsReceived = true
			for _, v := range values {
				w.changes.Add(v)
			}
		case values, ok := <-w.modelFilesystems.Changes():
			if !ok {
				return watcher.EnsureErr(w.modelFilesystems)
			}
			modelFilesystemsReceived = true
			for _, id := range values {
				filesystemTag := names.NewFilesystemTag(id)
				if err := w.modelFilesystemChanged(filesystemTag); err != nil {
					return errors.Trace(err)
				}
			}
		case values, ok := <-w.modelVolumeAttachments.Changes():
			if !ok {
				return watcher.EnsureErr(w.modelVolumeAttachments)
			}
			modelVolumeAttachmentsReceived = true
			for _, id := range values {
				machineTag, volumeTag, err := state.ParseVolumeAttachmentId(id)
				if err != nil {
					return errors.Annotate(err, "parsing volume attachment ID")
				}
				if machineTag != w.machine {
					continue
				}
				if err := w.modelVolumeAttachmentChanged(volumeTag); err != nil {
					return errors.Trace(err)
				}
			}
		case out <- w.changes.SortedValues():
			w.changes = make(set.Strings)
			out = nil
		}
		// NOTE(axw) we don't send any changes until we have received
		// an initial event from each of the watchers. This ensures
		// that we provide a complete view of the world in the initial
		// event, which is expected of all watchers.
		if machineFilesystemsReceived &&
			modelFilesystemsReceived &&
			modelVolumeAttachmentsReceived &&
			(!sentFirst || len(w.changes) > 0) {
			sentFirst = true
			out = w.out
		}
	}
}

func (w *machineFilesystemsWatcher) modelFilesystemChanged(filesystemTag names.FilesystemTag) error {
	filesystem, err := w.backend.Filesystem(filesystemTag)
	if errors.IsNotFound(err) {
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
	w.modelVolumeFilesystems[volumeTag] = filesystemTag
	if w.modelVolumesAttached.Contains(volumeTag) {
		w.changes.Add(filesystemTag.Id())
	}
	return nil
}

func (w *machineFilesystemsWatcher) modelVolumeAttachmentChanged(volumeTag names.VolumeTag) error {
	va, err := w.backend.VolumeAttachment(w.machine, volumeTag)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "getting volume attachment")
	}
	if errors.IsNotFound(err) || va.Life() == state.Dead {
		filesystemTag, ok := w.modelVolumeFilesystems[volumeTag]
		if ok {
			// If the volume attachment is Dead/removed,
			// the filesystem must have been removed. We
			// don't get a change notification for removed
			// entities, so we use this to clean up.
			delete(w.modelVolumeFilesystems, volumeTag)
			w.changes.Remove(filesystemTag.Id())
			w.modelVolumesAttached.Remove(volumeTag)
		}
		return nil
	}
	w.modelVolumesAttached.Add(volumeTag)
	if filesystemTag, ok := w.modelVolumeFilesystems[volumeTag]; ok {
		w.changes.Add(filesystemTag.Id())
	}
	return nil
}

// WatchModelManagedFilesystemAttachments returns a strings watcher that
// reports lifecycle changes to attachments of model-scoped filesystem that
// have no backing volume. Volume-backed filesystems are always managed by
// the machine to which they are attached.
func (fw Watchers) WatchModelManagedFilesystemAttachments() state.StringsWatcher {
	return newFilteredStringsWatcher(fw.Backend.WatchModelFilesystemAttachments(), func(id string) (bool, error) {
		_, filesystemTag, err := state.ParseFilesystemAttachmentId(id)
		if err != nil {
			return false, errors.Annotate(err, "parsing filesystem attachment ID")
		}
		f, err := fw.Backend.Filesystem(filesystemTag)
		if errors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Trace(err)
		}
		_, err = f.Volume()
		return err == state.ErrNoBackingVolume, nil
	})
}

// WatchMachineManagedFilesystemAttachments returns a strings watcher that
// reports lifecycle change sfor attachments to both machine-scoped filesystems,
// and model-scoped, volume-backed filesystems that are attached to the
// specified machine.
func (fw Watchers) WatchMachineManagedFilesystemAttachments(m names.MachineTag) state.StringsWatcher {
	w := &machineFilesystemAttachmentsWatcher{
		stringsWatcherBase: stringsWatcherBase{out: make(chan []string)},
		backend:            fw.Backend,
		machine:            m,
		changes:            make(set.Strings),
		machineFilesystemAttachments:     fw.Backend.WatchMachineFilesystemAttachments(m),
		modelFilesystemAttachments:       fw.Backend.WatchModelFilesystemAttachments(),
		modelVolumeAttachments:           fw.Backend.WatchModelVolumeAttachments(),
		modelVolumesAttached:             make(set.Tags),
		modelVolumeFilesystemAttachments: make(map[names.VolumeTag]string),
	}
	go func() {
		defer w.tomb.Done()
		defer watcher.Stop(w.machineFilesystemAttachments, &w.tomb)
		defer watcher.Stop(w.modelFilesystemAttachments, &w.tomb)
		defer watcher.Stop(w.modelVolumeAttachments, &w.tomb)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// machineFilesystemAttachmentsWatcher is a strings watcher that reports
// lifechcle changes for attachments to both machine-scoped filesystems,
// and model-scoped, volume-backed filesystems that are attached to the
// specified machine.
//
// NOTE(axw) we use the existence of the *volume* attachment rather than
// filesystem attachment because the filesystem attachment can be destroyed
// before the filesystem, but the volume attachment cannot.
type machineFilesystemAttachmentsWatcher struct {
	stringsWatcherBase
	changes                          set.Strings
	backend                          Backend
	machine                          names.MachineTag
	machineFilesystemAttachments     state.StringsWatcher
	modelFilesystemAttachments       state.StringsWatcher
	modelVolumeAttachments           state.StringsWatcher
	modelVolumesAttached             set.Tags
	modelVolumeFilesystemAttachments map[names.VolumeTag]string
}

func (w *machineFilesystemAttachmentsWatcher) loop() error {
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
		case values, ok := <-w.machineFilesystemAttachments.Changes():
			if !ok {
				return watcher.EnsureErr(w.machineFilesystemAttachments)
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
				machineTag, filesystemTag, err := state.ParseFilesystemAttachmentId(id)
				if err != nil {
					return errors.Annotate(err, "parsing filesystem attachment ID")
				}
				if machineTag != w.machine {
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
				machineTag, volumeTag, err := state.ParseVolumeAttachmentId(id)
				if err != nil {
					return errors.Annotate(err, "parsing volume attachment ID")
				}
				if machineTag != w.machine {
					continue
				}
				if err := w.modelVolumeAttachmentChanged(volumeTag); err != nil {
					return errors.Trace(err)
				}
			}
		case out <- w.changes.SortedValues():
			w.changes = make(set.Strings)
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

func (w *machineFilesystemAttachmentsWatcher) modelFilesystemAttachmentChanged(
	filesystemAttachmentId string,
	filesystemTag names.FilesystemTag,
) error {
	filesystem, err := w.backend.Filesystem(filesystemTag)
	if errors.IsNotFound(err) {
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

func (w *machineFilesystemAttachmentsWatcher) modelVolumeAttachmentChanged(volumeTag names.VolumeTag) error {
	va, err := w.backend.VolumeAttachment(w.machine, volumeTag)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "getting volume attachment")
	}
	if errors.IsNotFound(err) || va.Life() == state.Dead {
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
	go func() {
		defer fw.tomb.Done()
		defer watcher.Stop(fw.w, &fw.tomb)
		fw.tomb.Kill(fw.loop())
	}()
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
