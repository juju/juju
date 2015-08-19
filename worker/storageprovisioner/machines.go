// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/watcher"
)

// watchMachine starts a machine watcher if there is not already one for the
// specified tag. The watcher will notify the worker when the machine changes,
// for example when it is provisioned.
func watchMachine(ctx *context, tag names.MachineTag) {
	_, ok := ctx.machines[tag]
	if ok {
		return
	}
	w := newMachineWatcher(ctx.machineAccessor, tag, ctx.machineChanges)
	ctx.machines[tag] = w
}

// refreshMachine refreshes the specified machine's instance ID. If it is set,
// then the machine watcher is stopped and pending entities' parameters are
// updated. If the machine is not provisioned yet, this method is a no-op.
func refreshMachine(ctx *context, tag names.MachineTag) error {
	w, ok := ctx.machines[tag]
	if !ok {
		return errors.Errorf("machine %s is not being watched", tag.Id())
	}
	stopAndRemove := func() error {
		if err := w.stop(); err != nil {
			return errors.Annotate(err, "stopping machine watcher")
		}
		delete(ctx.machines, tag)
		return nil
	}
	results, err := ctx.machineAccessor.InstanceIds([]names.MachineTag{tag})
	if err != nil {
		return errors.Annotate(err, "getting machine instance ID")
	}
	if err := results[0].Error; err != nil {
		if params.IsCodeNotProvisioned(err) {
			return nil
		} else if params.IsCodeNotFound(err) {
			// Machine is gone, so stop watching.
			return stopAndRemove()
		}
		return errors.Annotate(err, "getting machine instance ID")
	}
	machineProvisioned(ctx, tag, instance.Id(results[0].Result))
	// machine provisioning is the only thing we care about;
	// stop the watcher.
	return stopAndRemove()
}

// machineProvisioned is called when a watched machine is provisioned.
func machineProvisioned(ctx *context, tag names.MachineTag, instanceId instance.Id) {
	for _, params := range ctx.incompleteVolumeParams {
		if params.Attachment.Machine != tag || params.Attachment.InstanceId != "" {
			continue
		}
		params.Attachment.InstanceId = instanceId
		updatePendingVolume(ctx, params)
	}
	for id, params := range ctx.incompleteVolumeAttachmentParams {
		if params.Machine != tag || params.InstanceId != "" {
			continue
		}
		params.InstanceId = instanceId
		updatePendingVolumeAttachment(ctx, id, params)
	}
	for id, params := range ctx.pendingFilesystemAttachments {
		if params.Machine != tag || params.InstanceId != "" {
			continue
		}
		params.InstanceId = instanceId
		ctx.pendingFilesystemAttachments[id] = params
	}
}

type machineWatcher struct {
	tomb       tomb.Tomb
	accessor   MachineAccessor
	tag        names.MachineTag
	instanceId instance.Id
	out        chan<- names.MachineTag
}

func newMachineWatcher(
	accessor MachineAccessor,
	tag names.MachineTag,
	out chan<- names.MachineTag,
) *machineWatcher {
	w := &machineWatcher{
		accessor: accessor,
		tag:      tag,
		out:      out,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w
}

func (mw *machineWatcher) stop() error {
	mw.tomb.Kill(nil)
	return mw.tomb.Wait()
}

func (mw *machineWatcher) loop() error {
	w, err := mw.accessor.WatchMachine(mw.tag)
	if err != nil {
		return errors.Annotate(err, "watching machine")
	}
	logger.Debugf("watching machine %s", mw.tag.Id())
	defer logger.Debugf("finished watching machine %s", mw.tag.Id())
	var out chan<- names.MachineTag
	for {
		select {
		case <-mw.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-w.Changes():
			if !ok {
				return watcher.EnsureErr(w)
			}
			out = mw.out
		case out <- mw.tag:
			out = nil
		}
	}
}
