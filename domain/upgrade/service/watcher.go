// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
)

type upgradeReadyWatcher struct {
	ctx      context.Context
	catacomb catacomb.Catacomb

	st          State
	upgradeUUID string

	in  <-chan []string
	out chan struct{}
}

// NewUpgradeReadyWatcher creates a watcher which notifies when all controller
// nodes have been registered, meaning the upgrade is ready to start
func NewUpgradeReadyWatcher(ctx context.Context, st State, wf WatcherFactory, upgradeUUID string) (watcher.NotifyWatcher, error) {
	namespaceWatcher, err := wf.NewNamespaceWatcher("upgrade_info_controller_node", changestream.Create|changestream.Update, "")
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeReadyWatcher{
		ctx:         ctx,
		st:          st,
		upgradeUUID: upgradeUUID,
		out:         make(chan struct{}),
		in:          namespaceWatcher.Changes(),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	if err := w.catacomb.Add(namespaceWatcher); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *upgradeReadyWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *upgradeReadyWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *upgradeReadyWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *upgradeReadyWatcher) loop() error {
	defer close(w.out)

	// By reassigning the in and out channels, we effectively ticktock between
	// read mode and dispatch mode. This ensures we always dispatch deltas that
	// we received before reading more, and every channel read/write is guarded
	// by checks of the tomb and subscription liveness.
	// Start in read mode so we don't send an erroneous initial message
	var out chan struct{}
	in := w.in

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-w.ctx.Done():
			w.catacomb.Kill(context.Cause(w.ctx))
		case _, ok := <-in:
			if !ok {
				return nil
			}
			ready, err := w.st.AllProvisionedControllersReady(w.ctx, w.upgradeUUID)
			if err != nil {
				return errors.Trace(err)
			}
			if ready {
				// Tick over to dispatch mode.
				in = nil
				out = w.out
			}
		case out <- struct{}{}:
			// We have dispatched. Tick over to read mode.
			in = w.in
			out = nil
		}
	}
}
