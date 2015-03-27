// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
)

// entityWatchers contains watchers for entities whose lifecycles are not under
// the control of this storage provisioner.
type entityWatchers struct {
	volumeAccessor           VolumeAccessor
	volumeAttachmentWatchers map[params.MachineStorageId]*volumeAttachmentWatcher
	volumeAttachmentsChanges chan []params.MachineStorageId
}

func newEntityWatchers(v VolumeAccessor) *entityWatchers {
	return &entityWatchers{
		volumeAccessor:           v,
		volumeAttachmentWatchers: make(map[params.MachineStorageId]*volumeAttachmentWatcher),
		volumeAttachmentsChanges: make(chan []params.MachineStorageId),
	}
}

func (w *entityWatchers) Stop() error {
	for _, w := range w.volumeAttachmentWatchers {
		if err := w.stop(); err != nil {
			return errors.Annotate(err, "stopping volume watcher")
		}
	}
	return nil
}

// watchVolumeAttachment starts a watcher for the volume attachment with the
// specified tags. The storage provisioner watches volume attachments which
// are not its responsibility when it needs to know about changes to their
// provisioning status.
func (w *entityWatchers) watchVolumeAttachment(m names.MachineTag, v names.VolumeTag) error {
	id := params.MachineStorageId{
		MachineTag:    m.String(),
		AttachmentTag: v.String(),
	}
	if _, ok := w.volumeAttachmentWatchers[id]; ok {
		return nil
	}
	watcher, err := newVolumeAttachmentWatcher(m, v, w.volumeAccessor, w.volumeAttachmentsChanges)
	if err != nil {
		return err
	}
	w.volumeAttachmentWatchers[id] = watcher
	return nil
}

type volumeAttachmentWatcher struct {
	tomb   tomb.Tomb
	id     params.MachineStorageId
	w      apiwatcher.NotifyWatcher
	output chan<- []params.MachineStorageId
}

func newVolumeAttachmentWatcher(
	machineTag names.MachineTag,
	volumeTag names.VolumeTag,
	accessor VolumeAccessor,
	out chan<- []params.MachineStorageId,
) (*volumeAttachmentWatcher, error) {
	id := params.MachineStorageId{
		MachineTag:    machineTag.String(),
		AttachmentTag: volumeTag.String(),
	}
	vw, err := accessor.WatchVolumeAttachment(machineTag, volumeTag)
	if err != nil {
		return nil, errors.Annotatef(err, "watching volume attachment %v", id)
	}
	w := &volumeAttachmentWatcher{
		id:     id,
		w:      vw,
		output: out,
	}
	go func() {
		defer w.tomb.Done()
		defer watcher.Stop(vw, &w.tomb)
		w.tomb.Kill(w.loop())
	}()
	return w, nil
}

func (w *volumeAttachmentWatcher) loop() error {
	outValue := []params.MachineStorageId{w.id}
	for {
		var out chan<- []params.MachineStorageId
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.w.Changes():
			if !ok {
				return watcher.EnsureErr(w.w)
			}
			out = w.output
		case out <- outValue:
			out = nil
		}
	}
}

func (w *volumeAttachmentWatcher) stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}
