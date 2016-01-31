// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

// watchMachine starts a machine watcher if there is not already one for the
// specified tag. The watcher will notify the worker when the machine changes,
// for example when it is provisioned.
func watchMachine(ctx *context, tag names.MachineTag) {
	_, ok := ctx.machines[tag]
	if ok {
		return
	}
	w, err := newMachineWatcher(ctx.config.Machines, tag, ctx.machineChanges)
	if err != nil {
		ctx.kill(errors.Trace(err))
	} else if err := ctx.addWorker(w); err != nil {
		ctx.kill(errors.Trace(err))
	} else {
		ctx.machines[tag] = w
	}
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
		worker.Stop(w)
		delete(ctx.machines, tag)
		return nil
	}
	results, err := ctx.config.Machines.InstanceIds([]names.MachineTag{tag})
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
	for id, params := range ctx.incompleteFilesystemAttachmentParams {
		if params.Machine != tag || params.InstanceId != "" {
			continue
		}
		params.InstanceId = instanceId
		updatePendingFilesystemAttachment(ctx, id, params)
	}
}

type machineWatcher struct {
	catacomb   catacomb.Catacomb
	accessor   MachineAccessor
	tag        names.MachineTag
	instanceId instance.Id
	out        chan<- names.MachineTag
}

func newMachineWatcher(
	accessor MachineAccessor,
	tag names.MachineTag,
	out chan<- names.MachineTag,
) (*machineWatcher, error) {
	w := &machineWatcher{
		accessor: accessor,
		tag:      tag,
		out:      out,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (mw *machineWatcher) loop() error {
	w, err := mw.accessor.WatchMachine(mw.tag)
	if err != nil {
		return errors.Annotate(err, "watching machine")
	}
	if err := mw.catacomb.Add(w); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("watching machine %s", mw.tag.Id())
	defer logger.Debugf("finished watching machine %s", mw.tag.Id())
	var out chan<- names.MachineTag
	for {
		select {
		case <-mw.catacomb.Dying():
			return mw.catacomb.ErrDying()
		case _, ok := <-w.Changes():
			if !ok {
				return errors.New("machine watcher closed")
			}
			out = mw.out
		case out <- mw.tag:
			out = nil
		}
	}
}

// Kill is part of the worker.Worker interface.
func (mw *machineWatcher) Kill() {
	mw.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (mw *machineWatcher) Wait() error {
	return mw.catacomb.Wait()
}
