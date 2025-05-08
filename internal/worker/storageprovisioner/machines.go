// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/rpc/params"
)

// watchMachine starts a machine watcher if there is not already one for the
// specified tag. The watcher will notify the worker when the machine changes,
// for example when it is provisioned.
func watchMachine(deps *dependencies, tag names.MachineTag) {
	_, ok := deps.machines[tag]
	if ok {
		return
	}
	w, err := newMachineWatcher(deps.config.Machines, tag, deps.machineChanges, deps.config.Logger)
	if err != nil {
		deps.kill(errors.Trace(err))
	} else if err := deps.addWorker(w); err != nil {
		deps.kill(errors.Trace(err))
	} else {
		deps.machines[tag] = w
	}
}

// refreshMachine refreshes the specified machine's instance ID. If it is set,
// then the machine watcher is stopped and pending entities' parameters are
// updated. If the machine is not provisioned yet, this method is a no-op.
func refreshMachine(ctx context.Context, deps *dependencies, tag names.MachineTag) error {
	w, ok := deps.machines[tag]
	if !ok {
		return errors.Errorf("machine %s is not being watched", tag.Id())
	}
	stopAndRemove := func() error {
		_ = worker.Stop(w)
		delete(deps.machines, tag)
		return nil
	}
	results, err := deps.config.Machines.InstanceIds(ctx, []names.MachineTag{tag})
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
	machineProvisioned(deps, tag, instance.Id(results[0].Result))
	// machine provisioning is the only thing we care about;
	// stop the watcher.
	return stopAndRemove()
}

// machineProvisioned is called when a watched machine is provisioned.
func machineProvisioned(deps *dependencies, tag names.MachineTag, instanceId instance.Id) {
	for _, params := range deps.incompleteVolumeParams {
		if params.Attachment.Machine != tag || params.Attachment.InstanceId != "" {
			continue
		}
		params.Attachment.InstanceId = instanceId
		updatePendingVolume(deps, params)
	}
	for id, params := range deps.incompleteVolumeAttachmentParams {
		if params.Machine != tag || params.InstanceId != "" {
			continue
		}
		params.InstanceId = instanceId
		updatePendingVolumeAttachment(deps, id, params)
	}
	for id, params := range deps.incompleteFilesystemAttachmentParams {
		if params.Machine != tag || params.InstanceId != "" {
			continue
		}
		params.InstanceId = instanceId
		updatePendingFilesystemAttachment(deps, id, params)
	}
}

type machineWatcher struct {
	catacomb catacomb.Catacomb
	accessor MachineAccessor
	tag      names.MachineTag
	out      chan<- names.MachineTag
	logger   logger.Logger
}

func newMachineWatcher(
	accessor MachineAccessor,
	tag names.MachineTag,
	out chan<- names.MachineTag,
	logger logger.Logger,
) (*machineWatcher, error) {
	w := &machineWatcher{
		accessor: accessor,
		tag:      tag,
		out:      out,
		logger:   logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "storage-provisioner-machine",
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (mw *machineWatcher) loop() error {
	ctx, cancel := mw.scopedContext()
	defer cancel()

	w, err := mw.accessor.WatchMachine(ctx, mw.tag)
	if err != nil {
		return errors.Annotate(err, "watching machine")
	}
	if err := mw.catacomb.Add(w); err != nil {
		return errors.Trace(err)
	}
	mw.logger.Debugf(ctx, "watching machine %s", mw.tag.Id())
	defer mw.logger.Debugf(ctx, "finished watching machine %s", mw.tag.Id())
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

func (mw *machineWatcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(mw.catacomb.Context(context.Background()))
}
