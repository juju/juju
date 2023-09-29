// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
)

type upgradeWatcher struct {
	ctx      context.Context
	catacomb catacomb.Catacomb

	st          State
	wf          WatcherFactory
	upgradeUUID string

	out chan struct{}
}

// NewUpgradeWatcher creates a watcher which notifies when controller node
// which owns the upgrade - complete the process.
func NewUpgradeWatcher(ctx context.Context, st State, wf WatcherFactory, upgradeUUID string) (watcher.NotifyWatcher, error) {
	w := &upgradeWatcher{
		ctx:         ctx,
		st:          st,
		wf:          wf,
		upgradeUUID: upgradeUUID,
		out:         make(chan struct{}),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *upgradeWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *upgradeWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *upgradeWatcher) Changes() <-chan struct{} {
	return w.out
}

func (w *upgradeWatcher) loop() error {
	defer close(w.out)

	uuidWatcher, err := w.wf.NewUUIDsWatcher("upgrade_info", changestream.Create|changestream.Update)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(uuidWatcher); err != nil {
		return errors.Trace(err)
	}

	var out chan struct{}
	in := uuidWatcher.Changes()

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
			upgradeInfo, err := w.st.SelectUpgradeInfo(w.scopedContext(), w.upgradeUUID)
			if err != nil {
				return errors.Trace(err)
			}
			if upgradeInfo.DBCompletedAt.Before(time.Now()) {
				// Tick over to dispatch mode.
				in = nil
				out = w.out
			}
		case out <- struct{}{}:
			// We have dispatched. Tick over to read mode.
			in = uuidWatcher.Changes()
			out = nil
		}
	}
}

func (w *upgradeWatcher) scopedContext() context.Context {
	return w.catacomb.Context(context.Background())
}
